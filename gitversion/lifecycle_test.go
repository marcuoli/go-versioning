package gitversion

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestGitPipeHelper uses finite-lived real child processes. Both helper modes
// explicitly exit, so a failed cancellation assertion cannot leave them running.
func TestGitPipeHelper(t *testing.T) {
	if os.Getenv("GO_VERSIONING_PIPE_HELPER") != "1" {
		return
	}
	marker := os.Getenv("GO_VERSIONING_PIPE_MARKER")
	if len(os.Args) > 0 && os.Args[len(os.Args)-1] == "child" {
		time.Sleep(2 * time.Second)
		if os.WriteFile(marker+".child-done", []byte("done"), 0600) != nil {
			os.Exit(3)
		}
		os.Exit(0)
	}
	child := exec.Command(os.Args[0], "-test.run=^TestGitPipeHelper$", "--", "child")
	child.Stdout = os.Stdout
	child.Stderr = os.Stderr
	if child.Start() != nil {
		os.Exit(3)
	}
	if os.WriteFile(marker+".started", []byte("started"), 0600) != nil {
		os.Exit(3)
	}
	err := child.Wait()
	if os.WriteFile(marker+".parent-done", []byte("done"), 0600) != nil || err != nil {
		os.Exit(3)
	}
	os.Exit(0)
}
func waitMarker(path string, limit time.Duration) bool {
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}
func TestCanceledGitDoesNotWaitForDescendantPipes(t *testing.T) {
	dir := t.TempDir()
	init := exec.Command("git", "init", "--bare", dir)
	init.Env = append(gitEnvironment(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1")
	if out, err := init.CombinedOutput(); err != nil {
		t.Fatalf("fixture init: %v %s", err, out)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	// Git aliases use Git's normal shell. Quote the executable as one shell word.
	alias := "!'" + strings.ReplaceAll(filepath.ToSlash(exe), "'", "'\"'\"'") + "' -test.run=^TestGitPipeHelper$ -- parent"
	marker := filepath.Join(t.TempDir(), "helper")
	t.Setenv("GO_VERSIONING_PIPE_HELPER", "1")
	t.Setenv("GO_VERSIONING_PIPE_MARKER", marker)
	r := &Repository{path: dir}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := r.run(ctx, "test pipe cancellation", "-c", "alias.pipe-helper="+alias, "pipe-helper")
		done <- err
	}()
	if !waitMarker(marker+".started", 5*time.Second) {
		t.Fatal("helper did not start")
	}
	t.Cleanup(func() {
		if !waitMarker(marker+".child-done", 5*time.Second) || !waitMarker(marker+".parent-done", 5*time.Second) {
			t.Error("finite-lived helpers did not finish")
		}
	})
	started := time.Now()
	cancel()
	err = <-done
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation error=%v", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("canceled Git waited for inherited pipes: %s", elapsed)
	}
}

func init() {
	if os.Getenv("GO_VERSIONING_CONSTRUCTOR_HELPER") != "1" {
		return
	}
	if os.WriteFile(os.Getenv("GO_VERSIONING_CONSTRUCTOR_MARKER"), []byte("started"), 0600) != nil {
		os.Exit(3)
	}
	// A direct helper, with no descendants, is killed by CommandContext. Its own
	// finite lifetime also protects a failed deadline implementation.
	time.Sleep(5 * time.Second)
	os.Exit(0)
}
func TestNewContextDeadlineBoundsGitValidation(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	source, err := os.Open(exe)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	tools := t.TempDir()
	name := "git"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	helper := filepath.Join(tools, name)
	destination, err := os.OpenFile(helper, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0755)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = io.Copy(destination, source); err != nil {
		destination.Close()
		t.Fatal(err)
	}
	if err = destination.Close(); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(t.TempDir(), "started")
	t.Setenv("PATH", tools+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("GO_VERSIONING_CONSTRUCTOR_HELPER", "1")
	t.Setenv("GO_VERSIONING_CONSTRUCTOR_MARKER", marker)
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err = NewContext(ctx, t.TempDir())
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("constructor deadline=%v", err)
	}
	if elapsed := time.Since(started); elapsed > 3*time.Second {
		t.Fatalf("constructor exceeded caller deadline: %s", elapsed)
	}
	if _, err = os.Stat(marker); err != nil {
		t.Fatal("Git validation helper did not run")
	}
}
