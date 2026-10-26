package gitversion_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	versioning "github.com/marcuoli/go-versioning"
	"github.com/marcuoli/go-versioning/gitversion"
)

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	var environment []string
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if key != "GIT_DIR" && key != "GIT_WORK_TREE" && !strings.HasPrefix(key, "GIT_CONFIG_") {
			environment = append(environment, entry)
		}
	}
	cmd.Env = append(environment, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_AUTHOR_NAME=Example", "GIT_AUTHOR_EMAIL=dev@example.com", "GIT_COMMITTER_NAME=Example", "GIT_COMMITTER_EMAIL=dev@example.com", "GIT_AUTHOR_DATE=2026-10-25T01:00:00Z", "GIT_COMMITTER_DATE=2026-10-25T01:00:00Z", "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %s", args, out)
	}
	return strings.TrimSpace(string(out))
}
func repo(t *testing.T) (string, *gitversion.Repository, string) {
	t.Helper()
	dir := t.TempDir()
	git(t, dir, "init", "-b", "main")
	git(t, dir, "config", "commit.gpgsign", "false")
	git(t, dir, "config", "tag.gpgSign", "false")
	commit := commit(t, dir)
	r, err := gitversion.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	return dir, r, commit
}
func commit(t *testing.T, dir string) string {
	t.Helper()
	git(t, dir, "commit", "--allow-empty", "-m", "Example change")
	return git(t, dir, "rev-parse", "HEAD")
}
func opts(commit string) gitversion.PlanOptions {
	return gitversion.PlanOptions{Commit: commit, Branch: "main", PR: 9, InitialCore: "1.2.3"}
}
func bare(t *testing.T, dir string) string {
	t.Helper()
	remote := t.TempDir()
	git(t, remote, "init", "--bare")
	git(t, dir, "remote", "add", "origin", remote)
	git(t, dir, "push", "origin", "main")
	return remote
}

