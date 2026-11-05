package buildcounter

import (
	"context"
	"fmt"
	versioning "github.com/marcuoli/go-versioning"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func legacySeed() Seed {
	return Seed{Number: 776, Method: "legacy", Commit: testCommit, Command: "verified legacy hook and VERSION history", Evidence: "retained legacy evidence digest", CheckedTime: "2026-11-05T20:00:00Z"}
}
func oldStore(t *testing.T) (*Store, string) {
	t.Helper()
	s, p := newStore(t)
	// A schema1 fixture is the released encoding; no VERSION/baseline existed.
	h, e := readHeader(p)
	if e != nil {
		t.Fatal(e)
	}
	h.Schema = 1
	b := filepath.Join(p, "counter.json")
	if e = os.Remove(b); e != nil {
		t.Fatal(e)
	}
	if e = writeNew(b, h); e != nil {
		t.Fatal(e)
	}
	if e = os.Remove(filepath.Join(p, "VERSION")); e != nil {
		t.Fatal(e)
	}
	return s, p
}
func TestMigratePreservesReceiptsAndAddsAllCurrentAttempts(t *testing.T) {
	ctx := context.Background()
	s, p := oldStore(t)
	a, e := s.Reserve(ctx, request("one"))
	if e != nil {
		t.Fatal(e)
	}
	// Preparation precedes another real build; migrate must count it under lock.
	seed := legacySeed()
	_, e = s.Reserve(ctx, request("two"))
	if e != nil {
		t.Fatal(e)
	}
	before, e := os.ReadFile(filepath.Join(p, "reservations", "00000000000000000001.json"))
	if e != nil {
		t.Fatal(e)
	}
	st, e := s.Migrate(ctx, seed)
	if e != nil || st.LastNumber != 778 {
		t.Fatalf("migrate %+v %v", st, e)
	}
	after, _ := os.ReadFile(filepath.Join(p, "reservations", "00000000000000000001.json"))
	if string(before) != string(after) {
		t.Fatal("rewrote historical receipt")
	}
	retry, e := s.Reserve(ctx, request("one"))
	if e != nil || retry != a {
		t.Fatalf("changed old retry: %+v %v", retry, e)
	}
	r, e := s.Reserve(ctx, request("three"))
	if e != nil || r.Info.OverallBuildNumber != 779 {
		t.Fatalf("next %+v %v", r, e)
	}
	st, e = s.Migrate(ctx, seed)
	if e != nil || st.LastNumber != 779 {
		t.Fatalf("migration retry %+v %v", st, e)
	}
	seed.Number++
	if _, e = s.Migrate(ctx, seed); e == nil {
		t.Fatal("changed one-time baseline")
	}
	data, _ := os.ReadFile(filepath.Join(p, "VERSION"))
	if string(data) != "779\n" {
		t.Fatalf("VERSION %q", data)
	}
	reopened, e := Open(p, "example-app")
	if e != nil {
		t.Fatal(e)
	}
	st, e = reopened.Status(ctx)
	if e != nil || st.LastNumber != 779 {
		t.Fatal(st, e)
	}
	h, e := readHeader(p)
	if e != nil || h.Schema != 2 {
		t.Fatal("old writers would accept migrated header", h, e)
	}
}
func TestSeedInitializationAndFailClosedState(t *testing.T) {
	for _, mode := range []string{"missing-version", "wrong-version", "full-release-version", "missing-baseline", "changed-baseline", "partial-migration", "overflow"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			p := filepath.Join(t.TempDir(), "seeded")
			seed := legacySeed()
			if mode == "overflow" {
				seed.Number = math.MaxUint64
			}
			s, e := InitializeSeed(ctx, p, "example-app", seed)
			if e != nil {
				t.Fatal(e)
			}
			switch mode {
			case "missing-version":
				e = os.Remove(filepath.Join(p, "VERSION"))
			case "wrong-version":
				e = os.WriteFile(filepath.Join(p, "VERSION"), []byte("775\n"), 0600)
			case "full-release-version":
				e = os.WriteFile(filepath.Join(p, "VERSION"), []byte("1.2.3+20261105.8\n"), 0600)
			case "missing-baseline":
				e = os.Remove(filepath.Join(p, "baseline.json"))
			case "changed-baseline":
				data, _ := os.ReadFile(filepath.Join(p, "baseline.json"))
				e = os.WriteFile(filepath.Join(p, "baseline.json"), append(data, ' '), 0600)
			case "partial-migration":
				h, _ := readHeader(p)
				h.BaselineSHA256 = ""
				e = os.Remove(filepath.Join(p, "counter.json"))
				if e == nil {
					e = writeNew(filepath.Join(p, "counter.json"), h)
				}
			}
			if e != nil {
				t.Fatal(e)
			}
			if _, e = s.Reserve(ctx, request("blocked")); e == nil {
				t.Fatal("allocated through corrupt/exhausted state")
			}
		})
	}
}
func TestSeedValidationAndConcurrentMigration(t *testing.T) {
	ctx := context.Background()
	bad := legacySeed()
	bad.Evidence = ""
	p := filepath.Join(t.TempDir(), "bad")
	if _, e := InitializeSeed(ctx, p, "example-app", bad); e == nil {
		t.Fatal("missing evidence accepted")
	}
	if _, e := os.Stat(p); !os.IsNotExist(e) {
		t.Fatal("invalid input created state")
	}
	s, p := newStore(t)
	var wg sync.WaitGroup
	fail := make(chan error, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := s.Migrate(ctx, legacySeed())
			if e != nil {
				fail <- e
			}
		}()
	}
	wg.Wait()
	close(fail)
	for e := range fail {
		t.Error(e)
	}
	r, e := s.Reserve(ctx, request("new"))
	if e != nil || r.Info.OverallBuildNumber != 777 {
		t.Fatal(r, e)
	}
	// A full release string in an old authority's VERSION is preserved, never overwritten.
	old, q := oldStore(t)
	file := filepath.Join(q, "VERSION")
	os.WriteFile(file, []byte("1.2.3+20261105.8\n"), 0600)
	if _, e = old.Migrate(ctx, legacySeed()); e == nil {
		t.Fatal("overwrote existing VERSION")
	}
	b, _ := os.ReadFile(file)
	if string(b) != "1.2.3+20261105.8\n" {
		t.Fatal("lost old VERSION")
	}
	_ = p
}

