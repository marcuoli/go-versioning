package gitversion_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	versioning "github.com/marcuoli/go-versioning"
	"github.com/marcuoli/go-versioning/gitversion"
)

func TestVerifyReleaseRequiresExistingTagHeadAndCleanSource(t *testing.T) {
	dir, r, first := repo(t)
	ctx := context.Background()
	options := opts(first)
	plan, err := r.Next(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.VerifyRelease(ctx, plan.Version, options); err == nil {
		t.Fatal("accepted uncreated release tag")
	}
	if _, err := r.Tag(ctx, options, ""); err != nil {
		t.Fatal(err)
	}
	if err := r.VerifyRelease(ctx, plan.Version, options); err != nil {
		t.Fatal(err)
	}
	if err := r.VerifyRelease(ctx, "1.2.3+20261024.99", options); err == nil {
		t.Fatal("accepted other release version")
	}
	dirty := filepath.Join(dir, "unstaged.txt")
	if err := os.WriteFile(dirty, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := r.VerifyRelease(ctx, plan.Version, options); err == nil {
		t.Fatal("accepted dirty release source")
	}
	if err := os.Remove(dirty); err != nil {
		t.Fatal(err)
	}
	second := commit(t, dir)
	if err := r.VerifyRelease(ctx, plan.Version, options); err == nil {
		t.Fatal("accepted non-HEAD release")
	}
	minor := gitversion.PlanOptions{Commit: second, Branch: "main", PR: 10, Bump: versioning.BumpMinor}
	bumped, err := r.Tag(ctx, minor, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := r.VerifyRelease(ctx, bumped.Plan.Version, minor); err != nil {
		t.Fatalf("reviewed minor bump failed: %v", err)
	}
	minor.Bump = versioning.BumpInherit
	if err := r.VerifyRelease(ctx, bumped.Plan.Version, minor); err == nil {
		t.Fatal("ignored release bump policy")
	}
}
