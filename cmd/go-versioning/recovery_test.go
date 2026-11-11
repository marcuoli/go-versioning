package main

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/marcuoli/go-versioning/buildcounter"
	"os"
	"path/filepath"
	"testing"
)

func TestCounterRecoverCLIRequiresExplicitMainAndApply(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "main")
	if err := os.Mkdir(repo, 0700); err != nil {
		t.Fatal(err)
	}
	cliGit(t, repo, "init", "-b", "main")
	cliGit(t, repo, "commit", "--allow-empty", "-m", "fixture")
	store := filepath.Join(root, "journal")
	file := filepath.Join(repo, "VERSION")
	s, err := buildcounter.InitializeAt(context.Background(), store, "example", file)
	if err != nil {
		t.Fatal(err)
	}
	commit := cliGit(t, repo, "rev-parse", "HEAD")
	reservation, err := s.Reserve(context.Background(), buildcounter.Request{ID: "one", Version: "0.0.0+unknown", Commit: commit})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(file, []byte("0\n"), 0600); err != nil {
		t.Fatal(err)
	}
	base := []string{"counter-recover", "--repo", repo, "--store", store, "--app", "example", "--counter-id", reservation.Info.BuildCounterID, "--expected-current", "0", "--expected-last", "1", "--json"}
	var out, stderr bytes.Buffer
	if code := run(context.Background(), base, &out, &stderr); code != 2 {
		t.Fatal("accepted implicit main", code, stderr.String())
	}
	missingCurrent := append(append([]string(nil), base[:9]...), base[11:]...)
	missingCurrent = append(missingCurrent, "--main-repo", repo)
	out.Reset()
	stderr.Reset()
	if code := run(context.Background(), missingCurrent, &out, &stderr); code != 2 {
		t.Fatal("accepted omitted expected-current", code, stderr.String())
	}
	args := append(base, "--main-repo", repo)
	out.Reset()
	stderr.Reset()
	if code := run(context.Background(), args, &out, &stderr); code != 0 {
		t.Fatal(code, stderr.String())
	}
	var plan buildcounter.Recovery
	if err = json.Unmarshal(out.Bytes(), &plan); err != nil || plan.Applied {
		t.Fatal(plan, err)
	}
	data, _ := os.ReadFile(file)
	if string(data) != "0\n" {
		t.Fatal("preview mutated VERSION")
	}
	args = append(args, "--apply")
	out.Reset()
	stderr.Reset()
	if code := run(context.Background(), args, &out, &stderr); code != 0 {
		t.Fatal(code, stderr.String())
	}
	data, _ = os.ReadFile(file)
	if string(data) != "1\n" {
		t.Fatal("apply did not recover")
	}
	cliGit(t, repo, "switch", "-c", "feature")
	out.Reset()
	stderr.Reset()
	if code := run(context.Background(), args, &out, &stderr); code == 0 {
		t.Fatal("accepted feature checkout as canonical main")
	}
}