func TestBootstrapTagAndImmutableRerun(t *testing.T) {
	dir, r, c := repo(t)
	o := opts(c)
	p, err := r.Next(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	if p.Tag != "v1.2.3+20261024.9" || p.Commit != c || p.PreviousTag != "" {
		t.Fatalf("plan=%+v", p)
	}
	if got := git(t, dir, "tag", "--list"); got != "" {
		t.Fatalf("next mutated tags: %s", got)
	}
	a, err := r.Tag(context.Background(), o, "")
	if err != nil {
		t.Fatal(err)
	}
	if !a.Local || !a.Created || a.Remote {
		t.Fatalf("result=%+v", a)
	}
	b, err := r.Tag(context.Background(), o, "")
	if err != nil {
		t.Fatal(err)
	}
	if !b.Local || b.Created {
		t.Fatalf("rerun=%+v", b)
	}
	info, err := r.Inspect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.Version != "1.2.3-snapshot+20261024.9" || info.Commit != c || info.BuildTime != "2026-10-25T01:00:00Z" {
		t.Fatalf("info=%+v", info)
	}
}
func TestAllocationValidation(t *testing.T) {
	dir, r, c := repo(t)
	cases := []gitversion.PlanOptions{opts(c[:12]), opts("HEAD"), opts(strings.Repeat("f", 40)), opts(c), opts(c), opts(c), opts(c)}
	cases[3].PR = 0
	cases[4].InitialCore = ""
	cases[5].Bump = versioning.BumpPatch
	cases[6].Branch = "missing"
	for i, o := range cases {
		if _, err := r.Next(context.Background(), o); err == nil {
			t.Errorf("case %d accepted", i)
		}
	}
	git(t, dir, "switch", "-c", "side")
	side := commit(t, dir)
	git(t, dir, "switch", "main")
	if _, err := r.Next(context.Background(), opts(side)); err == nil {
		t.Fatal("off-branch commit accepted")
	}
	if _, err := gitversion.New(""); err == nil {
		t.Fatal("implicit repository accepted")
	}
}
func TestCanonicalPriorityAndTargetExclusion(t *testing.T) {
	dir, r, c := repo(t)
	git(t, dir, "tag", "v1.2.3+20261024.1", c)
	middle := commit(t, dir)
	git(t, dir, "tag", "build/7.8.9+20261024.2", middle)
	target := commit(t, dir)
	o := opts(target)
	o.InitialCore = ""
	o.Bump = versioning.BumpMinor
	p, err := r.Next(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	if p.PreviousTag != "v1.2.3+20261024.1" || p.Tag != "v1.3.0+20261024.9" {
		t.Fatalf("plan=%+v", p)
	}
	if _, err = r.Tag(context.Background(), o, ""); err != nil {
		t.Fatal(err)
	}
	if _, err = r.Tag(context.Background(), o, ""); err != nil {
		t.Fatalf("bump reapplied on rerun: %v", err)
	}
}
func TestLegacyFallbackAnnotatedAndSnapshot(t *testing.T) {
	dir, r, c := repo(t)
	git(t, dir, "tag", "-a", "build/2.3.4+20261024.1", "-m", "Example release", c)
	target := commit(t, dir)
	o := opts(target)
	o.InitialCore = ""
	p, err := r.Next(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	if p.Tag != "v2.3.4+20261024.9" || p.PreviousTag != "build/2.3.4+20261024.1" {
		t.Fatalf("plan=%+v", p)
	}
	info, err := r.Inspect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.Version != "2.3.4-snapshot+20261024.1" {
		t.Fatalf("info=%+v", info)
	}
}
func TestConflictsMalformedAndAmbiguousTags(t *testing.T) {
	for _, mode := range []string{"malformed", "ambiguous", "target", "elsewhere"} {
		t.Run(mode, func(t *testing.T) {
			dir, r, c := repo(t)
			switch mode {
			case "malformed":
				git(t, dir, "tag", "v1.2.3+20260230.1", c)
			case "ambiguous":
				git(t, dir, "tag", "v1.2.3+20261024.1", c)
				git(t, dir, "tag", "v2.0.0+20261024.2", c)
			case "target":
				git(t, dir, "tag", "build/8.0.0+20261024.2", c)
			case "elsewhere":
				git(t, dir, "tag", "v1.2.3+20261024.9", c)
			}
			target := c
			if mode != "target" {
				target = commit(t, dir)
			}
			if _, err := r.Next(context.Background(), opts(target)); err == nil {
				t.Fatal("conflicting application tag accepted")
			}
		})
	}
}
func TestRemotePushIdempotenceAndFailureRecovery(t *testing.T) {
	dir, r, c := repo(t)
	remote := bare(t, dir)
	o := opts(c)
	result, err := r.Tag(context.Background(), o, "origin")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Local || !result.Remote || !result.Created {
		t.Fatalf("result=%+v", result)
	}
	result, err = r.Tag(context.Background(), o, "origin")
	if err != nil || result.Created || !result.Remote {
		t.Fatalf("rerun=%+v err=%v", result, err)
	}
	target := commit(t, dir)
	o = opts(target)
	o.InitialCore = ""
	o.PR = 10
	hook := filepath.Join(remote, "hooks", "pre-receive")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\nexit 1\n"), 0755); err != nil {
		t.Fatal(err)
	}
	result, err = r.Tag(context.Background(), o, "origin")
	if err == nil || !result.Local || !result.Created || result.Remote {
		t.Fatalf("failed push=%+v err=%v", result, err)
	}
	if strings.Contains(err.Error(), dir) || strings.Contains(err.Error(), remote) || strings.Contains(err.Error(), "missing") {
		t.Fatalf("unsafe error=%v", err)
	}
	if err := os.Remove(hook); err != nil {
		t.Fatal(err)
	}
	result, err = r.Tag(context.Background(), o, "origin")
	if err != nil || result.Created || !result.Remote {
		t.Fatalf("retry=%+v err=%v", result, err)
	}
}
func TestRemotePreflightDoesNotCreateTag(t *testing.T) {
	for _, mode := range []string{"missing-local", "moved", "missing-predecessor", "malformed", "target"} {
		t.Run(mode, func(t *testing.T) {
			dir, r, c := repo(t)
			remote := bare(t, dir)
			git(t, dir, "tag", "v1.2.3+20261024.1", c)
			target := commit(t, dir)
			o := opts(target)
			o.InitialCore = ""
			switch mode {
			case "missing-local":
				git(t, remote, "tag", "v4.0.0+20261024.2", c)
			case "moved":
				git(t, remote, "tag", "v1.2.3+20261024.1", c)
				git(t, dir, "tag", "-f", "v1.2.3+20261024.1", target)
			case "missing-predecessor":
			case "malformed":
				git(t, remote, "tag", "v1.2.3+20260230.1", c)
			case "target":
				git(t, dir, "push", "origin", "main")
				git(t, remote, "tag", "v9.0.0+20261024.8", target)
			}
			result, err := r.Tag(context.Background(), o, "origin")
			if err == nil || result.Created {
				t.Fatalf("preflight result=%+v err=%v", result, err)
			}
			if git(t, dir, "tag", "--list", "v1.2.3+20261024.9") != "" {
				t.Fatal("preflight created tag")
			}
		})
	}
}
func TestShallowCancellationAndEnvironmentIsolation(t *testing.T) {
	dir, r, c := repo(t)
	t.Setenv("GIT_DIR", filepath.Join(t.TempDir(), "private"))
	t.Setenv("GIT_WORK_TREE", t.TempDir())
	if _, err := r.Next(context.Background(), opts(c)); err != nil {
		t.Fatalf("inherited environment selected another repo: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := r.Inspect(ctx); err == nil {
		t.Fatal("cancellation ignored")
	}
	shallow := filepath.Join(t.TempDir(), "clone")
	git(t, t.TempDir(), "clone", "--depth=1", "file://"+filepath.ToSlash(dir), shallow)
	sr, err := gitversion.New(shallow)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = sr.Next(context.Background(), opts(c)); err == nil {
		t.Fatal("shallow bootstrap accepted")
	}
}
func TestRemoteNameValidation(t *testing.T) {
	_, r, c := repo(t)
	for _, remote := range []string{"--all", "https://fake-user:fake-placeholder@example.invalid/project.git", "x\nfake-placeholder", "missing"} {
		result, err := r.Tag(context.Background(), opts(c), remote)
		if err == nil || result.Created {
			t.Errorf("remote %q accepted", remote)
		}
		if err != nil && strings.Contains(err.Error(), "fake-placeholder") {
			t.Errorf("remote error leaks input: %v", err)
		}
	}
}
func TestSecondParentAndTagObjectCannotSupplyCommitIdentity(t *testing.T) {
	dir, r, base := repo(t)
	git(t, dir, "tag", "v1.2.3+20261024.1", base)
	git(t, dir, "switch", "-c", "side")
	side := commit(t, dir)
	git(t, dir, "tag", "v8.0.0+20261024.2", side)
	git(t, dir, "switch", "main")
	git(t, dir, "commit", "--allow-empty", "-m", "Example main change")
	git(t, dir, "merge", "--no-ff", "side", "-m", "Example merge")
	target := git(t, dir, "rev-parse", "HEAD")
	o := opts(side)
	o.InitialCore = ""
	if _, err := r.Next(context.Background(), o); err == nil {
		t.Fatal("second-parent commit admitted")
	}
	o = opts(target)
	o.InitialCore = ""
	p, err := r.Next(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	if p.PreviousTag != "v1.2.3+20261024.1" {
		t.Fatalf("second-parent tag used: %+v", p)
	}
	git(t, dir, "tag", "-a", "example", "-m", "Example annotated tag", target)
	o.Commit = git(t, dir, "rev-parse", "refs/tags/example")
	if _, err = r.Next(context.Background(), o); err == nil {
		t.Fatal("tag object hash accepted as full commit identity")
	}
}
func TestMalformedExplicitInitialCoreIsRejectedWithPredecessor(t *testing.T) {
	dir, r, c := repo(t)
	git(t, dir, "tag", "v1.2.3+20261024.1", c)
	o := opts(commit(t, dir))
	o.InitialCore = "not-a-version"
	if _, err := r.Next(context.Background(), o); err == nil {
		t.Fatal("malformed explicit initial core accepted")
	}
}
func TestSigningConfigurationDoesNotChangeLightweightTag(t *testing.T) {
	dir, r, c := repo(t)
	git(t, dir, "config", "tag.gpgSign", "true")
	result, err := r.Tag(context.Background(), opts(c), "")
	if err != nil {
		t.Fatal(err)
	}
	if kind := git(t, dir, "cat-file", "-t", result.Plan.Tag); kind != "commit" {
		t.Fatalf("tag is %s", kind)
	}
}
func TestExplicitInitialCoreRequiresBootstrap(t *testing.T) {
	dir, r, c := repo(t)
	git(t, dir, "tag", "v1.2.3+20261024.1", c)
	if _, err := r.Next(context.Background(), opts(commit(t, dir))); err == nil {
		t.Fatal("explicit initial core accepted with predecessor")
	}
}
func TestPushDestinationMustMatchValidatedRemote(t *testing.T) {
	for _, multiple := range []bool{false, true} {
		t.Run(map[bool]string{false: "different", true: "multiple"}[multiple], func(t *testing.T) {
			dir, r, c := repo(t)
			remote := bare(t, dir)
			other := t.TempDir()
			git(t, other, "init", "--bare")
			if multiple {
				git(t, dir, "config", "--add", "remote.origin.pushurl", remote)
			}
			git(t, dir, "config", "--add", "remote.origin.pushurl", other)
			result, err := r.Tag(context.Background(), opts(c), "origin")
			if err == nil || result.Created || result.Local {
				t.Fatalf("unverified destination accepted: %+v err=%v", result, err)
			}
		})
	}
}
func TestRejectsGraftAlteredHistory(t *testing.T) {
	for _, inherited := range []bool{false, true} {
		t.Run(map[bool]string{false: "repository", true: "inherited"}[inherited], func(t *testing.T) {
			dir, r, base := repo(t)
			git(t, dir, "tag", "v1.2.3+20261024.1", base)
			target := commit(t, dir)
			graft := filepath.Join(dir, ".git", "info", "grafts")
			if inherited {
				graft = filepath.Join(t.TempDir(), "grafts")
				t.Setenv("GIT_GRAFT_FILE", graft)
			}
			if err := os.WriteFile(graft, []byte(target+"\n"), 0600); err != nil {
				t.Fatal(err)
			}
			o := opts(target)
			o.InitialCore = "9.9.9"
			if p, err := r.Next(context.Background(), o); err == nil {
				t.Fatalf("graft silently changed predecessor: %+v", p)
			}
			if info, err := r.Inspect(context.Background()); err == nil {
				t.Fatalf("graft silently changed inspection: %+v", info)
			}
		})
	}
}
func TestNewContextRejectsCanceledAndExpiredContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := gitversion.NewContext(ctx, t.TempDir()); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled constructor=%v", err)
	}
	expired, stop := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer stop()
	if _, err := gitversion.NewContext(expired, t.TempDir()); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expired constructor=%v", err)
	}
	if _, err := gitversion.NewContext(nil, t.TempDir()); err == nil {
		t.Fatal("nil constructor context accepted")
	}
}
