package main

import (
	"bytes"
	"context"
	"encoding/json"
	versioning "github.com/marcuoli/go-versioning"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCounterCLIExternalAllocationAndRuntimeInjection(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "app")
	if err := os.Mkdir(repo, 0700); err != nil {
		t.Fatal(err)
	}
	cliGit(t, repo, "init", "-b", "main")
	if err := os.WriteFile(filepath.Join(repo, "source.txt"), []byte("synthetic\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cliGit(t, repo, "add", "source.txt")
	cliGit(t, repo, "commit", "-m", "fixture")
	store := filepath.Join(root, "counter")
	invoke := func(args ...string) (string, int) {
		t.Helper()
		var out, errOut bytes.Buffer
		code := run(context.Background(), args, &out, &errOut)
		if code != 0 {
			t.Log(errOut.String())
		}
		return out.String(), code
	}
	if _, code := invoke("counter-init", "--repo", repo, "--main-repo", repo, "--store", filepath.Join(repo, "state"), "--app", "example", "--json"); code != 2 {
		t.Fatal("accepted source-local counter")
	}
	if _, err := os.Stat(filepath.Join(repo, "state")); !os.IsNotExist(err) {
		t.Fatal("source-local admission created state")
	}
	if _, code := invoke("counter-init", "--repo", repo, "--main-repo", repo, "--store", store, "--app", "example", "--json"); code != 0 {
		t.Fatal("counter-init failed")
	}
	args := []string{"build", "--repo", repo, "--store", store, "--app", "example", "--request-id", "one", "--json"}
	started := time.Now().UTC()
	output, code := invoke(args...)
	if code != 0 {
		t.Fatal("build allocation failed")
	}
	var info versioning.Info
	if err := json.Unmarshal([]byte(output), &info); err != nil {
		t.Fatal(err)
	}
	stamp, err := time.Parse(time.RFC3339, info.BuildTime)
	if err != nil || stamp.Before(started) || stamp.After(time.Now().UTC()) {
		t.Fatalf("not actual reservation time: %s", info.BuildTime)
	}
	if info.OverallBuildNumber != 1 || info.BuildCounterID == "" || info.Version != versioning.UnknownVersion {
		t.Fatalf("info=%+v", info)
	}
	retry, code := invoke(args...)
	if code != 0 || retry != output {
		t.Fatalf("retry changed: %s", retry)
	}
	flagsArgs := append([]string(nil), args[:len(args)-1]...)
	flagsArgs = append(flagsArgs, "--format", "ldflags")
	flags, code := invoke(flagsArgs...)
	if code != 0 || len(flags) == 0 {
		t.Fatal("missing shared linker flags")
	}
	if _, err := info.LinkerFlags(); err != nil {
		t.Fatal(err)
	}
	if output, code := invoke("inspect", "--repo", repo, "--json"); code != 0 || bytes.Contains([]byte(output), []byte("overall_build_number")) {
		t.Fatalf("inspect allocated/inferred counter: %s", output)
	}
}

func TestCounterCLIUsageAndReleaseAdmission(t *testing.T) {
	for _, args := range [][]string{{"counter-init"}, {"build", "--json"}, {"counter-init", "--repo", ".", "--store", "relative", "--app", "app", "--json"}} {
		var out, errOut bytes.Buffer
		if code := run(context.Background(), args, &out, &errOut); code != 2 {
			t.Fatalf("usage code=%d stderr=%s", code, errOut.String())
		}
	}
}

func TestBuildCounterCLIRequiresVerifiedReleaseBeforeReserving(t *testing.T) {
	directory := t.TempDir()
	repo := filepath.Join(directory, "app")
	if err := os.Mkdir(repo, 0700); err != nil {
		t.Fatal(err)
	}
	cliGit(t, repo, "init", "-b", "main")
	if err := os.WriteFile(filepath.Join(repo, ".git", "info", "exclude"), []byte("/VERSION\n/VERSION.authority.json\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "source.txt"), []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	cliGit(t, repo, "add", "source.txt")
	cliGit(t, repo, "commit", "-m", "fixture")
	store := filepath.Join(directory, "counter")
	invoke := func(args ...string) (string, int) {
		t.Helper()
		var out, errOut bytes.Buffer
		code := run(context.Background(), args, &out, &errOut)
		if code != 0 {
			t.Log(errOut.String())
		}
		return out.String(), code
	}
	if _, code := invoke("counter-init", "--repo", repo, "--main-repo", repo, "--store", store, "--app", "example", "--json"); code != 0 {
		t.Fatal("initialize")
	}
	args := []string{"build", "--repo", repo, "--store", store, "--app", "example", "--request-id", "release-one", "--version", "1.2.3+20260102.42", "--json"}
	if _, code := invoke(args...); code == 0 {
		t.Fatal("unverified release accepted as source")
	}
	releaseArgs := append(append([]string(nil), args...), "--release", "--initial-core", "1.2.3")
	if _, code := invoke(releaseArgs...); code == 0 {
		t.Fatal("missing release tag accepted")
	}
	cliGit(t, repo, "tag", "v1.2.3+20260102.42")
	output, code := invoke(releaseArgs...)
	if code != 0 {
		t.Fatal("valid release failed")
	}
	var info versioning.Info
	if err := json.Unmarshal([]byte(output), &info); err != nil || info.OverallBuildNumber != 1 || info.Version != "1.2.3+20260102.42" {
		t.Fatalf("release=%s %v", output, err)
	}
	if err := os.WriteFile(filepath.Join(repo, "dirty.txt"), []byte("change"), 0600); err != nil {
		t.Fatal(err)
	}
	releaseArgs[8] = "release-two"
	if _, code := invoke(releaseArgs...); code == 0 {
		t.Fatal("dirty release allocated")
	}
}
