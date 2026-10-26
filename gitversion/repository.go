// Package gitversion inspects explicit Git repositories and allocates immutable
// application tags. It never fetches, moves, deletes, or force-pushes tags.
package gitversion

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	versioning "github.com/marcuoli/go-versioning"
)

// Repository is an explicit repository root, independent of the current directory.
type Repository struct{ path string }

const commandTimeout = 30 * time.Second

var fullCommit = regexp.MustCompile(`^(?:[0-9a-fA-F]{40}|[0-9a-fA-F]{64})$`)

// New validates an explicit worktree root or bare repository. Empty paths and
// directories that only inherit an enclosing repository are rejected.
func New(path string) (*Repository, error) { return NewContext(context.Background(), path) }

// NewContext validates an explicit repository root using the caller's deadline
// and cancellation. Nil, canceled, and expired contexts are rejected up front.
func NewContext(ctx context.Context, path string) (*Repository, error) {
	if ctx == nil {
		return nil, errors.New("context is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("repository validation: %w", err)
	}
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("repository path is required")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, errors.New("invalid repository path")
	}
	absolute, err = filepath.EvalSymlinks(absolute)
	if err != nil {
		return nil, errors.New("repository path is unavailable")
	}
	r := &Repository{path: absolute}
	bare, err := r.run(ctx, "repository validation", "rev-parse", "--is-bare-repository")
	if err != nil {
		return nil, err
	}
	var root string
	if bare == "true" {
		root, err = r.run(ctx, "repository validation", "rev-parse", "--absolute-git-dir")
	} else {
		root, err = r.run(ctx, "repository validation", "rev-parse", "--show-toplevel")
	}
	if err != nil {
		return nil, err
	}
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, errors.New("repository root is unavailable")
	}
	if filepath.Clean(resolved) != filepath.Clean(absolute) {
		return nil, errors.New("path must identify the repository root")
	}
	return r, nil
}

func gitEnvironment() []string {
	denied := map[string]bool{"GIT_DIR": true, "GIT_WORK_TREE": true, "GIT_INDEX_FILE": true, "GIT_COMMON_DIR": true, "GIT_OBJECT_DIRECTORY": true, "GIT_ALTERNATE_OBJECT_DIRECTORIES": true, "GIT_NAMESPACE": true, "GIT_SHALLOW_FILE": true, "GIT_GRAFT_FILE": true, "GIT_REPLACE_REF_BASE": true, "GIT_CONFIG": true, "GIT_CONFIG_PARAMETERS": true, "GIT_CONFIG_COUNT": true, "GIT_PREFIX": true, "GIT_INTERNAL_SUPER_PREFIX": true, "GIT_QUARANTINE_PATH": true}
	env := make([]string, 0, len(os.Environ()))
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		key = strings.ToUpper(key)
		if denied[key] || strings.HasPrefix(key, "GIT_CONFIG_KEY_") || strings.HasPrefix(key, "GIT_CONFIG_VALUE_") || key == "GIT_NO_REPLACE_OBJECTS" || key == "GIT_TERMINAL_PROMPT" || key == "GIT_NO_LAZY_FETCH" {
			continue
		}
		env = append(env, entry)
	}
	return append(env, "GIT_NO_REPLACE_OBJECTS=1", "GIT_TERMINAL_PROMPT=0", "GIT_NO_LAZY_FETCH=1", "GIT_GRAFT_FILE="+os.DevNull)
}

func (r *Repository) run(ctx context.Context, operation string, args ...string) (string, error) {
	if ctx == nil {
		return "", errors.New("context is required")
	}
	if r == nil || r.path == "" {
		return "", errors.New("repository is not initialized")
	}
	bounded, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()
	cmd := exec.CommandContext(bounded, "git", append([]string{"-C", r.path}, args...)...)
	cmd.Env = gitEnvironment()
	// A descendant can retain output pipes after Git exits or is canceled.
	// Bound pipe draining as well as the lifetime of the direct Git process.
	cmd.WaitDelay = 250 * time.Millisecond
	out, err := cmd.Output() // Git's stderr and command arguments never become public diagnostics.
	if err != nil {
		if bounded.Err() != nil {
			return "", fmt.Errorf("git %s: %w", operation, bounded.Err())
		}
		return "", fmt.Errorf("git %s failed", operation)
	}
	return strings.TrimSpace(string(out)), nil
}

