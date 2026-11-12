package buildcounter

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

const testCommit = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func newStore(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "counter")
	store, err := Initialize(context.Background(), path, "example-app")
	if err != nil {
		t.Fatal(err)
	}
	// Interrupted-header fixtures must use the physical path persisted by Open,
	// including when the host temporary directory has a filesystem alias.
	return store, store.path
}
func request(id string) Request { return Request{ID: id, Version: "0.0.0+unknown", Commit: testCommit} }

func TestExplicitInitializationAndPersistentAttemptSemantics(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing")
	if _, err := Open(path, "example-app"); err == nil {
		t.Fatal("missing store silently initialized")
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("read created store")
	}
	s, path := newStore(t)
	if _, err := Initialize(context.Background(), path, "example-app"); err == nil {
		t.Fatal("reset an existing authority")
	}
	if _, err := Open(path, "other-app"); err == nil {
		t.Fatal("accepted wrong application")
	}
	a, err := s.Reserve(context.Background(), request("attempt-1"))
	if err != nil || a.Info.OverallBuildNumber != 1 || a.Info.BuildCounterID == "" || a.Info.Commit != testCommit {
		t.Fatalf("first=%+v %v", a, err)
	}
	if _, err := time.Parse(time.RFC3339, a.Info.BuildTime); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path, "example-app")
	if err != nil {
		t.Fatal(err)
	}
	retry, err := reopened.Reserve(context.Background(), request("attempt-1"))
	if err != nil || retry != a {
		t.Fatalf("retry=%+v %v; first=%+v", retry, err, a)
	}
	changed := request("attempt-1")
	changed.Commit = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	if _, err := s.Reserve(context.Background(), changed); err == nil {
		t.Fatal("reused request for different inputs")
	}
	// A failed compiler invocation does not roll back its reservation.
	b, err := reopened.Reserve(context.Background(), request("attempt-2"))
	if err != nil || b.Info.OverallBuildNumber != 2 {
		t.Fatalf("next=%+v %v", b, err)
	}
	status, err := s.Status(context.Background())
	if err != nil || status.LastNumber != 2 || status.CounterID != a.Info.BuildCounterID {
		t.Fatalf("status=%+v %v", status, err)
	}
}

func TestConcurrentAllocatorsShareOneAuthority(t *testing.T) {
	_, path := newStore(t)
	const count = 24
	results := make(chan Reservation, count)
	failures := make(chan error, count)
	var wg sync.WaitGroup
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			s, err := Open(path, "example-app")
			if err != nil {
				failures <- err
				return
			}
			r, err := s.Reserve(context.Background(), request(time.Unix(int64(i), 0).Format(time.RFC3339)))
			if err != nil {
				failures <- err
				return
			}
			results <- r
		}(i)
	}
	wg.Wait()
	close(results)
	close(failures)
	for err := range failures {
		t.Error(err)
	}
	numbers := map[uint64]bool{}
	for r := range results {
		if numbers[r.Info.OverallBuildNumber] {
			t.Fatal("duplicate build number")
		}
		numbers[r.Info.OverallBuildNumber] = true
	}
	for i := uint64(1); i <= count; i++ {
		if !numbers[i] {
			t.Errorf("missing %d", i)
		}
	}
}

