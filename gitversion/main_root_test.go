package gitversion_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestCounterTrackingPolicyPreservesCaseVariantLegacyFile(t *testing.T) {
	dir, r, _ := repo(t)
	file := filepath.Join(dir, "version")
	if e := os.WriteFile(file, []byte("legacy release identity\n"), 0600); e != nil {
		t.Fatal(e)
	}
	git(t, dir, "add", "version")
	git(t, dir, "commit", "-m", "legacy lowercase identity")
	if e := r.CheckCounterFiles(context.Background()); e == nil {
		t.Fatal("case-variant tracked legacy file admitted")
	}
	data, e := os.ReadFile(file)
	if e != nil || string(data) != "legacy release identity\n" {
		t.Fatal("legacy source changed", e)
	}
}
