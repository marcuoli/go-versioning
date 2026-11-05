package gitversion

import (
	"context"
	"errors"
)

// VerifyRelease checks a previously allocated local tag, HEAD and clean worktree
// using the existing first-parent/core/date policy. It never creates refs or
// contacts a host; the caller still verifies the trusted merged-PR association
// and remote tag identity before publishing a release build.
func (r *Repository) VerifyRelease(ctx context.Context, version string, options PlanOptions) error {
	plan, err := r.Next(ctx, options)
	if err != nil {
		return err
	}
	if plan.Version != version {
		return errors.New("release version disagrees with allocation policy")
	}
	head, err := r.run(ctx, "release HEAD verification", "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return err
	}
	if head != plan.Commit {
		return errors.New("release must build the selected commit at HEAD")
	}
	tags, err := r.tags(ctx)
	if err != nil {
		return err
	}
	present := false
	for _, tag := range tags {
		if tag.name == plan.Tag && tag.commit == head {
			present = true
		}
	}
	if !present {
		return errors.New("release requires its existing immutable application tag")
	}
	status, err := r.run(ctx, "release worktree verification", "status", "--porcelain=v1", "--untracked-files=normal")
	if err != nil {
		return err
	}
	if status != "" {
		return errors.New("release requires a clean source worktree")
	}
	return nil
}