func TestAbandonedLockCancellationAndCorruptLedgerFailClosed(t *testing.T) {
	s, path := newStore(t)
	lock := filepath.Join(path, ".lock")
	if err := os.Mkdir(lock, 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := s.Reserve(ctx, request("waiting")); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("lock error=%v", err)
	}
	if _, err := os.Stat(lock); err != nil {
		t.Fatal("stole abandoned lock")
	}
	if err := os.Remove(lock); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Reserve(context.Background(), request("first")); err != nil {
		t.Fatal(err)
	}
	if err := s.issue(2); err != nil {
		t.Fatal(err)
	}
	record := filepath.Join(path, "reservations", "00000000000000000002.json")
	if err := os.WriteFile(record, []byte(`{"incomplete":`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Reserve(context.Background(), request("after-corruption")); err == nil {
		t.Fatal("ignored corrupt tail")
	}
	data, err := os.ReadFile(record)
	if err != nil || string(data) != `{"incomplete":` {
		t.Fatal("rewrote corrupt evidence")
	}
}

func TestLedgerIdentityAndSequenceCannotBeSilentlySkipped(t *testing.T) {
	for _, mode := range []string{"gap", "unknown-file", "mismatched-number", "wrong-authority", "missing-metadata"} {
		t.Run(mode, func(t *testing.T) {
			s, path := newStore(t)
			r, err := s.Reserve(context.Background(), request("one"))
			if err != nil {
				t.Fatal(err)
			}
			record := filepath.Join(path, "reservations", "00000000000000000001.json")
			switch mode {
			case "gap":
				if err := os.Rename(record, filepath.Join(path, "reservations", "00000000000000000002.json")); err != nil {
					t.Fatal(err)
				}
			case "unknown-file":
				if err := os.WriteFile(filepath.Join(path, "reservations", "ignored.txt"), []byte("evidence"), 0600); err != nil {
					t.Fatal(err)
				}
			case "mismatched-number":
				r.Info.OverallBuildNumber = 99
				data, _ := json.Marshal(r)
				if err := os.WriteFile(record, data, 0600); err != nil {
					t.Fatal(err)
				}
			case "wrong-authority":
				r.Info.BuildCounterID = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
				data, _ := json.Marshal(r)
				if err := os.WriteFile(record, data, 0600); err != nil {
					t.Fatal(err)
				}
			case "missing-metadata":
				if err := os.Remove(filepath.Join(path, "counter.json")); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := s.Reserve(context.Background(), request("two")); err == nil {
				t.Fatal("accepted inconsistent authority")
			}
		})
	}
}

func TestIndependentIssuedSequenceDetectsMissingTail(t *testing.T) {
	for _, mode := range []string{"missing-record", "missing-sequence", "truncated-sequence", "pending-issue"} {
		t.Run(mode, func(t *testing.T) {
			s, path := newStore(t)
			if _, err := s.Reserve(context.Background(), request("one")); err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "missing-record":
				if err := os.Remove(filepath.Join(path, "reservations", "00000000000000000001.json")); err != nil {
					t.Fatal(err)
				}
			case "missing-sequence":
				if err := os.Remove(filepath.Join(path, "issued.log")); err != nil {
					t.Fatal(err)
				}
			case "truncated-sequence":
				if err := os.WriteFile(filepath.Join(path, "issued.log"), nil, 0600); err != nil {
					t.Fatal(err)
				}
			case "pending-issue":
				if err := s.issue(2); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := s.Reserve(context.Background(), request("two")); err == nil {
				t.Fatal("reused an uncertain number")
			}
		})
	}
}
func TestCounterOverflowAndInvalidInputs(t *testing.T) {
	if _, err := nextNumber(^uint64(0)); err == nil {
		t.Fatal("counter wrapped")
	}
	if _, err := Initialize(nil, filepath.Join(t.TempDir(), "nil"), "app"); err == nil {
		t.Fatal("nil context accepted")
	}
	if _, err := Initialize(context.Background(), "relative", "app"); err == nil {
		t.Fatal("relative store accepted")
	}
	s, _ := newStore(t)
	if _, err := s.Reserve(nil, request("x")); err == nil {
		t.Fatal("nil reserve context accepted")
	}
	if _, err := s.Reserve(context.Background(), request("")); err == nil {
		t.Fatal("empty request accepted")
	}
}

func TestDuplicateAuthorityKeysAreNotSilentlyDiscarded(t *testing.T) {
	s, path := newStore(t)
	file := filepath.Join(path, "counter.json")
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	data = []byte(strings.Replace(string(data), `"schema":2`, `"schema":1,"schema":1`, 1))
	if err = os.WriteFile(file, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Reserve(context.Background(), request("bad-header")); err == nil {
		t.Fatal("silently discarded duplicate authority key")
	}
}
