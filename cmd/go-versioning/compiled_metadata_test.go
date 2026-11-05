package main

import (
	"encoding/json"
	"math"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	versioning "github.com/marcuoli/go-versioning"
)

func TestCompiledSharedLinkerMetadataWithoutRepositoryOrStore(t *testing.T) {
	_, source, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(source), "../.."))
	expected := versioning.Info{Version: versioning.UnknownVersion, Commit: strings.Repeat("a", 40), BuildTime: "2026-01-01T12:00:00Z", OverallBuildNumber: math.MaxUint64, BuildCounterID: "0123456789abcdef0123456789abcdef"}
	flags, err := expected.LinkerFlags()
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	binary := filepath.Join(directory, "metadata")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	compile := exec.Command("go", "build", "-trimpath", "-buildvcs=false", "-ldflags", flags, "-o", binary, "./examples/metadata")
	compile.Dir = root
	if output, err := compile.CombinedOutput(); err != nil {
		t.Fatalf("compile: %v %s", err, output)
	}
	run := exec.Command(binary)
	run.Dir = t.TempDir()
	run.Env = []string{"PATH="}
	output, err := run.CombinedOutput()
	if err != nil {
		t.Fatalf("run: %v %s", err, output)
	}
	var got versioning.Info
	if err = json.Unmarshal(output, &got); err != nil || got != expected {
		t.Fatalf("runtime=%+v err=%v output=%s", got, err, output)
	}
}
