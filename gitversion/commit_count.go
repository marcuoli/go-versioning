package gitversion

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"
)

// CommitCountEvidence records a one-time reachable-commit continuity seed.
// It is not the number of historical builds. Callers must first investigate and
// prefer their verified legacy sequence; no build operation invokes this method.
type CommitCountEvidence struct {
	Commit      string `json:"commit"`
	Count       uint64 `json:"count"`
	Command     string `json:"command"`
	CheckedTime string `json:"checked_time"`
}

// CommitCount counts unique commits reachable from HEAD, including merged side
// history but excluding unrelated refs. The caller designates the canonical local
// integration branch; HEAD must equal it and history must be complete. It does
// not fetch or authenticate the caller's designation against a hosting service.
func (r *Repository) CommitCount(ctx context.Context, branch string) (CommitCountEvidence, error) {
	var result CommitCountEvidence
	if err := r.complete(ctx); err != nil {
		return result, err
	}
	if branch == "" {
		return result, errors.New("canonical integration branch is required")
	}
	if _, err := r.run(ctx, "integration branch validation", "check-ref-format", "refs/heads/"+branch); err != nil {
		return result, err
	}
	head, err := r.run(ctx, "seed HEAD", "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return result, err
	}
	canonical, err := r.run(ctx, "seed branch", "rev-parse", "--verify", "refs/heads/"+branch+"^{commit}")
	if err != nil {
		return result, err
	}
	if !fullCommit.MatchString(head) || head != canonical {
		return result, errors.New("seed HEAD must match the designated canonical integration branch")
	}
	count, err := r.run(ctx, "reachable commit count", "rev-list", "--count", head, "--")
	if err != nil {
		return result, err
	}
	n, err := strconv.ParseUint(count, 10, 64)
	if err != nil || n == 0 || strconv.FormatUint(n, 10) != count {
		return result, errors.New("invalid reachable commit count")
	}
	// Detect a checkout movement during evidence collection. The actual count uses
	// the immutable full object ID, never --all or a moving branch expression.
	after, err := r.run(ctx, "seed HEAD verification", "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return result, err
	}
	afterBranch, err := r.run(ctx, "seed branch verification", "rev-parse", "--verify", "refs/heads/"+branch+"^{commit}")
	if err != nil {
		return result, err
	}
	if after != head || afterBranch != head {
		return result, errors.New("integration HEAD changed while collecting seed evidence")
	}
	return CommitCountEvidence{Commit: head, Count: n, Command: fmt.Sprintf("git rev-list --count %s --", head), CheckedTime: time.Now().UTC().Format(time.RFC3339Nano)}, nil
}
