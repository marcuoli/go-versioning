package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/marcuoli/go-versioning/buildcounter"
)

func TestMainVersionCLIFromLinkedAndSeparateCheckouts(t *testing.T) {
	root := t.TempDir()
	main := filepath.Join(root, "principal-a\u00e7\u00e3o")
	if e := os.Mkdir(main, 0700); e != nil {
		t.Fatal(e)
	}
	cliGit(t, main, "init", "-b", "main")
	cliGit(t, main, "commit", "--allow-empty", "-m", "canonical fixture")
	invoke := func(args ...string) (string, int) {
		t.Helper()
		var out, er bytes.Buffer
		code := run(context.Background(), args, &out, &er)
		if code != 0 {
			t.Log(er.String())
		}
		return out.String(), code
	}
	linked := filepath.Join(root, "linked")
	cliGit(t, main, "worktree", "add", "-b", "feature", linked)
	t.Cleanup(func() { cliGit(t, main, "worktree", "remove", linked) })
	store := filepath.Join(root, "journal")
	if out, code := invoke("counter-init", "--repo", linked, "--main-repo", main, "--store", store, "--app", "example", "--json"); code != 0 {
		t.Fatal(out)
	}
	if _, e := os.Stat(filepath.Join(main, "VERSION")); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(filepath.Join(linked, "VERSION")); !os.IsNotExist(e) {
		t.Fatal("VERSION followed linked source")
	}
	if _, e := os.Stat(filepath.Join(store, "VERSION")); !os.IsNotExist(e) {
		t.Fatal("VERSION remains in V-style journal")
	}
	if out, code := invoke("build", "--repo", linked, "--store", store, "--app", "example", "--request-id", "linked", "--json"); code != 0 {
		t.Fatal(out)
	}
	clone := filepath.Join(root, "ci-copy")
	cliGit(t, root, "clone", main, clone)
	if _, code := invoke("build", "--repo", clone, "--store", store, "--app", "example", "--request-id", "ci", "--json"); code == 0 {
		t.Fatal("accepted implicit independent checkout authority")
	}
	if out, code := invoke("build", "--repo", clone, "--main-repo", main, "--store", store, "--app", "example", "--request-id", "ci", "--json"); code != 0 {
		t.Fatal(out)
	}
	if _, code := invoke("counter-init", "--repo", clone, "--main-repo", linked, "--store", filepath.Join(root, "bad"), "--app", "example"); code == 0 {
		t.Fatal("designated a linked worktree as permanent main")
	}
}
func TestCounterRelocateCLIChangesNoReservation(t *testing.T) {
	root := t.TempDir()
	main := filepath.Join(root, "main")
	if e := os.Mkdir(main, 0700); e != nil {
		t.Fatal(e)
	}
	cliGit(t, main, "init", "-b", "main")
	sha := cliGit(t, main, "commit", "--allow-empty", "-m", "fixture")
	_ = sha
	p := filepath.Join(root, "legacy")
	s, e := buildcounter.Initialize(context.Background(), p, "example")
	if e != nil {
		t.Fatal(e)
	}
	commit := cliGit(t, main, "rev-parse", "HEAD")
	r, e := s.Reserve(context.Background(), buildcounter.Request{ID: "old", Version: "0.0.0+unknown", Commit: commit})
	if e != nil {
		t.Fatal(e)
	}
	var out, er bytes.Buffer
	bad := []string{"build", "--repo", main, "--store", p, "--app", "example", "--request-id", "must-not-allocate", "--json"}
	if code := run(context.Background(), bad, &out, &er); code == 0 {
		t.Fatal("legacy layout still allocated through new CLI")
	}
	status, e := s.Status(context.Background())
	if e != nil || status.TrackedAttempts != 1 {
		t.Fatal("legacy rejection changed count", status, e)
	}
	out.Reset()
	er.Reset()
	args := []string{"counter-relocate", "--repo", main, "--main-repo", main, "--store", p, "--app", "example", "--json"}
	if code := run(context.Background(), args, &out, &er); code != 0 {
		t.Fatal(out.String(), er.String())
	}
	again, e := s.Reserve(context.Background(), buildcounter.Request{ID: "old", Version: "0.0.0+unknown", Commit: commit})
	if e != nil || again != r {
		t.Fatal(again, e)
	}
	// The CLI returns the physical path; the input can retain a host temp alias.
	versionFile, e := filepath.EvalSymlinks(filepath.Join(main, "VERSION"))
	if e != nil {
		t.Fatal(e)
	}
	st, e := s.Status(context.Background())
	if e != nil || st.LastNumber != 1 || st.TrackedAttempts != 1 || st.VersionFile != versionFile {
		t.Fatal(st, e)
	}
}

