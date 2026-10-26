package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestUsageErrors(t *testing.T) {
	for _, args := range [][]string{nil, {"missing"}, {"inspect"}, {"next", "--repo", "."}, {"tag", "--repo", ".", "--push"}, {"inspect", "--repo", ".", "extra"}, {"next", "--repo", ".", "--pr", "0"}} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			var out, errOut bytes.Buffer
			if code := run(context.Background(), args, &out, &errOut); code != 2 {
				t.Fatalf("code=%d; stdout=%s stderr=%s", code, out.String(), errOut.String())
			}
			if errOut.Len() == 0 {
				t.Fatal("missing usage error")
			}
		})
	}
}

func TestHelp(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"inspect", "--help"}, {"next", "--help"}, {"tag", "--help"}} {
		var out, errOut bytes.Buffer
		if code := run(context.Background(), args, &out, &errOut); code != 0 {
			t.Fatalf("%v: code=%d; stderr=%s", args, code, errOut.String())
		}
		if !strings.Contains(out.String(), "go-versioning") {
			t.Fatalf("missing help: %q", out.String())
		}
	}
}

func TestJSONUsageError(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run(context.Background(), []string{"next", "--json"}, &out, &errOut); code != 2 {
		t.Fatalf("code=%d", code)
	}
	var result struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(errOut.Bytes(), &result); err != nil || result.Error == "" {
		t.Fatalf("error=%v, output=%q", err, errOut.String())
	}
	if out.Len() != 0 {
		t.Fatalf("unexpected stdout: %q", out.String())
	}
}

func TestCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out, errOut bytes.Buffer
	if code := run(ctx, []string{"inspect", "--repo", t.TempDir()}, &out, &errOut); code != 1 {
		t.Fatalf("code=%d stderr=%q", code, errOut.String())
	}
}
