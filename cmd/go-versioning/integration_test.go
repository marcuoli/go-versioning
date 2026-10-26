package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/marcuoli/go-versioning/gitversion"
)

func cliGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(strings.ToUpper(entry), "GIT_") {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	cmd.Env = append(cmd.Env, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+filepath.Join(dir, "empty-config"),
		"GIT_AUTHOR_NAME=Example", "GIT_AUTHOR_EMAIL=example@example.com", "GIT_COMMITTER_NAME=Example", "GIT_COMMITTER_EMAIL=example@example.com",
		"GIT_AUTHOR_DATE=2026-01-02T12:00:00Z", "GIT_COMMITTER_DATE=2026-01-02T12:00:00Z")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v: %s", args[0], err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestCLILocalPlanTagAndInspect(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("Git is required")
	}
	dir := t.TempDir()
	cliGit(t, dir, "init", "-b", "main")
	if err := os.WriteFile(filepath.Join(dir, "sample.txt"), []byte("synthetic fixture\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cliGit(t, dir, "add", "sample.txt")
	cliGit(t, dir, "commit", "-m", "Initial example")
	commit := cliGit(t, dir, "rev-parse", "HEAD")
	common := []string{"--repo", dir, "--branch", "main", "--commit", commit, "--pr", "42", "--initial-core", "1.2.3", "--json"}
	var out, errOut bytes.Buffer
	if code := run(context.Background(), append([]string{"next"}, common...), &out, &errOut); code != 0 {
		t.Fatalf("next=%d %s", code, errOut.String())
	}
	var plan gitversion.Plan
	if err := json.Unmarshal(out.Bytes(), &plan); err != nil {
		t.Fatal(err)
	}
	if plan.Version != "1.2.3+20260102.42" || plan.Commit != commit {
		t.Fatalf("plan=%+v", plan)
	}
	if tags := cliGit(t, dir, "tag", "--list"); tags != "" {
		t.Fatalf("dry run wrote tags: %q", tags)
	}
	out.Reset()
	errOut.Reset()
	if code := run(context.Background(), append([]string{"tag"}, common...), &out, &errOut); code != 0 {
		t.Fatalf("tag=%d %s", code, errOut.String())
	}
	var result gitversion.TagResult
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if !result.Local || !result.Created || result.Remote {
		t.Fatalf("result=%+v", result)
	}
	out.Reset()
	errOut.Reset()
	if code := run(context.Background(), []string{"inspect", "--repo", dir, "--json"}, &out, &errOut); code != 0 {
		t.Fatalf("inspect=%d %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "1.2.3-snapshot+20260102.42") {
		t.Fatalf("inspect output=%q", out.String())
	}
	out.Reset()
	errOut.Reset()
	if code := run(context.Background(), append([]string{"tag"}, common...), &out, &errOut); code != 0 {
		t.Fatalf("rerun=%d %s", code, errOut.String())
	}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Created || !result.Local {
		t.Fatalf("rerun=%+v", result)
	}
}

type brokenWriter struct{}

func (brokenWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestFailedOutputIsAnError(t *testing.T) {
	var errOut bytes.Buffer
	if code := run(context.Background(), []string{"--help"}, brokenWriter{}, &errOut); code != 1 {
		t.Fatalf("code=%d", code)
	}
}
