package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	versioning "github.com/marcuoli/go-versioning"
	"github.com/marcuoli/go-versioning/buildcounter"
	"github.com/marcuoli/go-versioning/gitversion"
)

func runCounter(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	machine := wantsJSON(args)
	fail := func(code int, err error, reservation *buildcounter.Reservation) int {
		if machine {
			_ = json.NewEncoder(stderr).Encode(struct {
				Error       string                    `json:"error"`
				Reservation *buildcounter.Reservation `json:"reservation,omitempty"`
			}{err.Error(), reservation})
		} else {
			_, _ = fmt.Fprintf(stderr, "go-versioning: %s\n", err)
		}
		return code
	}
	command := args[0]
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var repoPath, storePath, app, id, version, commit, format, branch, bump, initial string
	var release, seedCommits bool
	var seedFile, evidence, mainRepo, mainBranch string
	var timeout time.Duration
	var recovery buildcounter.RecoveryOptions
	fs.StringVar(&repoPath, "repo", "", "explicit repository root")
	fs.StringVar(&storePath, "store", "", "absolute external counter store")
	fs.StringVar(&app, "app", "", "stable application identity")
	fs.StringVar(&mainRepo, "main-repo", "", "designated primary permanent main root on this authority host")
	fs.StringVar(&mainBranch, "main-branch", "main", "primary integration branch")
	fs.BoolVar(&machine, "json", machine, "JSON output/errors")
	fs.DurationVar(&timeout, "timeout", 30*time.Second, "operation deadline")
	if command == "counter-init" || command == "counter-migrate" {
		fs.StringVar(&seedFile, "seed-file", "", "explicit legacy Seed JSON evidence")
		fs.BoolVar(&seedCommits, "seed-commits", false, "one-time canonical HEAD reachable-commit fallback")
		fs.StringVar(&branch, "branch", "main", "canonical integration branch for commit fallback")
		fs.StringVar(&evidence, "evidence", "", "legacy investigation evidence for commit fallback")
	}
	if command == "counter-recover" {
		fs.StringVar(&recovery.CounterID, "counter-id", "", "expected existing authority identity")
		fs.Uint64Var(&recovery.CurrentNumber, "expected-current", 0, "expected lagging VERSION value")
		fs.Uint64Var(&recovery.LastNumber, "expected-last", 0, "expected complete ledger high-water mark")
		fs.BoolVar(&recovery.Apply, "apply", false, "apply verified recovery after preserving its receipt")
	}
	if command == "build" {
		fs.StringVar(&id, "request-id", "", "unique logical build-attempt ID")
		fs.StringVar(&version, "version", "", "explicit source or verified release version")
		fs.StringVar(&commit, "commit", "", "optional expected full HEAD identity")
		fs.StringVar(&format, "format", "text", "text, json or ldflags")
		fs.BoolVar(&release, "release", false, "verify an existing local release tag and clean worktree")
		fs.StringVar(&branch, "branch", "main", "release target branch")
		fs.StringVar(&bump, "bump", "inherit", "reviewed release core bump")
		fs.StringVar(&initial, "initial-core", "", "explicit first release core")
	}
	if err := fs.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			if _, err = io.WriteString(stdout, helpText); err == nil {
				return 0
			}
			return fail(1, err, nil)
		}
		return fail(2, errors.New("invalid build/counter flags; use --help"), nil)
	}
	if fs.NArg() != 0 || repoPath == "" || storePath == "" || app == "" || timeout <= 0 {
		return fail(2, errors.New("--repo, --store, --app and a positive timeout are required; no positional arguments"), nil)
	}
	if (command == "counter-init" || command == "counter-relocate" || command == "counter-recover") && mainRepo == "" {
		return fail(2, errors.New("--main-repo is required for initialization/relocation/recovery; use real permanent main for adoption and a standalone isolated Git main for disposable tests"), nil)
	}
	if !filepath.IsAbs(storePath) {
		return fail(2, errors.New("--store must be an explicit absolute external path"), nil)
	}
	if command == "counter-recover" {
		present := map[string]bool{}
		fs.Visit(func(f *flag.Flag) { present[f.Name] = true })
		if !present["expected-current"] || !present["expected-last"] || recovery.CounterID == "" {
			return fail(2, errors.New("recovery requires --counter-id, --expected-current and --expected-last"), nil)
		}
	}
	if command == "build" && (id == "" || (format != "text" && format != "json" && format != "ldflags")) {
		return fail(2, errors.New("build requires --request-id and --format text|json|ldflags"), nil)
	}
	if command == "build" && !release && (initial != "" || bump != "inherit") {
		return fail(2, errors.New("release allocation policy flags require --release"), nil)
	}
	if ctx == nil {
		return fail(1, errors.New("context is required"), nil)
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	repository, err := gitversion.NewContext(ctx, repoPath)
	if err != nil {
		return fail(1, err, nil)
	}
	if err = externalStore(repoPath, storePath); err != nil {
		return fail(2, err, nil)
	}
	if command == "counter-recover" {
		root, err := counterMain(ctx, repository, mainRepo, mainBranch)
		if err != nil {
			return fail(2, err, nil)
		}
		if err = externalStore(root, storePath); err != nil {
			return fail(2, err, nil)
		}
		store, err := buildcounter.Open(storePath, app)
		if err != nil {
			return fail(1, err, nil)
		}
		recovery.VersionFile = filepath.Join(root, "VERSION")
		result, err := store.RecoverVersion(ctx, recovery)
		if err != nil {
			return fail(1, err, nil)
		}
		if machine {
			err = json.NewEncoder(stdout).Encode(result)
		} else {
			_, err = fmt.Fprintf(stdout, "Counter ID: %s\nVERSION: %d -> %d\nApplied: %t\nAlready recovered: %t\nReceipt: %s\n", result.CounterID, result.PreviousNumber, result.LastNumber, result.Applied, result.AlreadyRecovered, result.ReceiptFile)
		}
		if err != nil {
			return fail(1, errors.New("could not write recovery output; inspect authority before retrying"), nil)
		}
		return 0
	}
	if command == "counter-init" || command == "counter-migrate" || command == "counter-status" || command == "counter-relocate" || command == "counter-check" {
		var store *buildcounter.Store
		var status buildcounter.Status
		var root string
		if command == "counter-init" || command == "counter-relocate" || command == "counter-check" {
			root, err = counterMain(ctx, repository, mainRepo, mainBranch)
			if err != nil {
				return fail(2, err, nil)
			}
			if err = externalStore(root, storePath); err != nil {
				return fail(2, err, nil)
			}
		}
		if command == "counter-check" {
			store, err = buildcounter.Open(storePath, app)
			if err == nil {
				status, err = store.CheckLocation(ctx, filepath.Join(root, "VERSION"))
			}
		} else if command == "counter-relocate" {
			store, err = buildcounter.Open(storePath, app)
			if err == nil {
				status, err = store.RelocateVersion(ctx, filepath.Join(root, "VERSION"))
			}
		} else if command == "counter-status" {
			store, err = buildcounter.Open(storePath, app)
		} else {
			seedRepo := repository
			if seedCommits {
				if root == "" {
					root, err = counterMain(ctx, repository, mainRepo, mainBranch)
					if err != nil {
						return fail(2, err, nil)
					}
				}
				seedRepo, err = gitversion.NewContext(ctx, root)
				if err != nil {
					return fail(1, err, nil)
				}
			}
			seed, e := counterSeed(ctx, seedRepo, seedFile, seedCommits, branch, evidence)
			if e != nil {
				return fail(2, e, nil)
			}
			if command == "counter-init" {
				if seed == nil {
					store, err = buildcounter.InitializeAt(ctx, storePath, app, filepath.Join(root, "VERSION"))
				} else {
					store, err = buildcounter.InitializeSeedAt(ctx, storePath, app, filepath.Join(root, "VERSION"), *seed)
				}
			} else {
				if seed == nil {
					return fail(2, errors.New("counter-migrate requires --seed-file or --seed-commits"), nil)
				}
				store, err = buildcounter.Open(storePath, app)
				if err == nil {
					status, err = store.Migrate(ctx, *seed)
				}
			}
		}
		if err != nil {
			return fail(1, err, nil)
		}
		if command != "counter-migrate" && command != "counter-relocate" && command != "counter-check" {
			status, err = store.Status(ctx)
			if err != nil {
				return fail(1, err, nil)
			}
		}
		if machine {
			err = json.NewEncoder(stdout).Encode(status)
		} else {
			_, err = fmt.Fprintf(stdout, "Application: %s\nCounter ID: %s\nLast number: %d\n", status.Application, status.CounterID, status.LastNumber)
		}
		if err != nil {
			return fail(1, errors.New("could not write counter output; inspect authority before retrying"), nil)
		}
		return 0
	}

	info, err := repository.Inspect(ctx)
	if err != nil {
		return fail(1, err, nil)
	}
	if commit != "" && !strings.EqualFold(commit, info.Commit) {
		return fail(2, errors.New("--commit must match repository HEAD"), nil)
	}
	if version != "" {
		info.Version = version
	}
	// Discard Inspect's commit timestamp. Reserve supplies the actual original
	// build-attempt timestamp, including on an idempotent allocation retry.
	info.BuildTime = ""
	info, err = versioning.Resolve(info, versioning.Info{})
	if err != nil {
		return fail(2, err, nil)
	}
	if release {
		parsed, err := versioning.Parse(info.Version)
		if err != nil || parsed.Snapshot {
			return fail(2, errors.New("--release requires a full application release version"), nil)
		}
		options := gitversion.PlanOptions{Commit: info.Commit, Branch: branch, PR: parsed.PR, Bump: versioning.Bump(bump), InitialCore: initial}
		if err = repository.VerifyRelease(ctx, info.Version, options); err != nil {
			return fail(1, err, nil)
		}
	} else if info.Version != versioning.UnknownVersion {
		parsed, _ := versioning.Parse(info.Version)
		if !parsed.Snapshot {
			return fail(2, errors.New("source builds require snapshot/unknown metadata; use --release for a verified tag"), nil)
		}
	}
	store, err := buildcounter.Open(storePath, app)
	if err != nil {
		return fail(1, err, nil)
	}
	if err = checkCounterMain(ctx, store, repository, mainRepo, mainBranch); err != nil {
		return fail(2, err, nil)
	}
	reservation, err := store.Reserve(ctx, buildcounter.Request{ID: id, Version: info.Version, Commit: info.Commit})
	if err != nil {
		var state *buildcounter.Reservation
		if reservation.Info.OverallBuildNumber != 0 {
			state = &reservation
		}
		return fail(1, err, state)
	}
	switch {
	case format == "ldflags":
		var flags string
		flags, err = reservation.Info.LinkerFlags()
		if err == nil {
			_, err = fmt.Fprintln(stdout, flags)
		}
	case machine || format == "json":
		err = json.NewEncoder(stdout).Encode(reservation.Info)
	default:
		err = render(stdout, false, reservation.Info)
	}
	if err != nil {
		return fail(1, errors.New("could not write reserved build metadata; retry the same request ID"), &reservation)
	}
	return 0
}

// Resolve existing parents and symlinks before comparing paths. No allocator
// state may be placed inside its source checkout (or contain that checkout).
func externalStore(repo, store string) error {
	source, err := filepath.Abs(repo)
	if err != nil {
		return err
	}
	source, err = filepath.EvalSymlinks(source)
	if err != nil {
		return err
	}
	target, err := filepath.EvalSymlinks(store)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		parent, e := filepath.EvalSymlinks(filepath.Dir(store))
		if e != nil {
			return errors.New("counter parent directory must already exist outside source")
		}
		target = filepath.Join(parent, filepath.Base(store))
	}
	contains := func(base, path string) bool {
		relative, e := filepath.Rel(base, path)
		return e == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
	}
	if contains(source, target) || contains(target, source) {
		return errors.New("counter store must be outside the source repository")
	}
	return nil
}
