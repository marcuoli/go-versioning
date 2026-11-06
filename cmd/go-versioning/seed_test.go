package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	versioning "github.com/marcuoli/go-versioning"
	"github.com/marcuoli/go-versioning/buildcounter"
)

func TestCounterSeedCLIAndOneTimeCommitFallback(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "app")
	if e := os.Mkdir(repo, 0700); e != nil {
		t.Fatal(e)
	}
	cliGit(t, repo, "init", "-b", "main")
	cliGit(t, repo, "commit", "--allow-empty", "-m", "first")
	cliGit(t, repo, "commit", "--allow-empty", "-m", "second")
	invoke := func(args ...string) (string, int) {
		t.Helper()
		var out, er bytes.Buffer
		code := run(context.Background(), args, &out, &er)
		if code != 0 {
			t.Log(er.String())
		}
		return out.String(), code
	}
	store := filepath.Join(root, "counter")
	base := []string{"--repo", repo, "--main-repo", repo, "--store", store, "--app", "example", "--json"}
	args := append([]string{"counter-init"}, base...)
	args = append(args, "--seed-commits", "--evidence", "legacy VERSION, hooks and scripts inspected: no prior computation/value")
	out, code := invoke(args...)
	if code != 0 {
		t.Fatal(out)
	}
	var st buildcounter.Status
	if e := json.Unmarshal([]byte(out), &st); e != nil || st.LastNumber != 2 {
		t.Fatal(st, e)
	}
	args = append([]string{"build"}, base...)
	args = append(args, "--request-id", "build-one")
	out, code = invoke(args...)
	if code != 0 {
		t.Fatal(out)
	}
	var info versioning.Info
	if e := json.Unmarshal([]byte(out), &info); e != nil || info.OverallBuildNumber != 3 {
		t.Fatal(info, e)
	}
	// Git advances, but normal builds keep incrementing the authority.
	cliGit(t, repo, "commit", "--allow-empty", "-m", "third")
	cliGit(t, repo, "commit", "--allow-empty", "-m", "fourth")
	args[len(args)-1] = "build-two"
	out, code = invoke(args...)
	if code != 0 {
		t.Fatal(out)
	}
	if e := json.Unmarshal([]byte(out), &info); e != nil || info.OverallBuildNumber != 4 {
		t.Fatal(info, e)
	}
	if out, code = invoke(append([]string{"counter-status"}, base...)...); code != 0 {
		t.Fatal(out)
	}
	other := filepath.Join(root, "invalid")
	args = []string{"counter-init", "--repo", repo, "--main-repo", repo, "--store", other, "--app", "example", "--seed-commits"}
	if _, code = invoke(args...); code != 2 {
		t.Fatal("missing investigation admitted")
	}
	if _, e := os.Stat(other); !os.IsNotExist(e) {
		t.Fatal("invalid seed created state")
	}
	repo = filepath.Join(root, "legacy-app")
	if e := os.Mkdir(repo, 0700); e != nil {
		t.Fatal(e)
	}
	cliGit(t, repo, "init", "-b", "main")
	cliGit(t, repo, "commit", "--allow-empty", "-m", "legacy fixture")
	legacy := buildcounter.Seed{Number: 70, Method: "legacy", Commit: cliGit(t, repo, "rev-parse", "HEAD"), Command: "legacy hook inspection", Evidence: "verified historical third component increments on commits", CheckedTime: "2026-11-05T20:00:00Z"}
	data, _ := json.Marshal(legacy)
	seed := filepath.Join(root, "legacy.json")
	if e := os.WriteFile(seed, data, 0600); e != nil {
		t.Fatal(e)
	}
	args = []string{"counter-init", "--repo", repo, "--main-repo", repo, "--store", other, "--app", "example", "--json"}
	if _, code = invoke(args...); code != 0 {
		t.Fatal("init")
	}
	args[0] = "counter-migrate"
	args = append(args, "--seed-file", seed)
	out, code = invoke(args...)
	if code != 0 {
		t.Fatal(out)
	}
	if e := json.Unmarshal([]byte(out), &st); e != nil || st.LastNumber != 70 {
		t.Fatal(st, e)
	}
	retry, c := invoke(args...)
	if c != 0 || retry != out {
		t.Fatal("migration response retry changed", retry)
	}
}