func TestMigrationWithConcurrentBuildersAndStaleVersion(t *testing.T) {
	ctx := context.Background()
	s, p := oldStore(t)
	const builders = 12
	var wg sync.WaitGroup
	fail := make(chan error, builders+1)
	for i := 0; i < builders; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, e := s.Reserve(ctx, request(fmt.Sprintf("during-%d", i)))
			if e != nil {
				fail <- e
			}
		}(i)
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, e := s.Migrate(ctx, legacySeed())
		if e != nil {
			fail <- e
		}
	}()
	wg.Wait()
	close(fail)
	for e := range fail {
		t.Error(e)
	}
	st, e := s.Status(ctx)
	if e != nil || st.LastNumber != 776+builders || st.TrackedAttempts != builders {
		t.Fatal(st, e)
	}
	// Simulate interruption after durable issuance/receipt but before VERSION.
	n, _ := nextNumber(st.LastNumber)
	if e = s.issue(n); e != nil {
		t.Fatal(e)
	}
	r := Reservation{Application: s.application, RequestID: "interrupted", Info: versioning.Info{Version: "0.0.0+unknown", Commit: testCommit, BuildCounterID: s.id, OverallBuildNumber: n, BuildTime: time.Now().UTC().Format(time.RFC3339Nano)}}
	if e = writeNew(filepath.Join(p, "reservations", fmt.Sprintf("%020d.json", n)), r); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Status(ctx); e == nil {
		t.Fatal("partial issuance silently adopted")
	}
}

func TestFailedVersionReplacementPreservesDestinationAndBlocksAllocation(t *testing.T) {
	ctx := context.Background()
	s, p := newStore(t)
	// A nonempty destination directory forces atomic publication failure. Validate
	// first, then simulate the failure through the same staged publication primitive.
	if e := os.Remove(filepath.Join(p, "VERSION")); e != nil {
		t.Fatal(e)
	}
	if e := os.Mkdir(filepath.Join(p, "VERSION"), 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(p, "VERSION", "evidence"), []byte("retain"), 0600); e != nil {
		t.Fatal(e)
	}
	if e := s.replaceVersion(1); e == nil {
		t.Fatal("publication unexpectedly replaced a directory")
	}
	if _, e := s.Reserve(ctx, request("blocked")); e == nil {
		t.Fatal("continued after damaged VERSION")
	}
}

func TestEscapedSeedSizeRejectedBeforeMutation(t *testing.T) {
	ctx := context.Background()
	seed := legacySeed()
	seed.Command = strings.Repeat("&", 4096)
	seed.Evidence = seed.Command
	path := filepath.Join(t.TempDir(), "too-large")
	if _, err := InitializeSeed(ctx, path, "example-app", seed); err == nil {
		t.Fatal("oversized encoded receipt admitted")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("invalid seed created authority")
	}
	store, p := newStore(t)
	before, _ := os.ReadFile(filepath.Join(p, "counter.json"))
	if _, err := store.Migrate(ctx, seed); err == nil {
		t.Fatal("oversized migration admitted")
	}
	after, _ := os.ReadFile(filepath.Join(p, "counter.json"))
	if string(before) != string(after) {
		t.Fatal("invalid seed mutated header")
	}
	if _, err := store.Reserve(ctx, request("still-usable")); err != nil {
		t.Fatal(err)
	}
}
