package buildcounter

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func mainVersion(t *testing.T) string {
	t.Helper()
	// Hand-built owners and recovery receipts must match the canonical paths
	// persisted by the API, including macOS's /var -> /private/var temp alias.
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(root, "VERSION")
}
func TestRelocatePreservesHistoryAndResumes(t *testing.T) {
	ctx := context.Background()
	s, p := newStore(t)
	seed := legacySeed()
	if _, e := s.Migrate(ctx, seed); e != nil {
		t.Fatal(e)
	}
	a, e := s.Reserve(ctx, request("one"))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Reserve(ctx, request("two")); e != nil {
		t.Fatal(e)
	}
	before := map[string]string{}
	for _, name := range []string{"issued.log", "baseline.json", "reservations/00000000000000000777.json", "reservations/00000000000000000778.json"} {
		b, e := os.ReadFile(filepath.Join(p, name))
		if e != nil {
			t.Fatal(e)
		}
		before[name] = string(b)
	}
	target := mainVersion(t)
	st, e := s.RelocateVersion(ctx, target)
	if e != nil || st.LastNumber != 778 || st.CounterID != a.Info.BuildCounterID {
		t.Fatal(st, e)
	}
	for name, data := range before {
		b, e := os.ReadFile(filepath.Join(p, name))
		if e != nil || string(b) != data {
			t.Fatal("changed history", name, e)
		}
	}
	if _, e = os.Stat(filepath.Join(p, "VERSION")); !os.IsNotExist(e) {
		t.Fatal("old live VERSION remains")
	}
	b, e := os.ReadFile(filepath.Join(p, "VERSION.retired"))
	if e != nil || string(b) != "778\n" {
		t.Fatal(string(b), e)
	}
	retry, e := s.Reserve(ctx, request("one"))
	if e != nil || retry != a {
		t.Fatal("changed old request", retry, e)
	}
	r, e := s.Reserve(ctx, request("three"))
	if e != nil || r.Info.OverallBuildNumber != 779 {
		t.Fatal(r, e)
	}
	st, e = s.RelocateVersion(ctx, target)
	if e != nil || st.LastNumber != 779 {
		t.Fatal(st, e)
	}
	if _, e = s.RelocateVersion(ctx, mainVersion(t)); e == nil {
		t.Fatal("silently moved bound authority again")
	}
	bound, e := s.BoundVersionFile()
	if e != nil || bound != target {
		t.Fatal(bound, e)
	}
}
func TestBoundInitializationOwnershipAndBaseline(t *testing.T) {
	ctx := context.Background()
	target := mainVersion(t)
	p := filepath.Join(t.TempDir(), "journal")
	s, e := InitializeSeedAt(ctx, p, "example-app", target, legacySeed())
	if e != nil {
		t.Fatal(e)
	}
	r, e := s.Reserve(ctx, request("first"))
	if e != nil || r.Info.OverallBuildNumber != 777 {
		t.Fatal(r, e)
	}
	other := filepath.Join(t.TempDir(), "other")
	otherSeed := legacySeed()
	otherSeed.Number = 777
	if _, e = InitializeSeedAt(ctx, other, "example-app", target, otherSeed); e == nil {
		t.Fatal("second store claimed VERSION")
	}
	if _, e = os.Stat(filepath.Join(p, "VERSION")); !os.IsNotExist(e) {
		t.Fatal("new live VERSION in journal")
	}
	// A seed added after location binding must preserve schema3 and external file.
	target2 := mainVersion(t)
	s2, e := InitializeAt(ctx, filepath.Join(t.TempDir(), "plain"), "example-app", target2)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s2.Reserve(ctx, request("before-seed")); e != nil {
		t.Fatal(e)
	}
	st, e := s2.Migrate(ctx, legacySeed())
	if e != nil || st.LastNumber != 777 {
		t.Fatal(st, e)
	}
	h, e := readHeader(s2.path)
	if e != nil || h.Schema != 3 {
		t.Fatal(h, e)
	}
	b, _ := os.ReadFile(target2)
	if string(b) != "777\n" {
		t.Fatal(string(b))
	}
}
func TestRelocationInterruptionAndConflict(t *testing.T) {
	ctx := context.Background()
	s, p := newStore(t)
	target := mainVersion(t)
	if e := os.WriteFile(target, []byte("1.2.3+20261105.9\n"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := s.RelocateVersion(ctx, target); e == nil {
		t.Fatal("overwrote release identity")
	}
	h, _ := readHeader(p)
	if h.Schema != 2 {
		t.Fatal("mutated on conflicting input")
	}
	os.Remove(target)
	// Simulate interruption after publication of the fully recorded intent.
	h.Schema = 3
	h.VersionFile = target
	h.JournalPath = p
	h.VersionPending = true
	h.HadVersion = true
	h.LocatedNumber = 0
	if e := replaceJSON(filepath.Join(p, "counter.json"), h); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Reserve(ctx, request("blocked")); e == nil {
		t.Fatal("allocated during incomplete relocation")
	}
	if _, e := s.Status(ctx); e == nil {
		t.Fatal("read incomplete relocation as complete")
	}
	st, e := s.RelocateVersion(ctx, target)
	if e != nil || st.LastNumber != 0 {
		t.Fatal(st, e)
	}
	// Removing owner evidence must stop allocation, not recreate ownership.
	if e = os.Remove(target + ".authority.json"); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Reserve(ctx, request("no-owner")); e == nil {
		t.Fatal("ignored missing binding")
	}
}
func TestConcurrentRelocationAndAllocation(t *testing.T) {
	ctx := context.Background()
	s, _ := newStore(t)
	target := mainVersion(t)
	var wg sync.WaitGroup
	fail := make(chan error, 16)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, e := s.Reserve(ctx, request(fmt.Sprintf("worker-%d", i)))
			if e != nil {
				info, statErr := os.Lstat(filepath.Join(s.path, ".lock"))
				t.Logf("lock failure raw=%#v stat=%v info=%v", e, statErr, info)
				fail <- e
			}
		}(i)
	}
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := s.RelocateVersion(ctx, target)
			if e != nil {
				info, statErr := os.Lstat(filepath.Join(s.path, ".lock"))
				t.Logf("lock failure raw=%#v stat=%v info=%v", e, statErr, info)
				fail <- e
			}
		}()
	}
	wg.Wait()
	close(fail)
	for e := range fail {
		t.Error(e)
	}
	st, e := s.Status(ctx)
	if e != nil || st.LastNumber != 12 {
		t.Fatal(st, e)
	}
}