func TestCounterCheckCLIReadsWithoutAllocation(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "main")
	if e := os.Mkdir(repo, 0700); e != nil {
		t.Fatal(e)
	}
	cliGit(t, repo, "init", "-b", "main")
	cliGit(t, repo, "commit", "--allow-empty", "-m", "fixture")
	store := filepath.Join(root, "journal")
	var out, er bytes.Buffer
	base := []string{"--repo", repo, "--main-repo", repo, "--store", store, "--app", "example", "--json"}
	if code := run(context.Background(), append([]string{"counter-init"}, base...), &out, &er); code != 0 {
		t.Fatal(er.String())
	}
	out.Reset()
	er.Reset()
	if code := run(context.Background(), append([]string{"counter-check"}, base...), &out, &er); code != 0 {
		t.Fatal(er.String())
	}
	s, e := buildcounter.Open(store, "example")
	if e != nil {
		t.Fatal(e)
	}
	st, e := s.Status(context.Background())
	if e != nil || st.LastNumber != 0 || !st.LocationCompliant {
		t.Fatal(st, e)
	}
}

func TestCounterInitRequiresExplicitMainAndReviewedTrackingPolicy(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "main")
	if e := os.Mkdir(repo, 0700); e != nil {
		t.Fatal(e)
	}
	cliGit(t, repo, "init", "-b", "main")
	cliGit(t, repo, "commit", "--allow-empty", "-m", "fixture")
	store := filepath.Join(root, "temporary-journal")
	var out, er bytes.Buffer
	// This models the consumer hazard: real source with a throwaway journal, but
	// no explicit main-write designation. It must leave both paths untouched.
	args := []string{"counter-init", "--repo", repo, "--store", store, "--app", "example"}
	if code := run(context.Background(), args, &out, &er); code == 0 {
		t.Fatal("implicit test journal bound main")
	}
	if _, e := os.Stat(filepath.Join(repo, "VERSION")); !os.IsNotExist(e) {
		t.Fatal("implicit call wrote main")
	}
	if _, e := os.Stat(store); !os.IsNotExist(e) {
		t.Fatal("implicit call created journal")
	}
	if e := os.WriteFile(filepath.Join(repo, "VERSION"), []byte("0\n"), 0600); e != nil {
		t.Fatal(e)
	}
	cliGit(t, repo, "add", "-f", "VERSION")
	cliGit(t, repo, "commit", "-m", "legacy tracked value")
	args = append(args, "--main-repo", repo)
	out.Reset()
	er.Reset()
	if code := run(context.Background(), args, &out, &er); code == 0 {
		t.Fatal("tracked legacy VERSION adopted without migration")
	}
	b, _ := os.ReadFile(filepath.Join(repo, "VERSION"))
	if string(b) != "0\n" {
		t.Fatal("legacy file changed")
	}
	if _, e := os.Stat(store); !os.IsNotExist(e) {
		t.Fatal("tracked rejection created journal")
	}
}

func TestCounterInitRejectsMissingIgnoreRulesWithoutCreatingState(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "main")
	if e := os.Mkdir(repo, 0700); e != nil {
		t.Fatal(e)
	}
	cliGit(t, repo, "init", "-b", "main")
	cliGit(t, repo, "commit", "--allow-empty", "-m", "fixture")
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(root, "empty-global"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	if e := os.WriteFile(filepath.Join(repo, ".git", "info", "exclude"), nil, 0600); e != nil {
		t.Fatal(e)
	}
	store := filepath.Join(root, "journal")
	var out, er bytes.Buffer
	args := []string{"counter-init", "--repo", repo, "--main-repo", repo, "--store", store, "--app", "example"}
	if code := run(context.Background(), args, &out, &er); code == 0 {
		t.Fatal("unignored mutable files accepted")
	}
	if _, e := os.Stat(store); !os.IsNotExist(e) {
		t.Fatal("created journal before policy check")
	}
	if _, e := os.Stat(filepath.Join(repo, "VERSION")); !os.IsNotExist(e) {
		t.Fatal("created root file before policy check")
	}
}
