package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	versioning "github.com/marcuoli/go-versioning"
	"github.com/marcuoli/go-versioning/gitversion"
)

const helpText = `go-versioning: application version metadata and immutable Git tags

Usage:
  go-versioning inspect --repo DIR [--json]
  go-versioning next --repo DIR --branch BRANCH --commit SHA --pr NUMBER [options]
  go-versioning tag --repo DIR --branch BRANCH --commit SHA --pr NUMBER [options]

Options:
  --bump inherit|major|minor|patch  Inherit the core unless a bump is requested
  --initial-core CORE              Explicit first application version core
  --push --remote NAME             Push the exact tag to a configured Git remote
  --timeout DURATION               Overall operation deadline (default 30s)
  --json                           Emit JSON output and JSON errors
  --help                           Show this help

next is read-only. tag creates locally unless --push --remote is supplied.
A complete, non-shallow repository and a full target commit are required.
`

type arguments struct {
	repo, branch, commit, pr, bump, initial, remote string
	json, push                                      bool
	timeout                                         time.Duration
}

type errorOutput struct {
	Error string                `json:"error"`
	State *gitversion.TagResult `json:"state,omitempty"`
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	machine := wantsJSON(args)
	fail := func(code int, err error, state *gitversion.TagResult) int {
		if machine {
			_ = json.NewEncoder(stderr).Encode(errorOutput{Error: err.Error(), State: state})
		} else {
			_, _ = fmt.Fprintf(stderr, "go-versioning: %s\n", err)
			if state != nil && state.Local {
				_, _ = fmt.Fprintf(stderr, "Local tag retained: %s (remote confirmed: %t)\n", state.Plan.Tag, state.Remote)
			}
		}
		return code
	}
	if len(args) == 0 {
		return fail(2, errors.New("a subcommand is required; use --help"), nil)
	}
	if args[0] == "--help" || args[0] == "-h" {
		if _, err := io.WriteString(stdout, helpText); err != nil {
			return fail(1, errors.New("could not write help"), nil)
		}
		return 0
	}
	command := args[0]
	if command != "inspect" && command != "next" && command != "tag" {
		return fail(2, errors.New("unknown subcommand; use --help"), nil)
	}
	var a arguments
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&a.repo, "repo", "", "repository directory")
	fs.BoolVar(&a.json, "json", false, "JSON output")
	fs.DurationVar(&a.timeout, "timeout", 30*time.Second, "operation deadline")
	if command != "inspect" {
		fs.StringVar(&a.branch, "branch", "", "target branch")
		fs.StringVar(&a.commit, "commit", "", "full target commit SHA")
		fs.StringVar(&a.pr, "pr", "", "positive pull-request number")
		fs.StringVar(&a.bump, "bump", "inherit", "core bump")
		fs.StringVar(&a.initial, "initial-core", "", "initial application version core")
	}
	if command == "tag" {
		fs.BoolVar(&a.push, "push", false, "push the exact tag")
		fs.StringVar(&a.remote, "remote", "", "configured remote name")
	}
	if err := fs.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			if _, err := io.WriteString(stdout, helpText); err != nil {
				return fail(1, errors.New("could not write help"), nil)
			}
			return 0
		}
		return fail(2, errors.New("invalid flags; use the subcommand's --help"), nil)
	}
	machine = a.json
	if fs.NArg() != 0 {
		return fail(2, errors.New("positional arguments are not accepted"), nil)
	}
	if a.repo == "" {
		return fail(2, errors.New("--repo is required"), nil)
	}
	if a.timeout <= 0 {
		return fail(2, errors.New("--timeout must be positive"), nil)
	}
	var options gitversion.PlanOptions
	if command != "inspect" {
		if a.branch == "" || a.commit == "" || a.pr == "" {
			return fail(2, errors.New("--branch, --commit and --pr are required"), nil)
		}
		pr, err := positiveNumber(a.pr)
		if err != nil {
			return fail(2, err, nil)
		}
		if !fullSHA(a.commit) {
			return fail(2, errors.New("--commit must be a full hexadecimal Git commit SHA"), nil)
		}
		bump := versioning.Bump(a.bump)
		switch bump {
		case versioning.BumpInherit, versioning.BumpMajor, versioning.BumpMinor, versioning.BumpPatch:
		default:
			return fail(2, errors.New("--bump must be inherit, major, minor or patch"), nil)
		}
		if a.initial != "" {
			if _, err := versioning.ParseCore(a.initial); err != nil {
				return fail(2, errors.New("--initial-core must be a valid SemVer core"), nil)
			}
			if bump != versioning.BumpInherit {
				return fail(2, errors.New("--initial-core cannot be combined with a bump"), nil)
			}
		}
		options = gitversion.PlanOptions{Commit: strings.ToLower(a.commit), Branch: a.branch, PR: pr, Bump: bump, InitialCore: a.initial}
	}
	if a.push && a.remote == "" {
		return fail(2, errors.New("--push requires --remote"), nil)
	}
	if !a.push && a.remote != "" {
		return fail(2, errors.New("--remote requires --push"), nil)
	}
	if err := ctx.Err(); err != nil {
		return fail(1, err, nil)
	}
	ctx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()
	repo, err := gitversion.NewContext(ctx, a.repo)
	if err != nil {
		return fail(1, err, nil)
	}
	switch command {
	case "inspect":
		info, err := repo.Inspect(ctx)
		if err != nil {
			return fail(1, err, nil)
		}
		if err = render(stdout, a.json, info); err != nil {
			return fail(1, errors.New("could not write output"), nil)
		}
	case "next":
		plan, err := repo.Next(ctx, options)
		if err != nil {
			return fail(1, err, nil)
		}
		if err = render(stdout, a.json, plan); err != nil {
			return fail(1, errors.New("could not write output"), nil)
		}
	case "tag":
		remote := ""
		if a.push {
			remote = a.remote
		}
		result, err := repo.Tag(ctx, options, remote)
		if err != nil {
			return fail(1, err, &result)
		}
		if err = render(stdout, a.json, result); err != nil {
			return fail(1, errors.New("could not write output"), &result)
		}
	}
	return 0
}