func TestCopiedPendingRelocationCannotClaimMain(t *testing.T) {
	ctx := context.Background()
	s, p := newStore(t)
	target := mainVersion(t)
	h, e := readHeader(p)
	if e != nil {
		t.Fatal(e)
	}
	h.Schema = 3
	h.VersionFile = target
	h.JournalPath = p
	h.VersionPending = true
	h.HadVersion = true
	if e = replaceJSON(filepath.Join(p, "counter.json"), h); e != nil {
		t.Fatal(e)
	}
	copyPath := filepath.Join(t.TempDir(), "copy")
	if e = os.Mkdir(copyPath, 0700); e != nil {
		t.Fatal(e)
	}
	if e = os.Mkdir(filepath.Join(copyPath, "reservations"), 0700); e != nil {
		t.Fatal(e)
	}
	for _, name := range []string{"counter.json", "issued.log", "VERSION"} {
		data, e := os.ReadFile(filepath.Join(p, name))
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(filepath.Join(copyPath, name), data, 0600); e != nil {
			t.Fatal(e)
		}
	}
	if _, e = Open(copyPath, "example-app"); e == nil {
		t.Fatal("copied pending authority accepted")
	}
	if _, e = os.Stat(target + ".authority.json"); !os.IsNotExist(e) {
		t.Fatal("copy claimed target")
	}
	if _, e = s.RelocateVersion(ctx, target); e != nil {
		t.Fatal(e)
	}
}