func (r *Repository) complete(ctx context.Context) error {
	// Reject configured grafts and neutralize graft application in every Git
	// subprocess. Shallow checks alone cannot detect graft-truncated history.
	if os.Getenv("GIT_GRAFT_FILE") != "" {
		return errors.New("inherited Git graft configuration is not supported")
	}
	common, err := r.run(ctx, "graft history validation", "rev-parse", "--git-common-dir")
	if err != nil {
		return err
	}
	if !filepath.IsAbs(common) {
		common = filepath.Join(r.path, common)
	}
	grafts, err := os.Open(filepath.Join(common, "info", "grafts"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return errors.New("Git graft history validation failed")
	}
	if err == nil {
		defer grafts.Close()
		lines := bufio.NewScanner(grafts)
		for lines.Scan() {
			line := strings.TrimSpace(lines.Text())
			if line != "" && !strings.HasPrefix(line, "#") {
				return errors.New("active Git grafts are incompatible with complete history")
			}
		}
		if lines.Err() != nil {
			return errors.New("Git graft history validation failed")
		}
	}
	shallow, err := r.run(ctx, "history validation", "rev-parse", "--is-shallow-repository")
	if err != nil {
		return err
	}
	if shallow != "false" {
		return errors.New("complete first-parent history is required; shallow repository rejected")
	}
	return nil
}
func (r *Repository) commitTime(ctx context.Context, commit string) (time.Time, error) {
	value, err := r.run(ctx, "commit time", "show", "-s", "--format=%cI", commit, "--")
	if err != nil {
		return time.Time{}, err
	}
	date, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, errors.New("invalid Git committer time")
	}
	return date, nil
}

type applicationTag struct {
	name, commit string
	version      versioning.Version
	legacy       bool
}

func candidate(name string) bool {
	return strings.HasPrefix(name, "build/") || (strings.HasPrefix(name, "v") && strings.Contains(name, "+"))
}
func (r *Repository) tags(ctx context.Context) ([]applicationTag, error) {
	names, err := r.run(ctx, "tag inventory", "for-each-ref", "--format=%(refname:strip=2)", "refs/tags/")
	if err != nil {
		return nil, err
	}
	var result []applicationTag
	for _, name := range strings.Split(names, "\n") {
		if !candidate(name) {
			continue
		}
		v, legacy, err := versioning.ParseTag(name)
		if err != nil {
			return nil, errors.New("malformed application tag in repository")
		}
		commit, err := r.run(ctx, "tag commit validation", "rev-parse", "--verify", "refs/tags/"+name+"^{commit}")
		if err != nil {
			return nil, err
		}
		result = append(result, applicationTag{name: name, commit: commit, version: v, legacy: legacy})
	}
	return result, nil
}
func (r *Repository) history(ctx context.Context, commit string) ([]string, error) {
	value, err := r.run(ctx, "first-parent history", "rev-list", "--first-parent", commit, "--")
	if err != nil {
		return nil, err
	}
	if value == "" {
		return nil, errors.New("first-parent history is unavailable")
	}
	return strings.Split(value, "\n"), nil
}
func predecessor(history []string, tags []applicationTag) (*applicationTag, error) {
	for _, legacy := range []bool{false, true} {
		for _, commit := range history {
			var found *applicationTag
			for i := range tags {
				tag := &tags[i]
				if tag.legacy == legacy && tag.commit == commit {
					if found != nil {
						return nil, errors.New("ambiguous application tags on first-parent history")
					}
					found = tag
				}
			}
			if found != nil {
				return found, nil
			}
		}
	}
	return nil, nil
}

// Inspect reads HEAD identity and committer time. Every source-derived version
// is a snapshot, including a checkout exactly at a release tag. Without an
// application tag on first-parent history Version is versioning.UnknownVersion.
func (r *Repository) Inspect(ctx context.Context) (versioning.Info, error) {
	if err := r.complete(ctx); err != nil {
		return versioning.Info{}, err
	}
	commit, err := r.run(ctx, "HEAD inspection", "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return versioning.Info{}, err
	}
	stamp, err := r.commitTime(ctx, commit)
	if err != nil {
		return versioning.Info{}, err
	}
	tags, err := r.tags(ctx)
	if err != nil {
		return versioning.Info{}, err
	}
	history, err := r.history(ctx, commit)
	if err != nil {
		return versioning.Info{}, err
	}
	previous, err := predecessor(history, tags)
	if err != nil {
		return versioning.Info{}, err
	}
	info := versioning.Info{Version: versioning.UnknownVersion, Commit: commit, BuildTime: stamp.Format(time.RFC3339)}
	if previous != nil {
		v := previous.version
		v.Snapshot = true
		info.Version = v.String()
	}
	return info, nil
}