func wantsJSON(args []string) bool {
	for _, arg := range args {
		if arg == "--json" || arg == "-json" || arg == "--json=true" || arg == "-json=true" {
			return true
		}
	}
	return false
}

func positiveNumber(value string) (uint64, error) {
	if value == "" || value[0] < '1' || value[0] > '9' {
		return 0, errors.New("--pr must be a positive decimal integer without leading zeroes")
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return 0, errors.New("--pr must be a positive decimal integer without leading zeroes")
		}
	}
	n, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return 0, errors.New("--pr exceeds the supported integer range")
	}
	return n, nil
}

func fullSHA(value string) bool {
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	for _, r := range value {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f' || r >= 'A' && r <= 'F') {
			return false
		}
	}
	return true
}

func render(out io.Writer, machine bool, value any) error {
	if machine {
		return json.NewEncoder(out).Encode(value)
	}
	switch v := value.(type) {
	case versioning.Info:
		_, err := fmt.Fprintf(out, "Version: %s\nCommit: %s\nBuild time: %s\n", v.Version, v.Commit, v.BuildTime)
		return err
	case gitversion.Plan:
		_, err := fmt.Fprintf(out, "Version: %s\nTag: %s\nCommit: %s\nBranch: %s\nPrevious tag: %s\nVerification: %s\n", v.Version, v.Tag, v.Commit, v.Branch, v.PreviousTag, v.Verification)
		return err
	case gitversion.TagResult:
		if err := render(out, false, v.Plan); err != nil {
			return err
		}
		_, err := fmt.Fprintf(out, "Local: %t\nRemote: %t\nCreated: %t\n", v.Local, v.Remote, v.Created)
		return err
	}
	return errors.New("unsupported output")
}
