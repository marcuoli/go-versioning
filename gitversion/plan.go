package gitversion

import (
	"context"
	"errors"
	"strings"

	versioning "github.com/marcuoli/go-versioning"
)

// PlanOptions supplies explicit allocation identity and first-allocation policy.
// InitialCore is used only when no predecessor exists; bumps require a predecessor.
type PlanOptions struct {
	Commit      string          `json:"commit"`
	Branch      string          `json:"branch"`
	InitialCore string          `json:"initial_core"`
	PR          uint64          `json:"pr"`
	Bump        versioning.Bump `json:"bump"`
}

// Plan is a read-only allocation proposal. Verification describes the history
// checked locally; remote validation is performed separately by Tag.
type Plan struct {
	Version      string `json:"version"`
	Tag          string `json:"tag"`
	Commit       string `json:"commit"`
	PreviousTag  string `json:"previous_tag"`
	Branch       string `json:"branch"`
	Verification string `json:"verification"`
}

func (r *Repository) branchCommit(ctx context.Context, branch string) (string, error) {
	if branch == "" || strings.HasPrefix(branch, "-") || strings.ContainsAny(branch, "\x00\r\n") {
		return "", errors.New("valid target branch is required")
	}
	ref := branch
	if !strings.HasPrefix(ref, "refs/") {
		ref = "refs/heads/" + ref
	}
	if !strings.HasPrefix(ref, "refs/heads/") && !strings.HasPrefix(ref, "refs/remotes/") {
		return "", errors.New("target must name a branch")
	}
	if _, err := r.run(ctx, "branch name validation", "check-ref-format", ref); err != nil {
		return "", err
	}
	return r.run(ctx, "target branch resolution", "rev-parse", "--verify", ref+"^{commit}")
}

// Next computes an immutable tag without writing refs. Commit must be a full
// object identity on Branch's first-parent history. Current-commit tags are
// validated as reruns, never used as the predecessor for another bump.
func (r *Repository) Next(ctx context.Context, o PlanOptions) (Plan, error) {
	if !fullCommit.MatchString(o.Commit) {
		return Plan{}, errors.New("target commit must be a full hexadecimal identity")
	}
	if o.InitialCore != "" {
		if _, err := versioning.ParseCore(o.InitialCore); err != nil {
			return Plan{}, errors.New("explicit initial core must be valid")
		}
	}
	if o.PR == 0 {
		return Plan{}, errors.New("PR must be positive")
	}
	if err := r.complete(ctx); err != nil {
		return Plan{}, err
	}
	target, err := r.run(ctx, "target commit resolution", "rev-parse", "--verify", o.Commit+"^{commit}")
	if err != nil {
		return Plan{}, err
	}
	if target != strings.ToLower(o.Commit) {
		return Plan{}, errors.New("target identity does not identify that commit exactly")
	}
	tip, err := r.branchCommit(ctx, o.Branch)
	if err != nil {
		return Plan{}, err
	}
	branchHistory, err := r.history(ctx, tip)
	if err != nil {
		return Plan{}, err
	}
	admitted := false
	for _, c := range branchHistory {
		if c == target {
			admitted = true
			break
		}
	}
	if !admitted {
		return Plan{}, errors.New("target commit is not on the branch first-parent history")
	}
	history, err := r.history(ctx, target)
	if err != nil {
		return Plan{}, err
	}
	tags, err := r.tags(ctx)
	if err != nil {
		return Plan{}, err
	}
	previous, err := predecessor(history[1:], tags)
	if err != nil {
		return Plan{}, err
	}
	var core versioning.Core
	if previous == nil {
		if o.Bump != "" && o.Bump != versioning.BumpInherit {
			return Plan{}, errors.New("initial allocation cannot also bump a version")
		}
		core, err = versioning.ParseCore(o.InitialCore)
		if err != nil {
			return Plan{}, errors.New("initial core is required and must be valid without a predecessor")
		}
	} else {
		if o.InitialCore != "" {
			return Plan{}, errors.New("initial core is only valid without a predecessor")
		}
		core = previous.version.Core
	}
	stamp, err := r.commitTime(ctx, target)
	if err != nil {
		return Plan{}, err
	}
	v, err := versioning.Next(core, stamp, o.PR, o.Bump)
	if err != nil {
		return Plan{}, errors.New("invalid version allocation policy")
	}
	name, err := v.Tag()
	if err != nil {
		return Plan{}, errors.New("invalid allocated version")
	}
	plan := Plan{Version: v.String(), Tag: name, Commit: target, Branch: o.Branch, Verification: "local-only: complete first-parent history; remote not verified"}
	if previous != nil {
		plan.PreviousTag = previous.name
	}
	currentCount := 0
	for _, tag := range tags {
		if tag.commit == target {
			currentCount++
			if tag.name != name {
				return Plan{}, errors.New("target already has a different application tag")
			}
		}
		if tag.name == name && tag.commit != target {
			return Plan{}, errors.New("expected application tag belongs to another commit")
		}
	}
	if currentCount > 1 {
		return Plan{}, errors.New("target has ambiguous application tags")
	}
	return plan, nil
}
