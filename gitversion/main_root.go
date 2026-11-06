package gitversion

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
)

// PrimaryMain returns the primary checkout only when it is on the explicitly
// designated integration branch. Linked/CI source checkouts never become a new
// VERSION authority by accident. This checks Git designation, not permanence,
// remote project identity or whether a host mount is the real canonical root.
func (r *Repository) PrimaryMain(ctx context.Context, branch string) (string, error) {
	if branch == "" {
		return "", errors.New("main branch is required")
	}
	if _, e := r.run(ctx, "main branch validation", "check-ref-format", "refs/heads/"+branch); e != nil {
		return "", e
	}
	output, e := r.run(ctx, "primary main checkout", "worktree", "list", "--porcelain", "-z")
	if e != nil {
		return "", e
	}
	first := strings.SplitN(output, "\x00\x00", 2)[0]
	root := ""
	onBranch := false
	for _, line := range strings.Split(first, "\x00") {
		if strings.HasPrefix(line, "worktree ") {
			root = strings.TrimPrefix(line, "worktree ")
		}
		if line == "branch refs/heads/"+branch {
			onBranch = true
		}
		if line == "bare" {
			return "", errors.New("main VERSION requires a primary worktree, not a bare repository")
		}
	}
	if root == "" || !onBranch {
		return "", errors.New("primary permanent checkout must be on the designated main branch")
	}
	resolved, e := filepath.EvalSymlinks(root)
	if e != nil {
		return "", e
	}
	return resolved, nil
}

// CheckCounterFiles enforces the common operational-file Git policy without
// editing source or ignore rules. Call it on permanent primary main before
// binding/allocating; tracked legacy files require an owning-project migration.
func (r *Repository) CheckCounterFiles(ctx context.Context) error {
	tracked, e := r.run(ctx, "VERSION tracking validation", "ls-files", "-z", "--", ":(top,icase,literal)VERSION", ":(top,icase,literal)VERSION.authority.json")
	if e != nil {
		return e
	}
	if tracked != "" {
		return errors.New("main VERSION/ownership is tracked; preserve legacy semantics through a reviewed migration before using numeric operational files")
	}
	for _, name := range []string{"VERSION", "VERSION.authority.json"} {
		if _, e = r.run(ctx, "VERSION ignore validation", "check-ignore", "--quiet", "--no-index", "--", name); e != nil {
			return errors.New("main VERSION and VERSION.authority.json must both be Git-ignored through reviewed project rules; the tool will not edit or untrack them")
		}
	}
	return nil
}
