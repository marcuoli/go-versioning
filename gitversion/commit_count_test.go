package gitversion_test

import (
	"context"
	"github.com/marcuoli/go-versioning/gitversion"
	"testing"
)

func TestCommitCountCanonicalReachableGraph(t *testing.T) {
	dir, r, root := repo(t)
	git(t, dir, "checkout", "-b", "unrelated")
	git(t, dir, "commit", "--allow-empty", "-m", "unrelated one")
	git(t, dir, "commit", "--allow-empty", "-m", "unrelated two")
	git(t, dir, "checkout", "main")
	git(t, dir, "checkout", "-b", "side")
	git(t, dir, "commit", "--allow-empty", "-m", "side change")
	git(t, dir, "checkout", "main")
	git(t, dir, "commit", "--allow-empty", "-m", "main change")
	git(t, dir, "merge", "--no-ff", "side", "-m", "merge")
	evidence, err := r.CommitCount(context.Background(), "main")
	if err != nil || evidence.Count != 4 || evidence.Commit != git(t, dir, "rev-parse", "HEAD") || evidence.Command == "" || evidence.CheckedTime == "" {
		t.Fatalf("%+v %v", evidence, err)
	}
	git(t, dir, "checkout", "--detach", root)
	if _, err = r.CommitCount(context.Background(), "main"); err == nil {
		t.Fatal("accepted noncanonical HEAD")
	}
	git(t, dir, "checkout", "main")
	remote := bare(t, dir)
	shallow := t.TempDir()
	git(t, shallow, "clone", "--depth=1", "--branch=main", "file://"+remote, ".")
	sr, err := gitversion.New(shallow)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = sr.CommitCount(context.Background(), "main"); err == nil {
		t.Fatal("accepted shallow seed")
	}
	if _, err = r.CommitCount(nil, "main"); err == nil {
		t.Fatal("accepted nil context")
	}
}