func TestCompletedStepsResumeBeforeFinalHeader(t *testing.T) {
	ctx := context.Background()
	s, p := newStore(t)
	target := mainVersion(t)
	if _, e := s.RelocateVersion(ctx, target); e != nil {
		t.Fatal(e)
	}
	h, e := readHeader(p)
	if e != nil {
		t.Fatal(e)
	}
	h.VersionPending = true
	if e = replaceJSON(filepath.Join(p, "counter.json"), h); e != nil {
		t.Fatal(e)
	}
	st, e := s.RelocateVersion(ctx, target)
	if e != nil || st.LastNumber != 0 {
		t.Fatal(st, e)
	}
	if e = os.Remove(filepath.Join(p, "VERSION.retired")); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Reserve(ctx, request("missing-evidence")); e == nil {
		t.Fatal("lost retired evidence silently ignored")
	}
}

func TestSharedLocationCheckIsReadOnlyAndRejectsLegacy(t *testing.T) {
	ctx := context.Background()
	s, p := newStore(t)
	target := mainVersion(t)
	if _, e := s.CheckLocation(ctx, target); e == nil {
		t.Fatal("legacy layout passed compliance check")
	}
	st, e := s.Status(ctx)
	if e != nil || st.LastNumber != 0 || st.LocationCompliant {
		t.Fatal(st, e)
	}
	if _, e = s.RelocateVersion(ctx, target); e != nil {
		t.Fatal(e)
	}
	before, _ := os.ReadFile(filepath.Join(p, "issued.log"))
	st, e = s.CheckLocation(ctx, target)
	if e != nil || !st.LocationCompliant || st.TrackedAttempts != 0 {
		t.Fatal(st, e)
	}
	after, _ := os.ReadFile(filepath.Join(p, "issued.log"))
	if string(before) != string(after) {
		t.Fatal("check allocated")
	}
	if _, e = s.CheckLocation(ctx, mainVersion(t)); e == nil {
		t.Fatal("wrong primary root accepted")
	}
}

func TestMissingMainVersionRelocatesExisting27WithoutAllocation(t *testing.T) {
	ctx := context.Background()
	p := filepath.Join(t.TempDir(), "existing")
	seed := legacySeed()
	seed.Number = 26
	s, e := InitializeSeed(ctx, p, "example-app", seed)
	if e != nil {
		t.Fatal(e)
	}
	receipt, e := s.Reserve(ctx, request("existing-27"))
	if e != nil || receipt.Info.OverallBuildNumber != 27 {
		t.Fatal(receipt, e)
	}
	before := map[string]string{}
	for _, file := range []string{"issued.log", "baseline.json", "reservations/00000000000000000027.json"} {
		b, e := os.ReadFile(filepath.Join(p, file))
		if e != nil {
			t.Fatal(e)
		}
		before[file] = string(b)
	}
	target := mainVersion(t)
	if _, e = s.CheckLocation(ctx, target); e == nil {
		t.Fatal("legacy preflight should report relocation required")
	}
	if _, e = os.Stat(target); !os.IsNotExist(e) {
		t.Fatal("preflight created placeholder")
	}
	st, e := s.RelocateVersion(ctx, target)
	if e != nil || st.LastNumber != 27 || st.CounterID != receipt.Info.BuildCounterID || st.TrackedAttempts != 1 {
		t.Fatal(st, e)
	}
	for name, data := range before {
		b, e := os.ReadFile(filepath.Join(p, name))
		if e != nil || string(b) != data {
			t.Fatal("history changed", name, e)
		}
	}
	st, e = s.CheckLocation(ctx, target)
	if e != nil || st.LastNumber != 27 || st.TrackedAttempts != 1 {
		t.Fatal(st, e)
	}
	again, e := s.Reserve(ctx, request("existing-27"))
	if e != nil || again != receipt {
		t.Fatal("retry changed", again, e)
	}
	b, _ := os.ReadFile(target)
	if string(b) != "27\n" {
		t.Fatal(string(b))
	}
}
