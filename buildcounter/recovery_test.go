package buildcounter

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func recoveryFixture(t *testing.T) (*Store, RecoveryOptions) {
	t.Helper()
	target := mainVersion(t)
	s, err := InitializeAt(context.Background(), filepath.Join(t.TempDir(), "journal"), "example-app", target)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"first", "second"} {
		if _, err = s.Reserve(context.Background(), request(id)); err != nil {
			t.Fatal(err)
		}
	}
	if err = os.WriteFile(target, []byte("1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	return s, RecoveryOptions{CounterID: s.id, VersionFile: target, CurrentNumber: 1, LastNumber: 2}
}

func TestRecoverVersionPreservesHistoryAndRetries(t *testing.T) {
	ctx := context.Background()
	s, options := recoveryFixture(t)
	before := map[string]string{}
	for _, name := range []string{"counter.json", "issued.log", "reservations/00000000000000000001.json", "reservations/00000000000000000002.json"} {
		data, err := os.ReadFile(filepath.Join(s.path, name))
		if err != nil {
			t.Fatal(err)
		}
		before[name] = string(data)
	}
	if _, err := s.Status(ctx); err == nil {
		t.Fatal("normal status accepted rollback")
	}
	preview, err := s.RecoverVersion(ctx, options)
	if err != nil || preview.Applied {
		t.Fatal(preview, err)
	}
	if _, err = os.Stat(preview.ReceiptFile); !os.IsNotExist(err) {
		t.Fatal("preview wrote receipt", err)
	}
	if err = checkNumberFile(options.VersionFile, 1, false); err != nil {
		t.Fatal(err)
	}
	options.Apply = true
	applied, err := s.RecoverVersion(ctx, options)
	if err != nil || !applied.Applied {
		t.Fatal(applied, err)
	}
	receipt, err := os.ReadFile(applied.ReceiptFile)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := s.RecoverVersion(ctx, options)
	if err != nil || !retry.AlreadyRecovered {
		t.Fatal(retry, err)
	}
	again, _ := os.ReadFile(applied.ReceiptFile)
	if string(again) != string(receipt) {
		t.Fatal("rewrote recovery receipt")
	}
	for name, want := range before {
		data, err := os.ReadFile(filepath.Join(s.path, name))
		if err != nil || string(data) != want {
			t.Fatal("changed allocation history", name, err)
		}
	}
	old, err := s.Reserve(ctx, request("first"))
	if err != nil || old.Info.OverallBuildNumber != 1 {
		t.Fatal(old, err)
	}
	next, err := s.Reserve(ctx, request("third"))
	if err != nil || next.Info.OverallBuildNumber != 3 {
		t.Fatal(next, err)
	}
	if _, err = s.RecoverVersion(ctx, options); err == nil {
		t.Fatal("accepted stale ledger expectation")
	}
}

func TestRecoverVersionRejectsUnverifiedState(t *testing.T) {
	for _, kind := range []string{"identity", "path", "current", "highwater", "ahead", "receipt-gap", "issued-tail", "owner", "pending", "legacy-live", "malformed", "canceled"} {
		t.Run(kind, func(t *testing.T) {
			s, o := recoveryFixture(t)
			o.Apply = true
			ctx := context.Background()
			switch kind {
			case "identity":
				o.CounterID = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
			case "path":
				o.VersionFile = mainVersion(t)
			case "current":
				o.CurrentNumber = 0
			case "highwater":
				o.LastNumber = 3
			case "ahead":
				os.WriteFile(o.VersionFile, []byte("3\n"), 0600)
				o.CurrentNumber = 3
			case "receipt-gap":
				os.Remove(filepath.Join(s.path, "reservations", "00000000000000000002.json"))
			case "issued-tail":
				os.WriteFile(filepath.Join(s.path, "issued.log"), []byte("00000000000000000001\n"), 0600)
			case "owner":
				os.Remove(o.VersionFile + ".authority.json")
			case "pending":
				h, _ := readHeader(s.path)
				h.VersionPending = true
				if err := replaceJSON(filepath.Join(s.path, "counter.json"), h); err != nil {
					t.Fatal(err)
				}
			case "legacy-live":
				os.WriteFile(filepath.Join(s.path, "VERSION"), []byte("1\n"), 0600)
			case "malformed":
				os.WriteFile(o.VersionFile, []byte("01\n"), 0600)
			case "canceled":
				c, cancel := context.WithCancel(ctx)
				cancel()
				ctx = c
			}
			before, _ := os.ReadFile(o.VersionFile)
			if _, err := s.RecoverVersion(ctx, o); err == nil {
				t.Fatal("accepted", kind)
			}
			after, _ := os.ReadFile(o.VersionFile)
			if string(before) != string(after) {
				t.Fatal("mutated rejected VERSION")
			}
		})
	}
}

func TestRecoverVersionResumesReceiptAfterPublicationFailure(t *testing.T) {
	s, o := recoveryFixture(t)
	ctx := context.Background()
	plan, err := s.RecoverVersion(ctx, o)
	if err != nil {
		t.Fatal(err)
	}
	receipt := recoveryReceipt{Application: s.application, CounterID: s.id, VersionFile: o.VersionFile, PreviousNumber: 1, LastNumber: 2, OriginalVersion: "1\n", CreatedTime: "2026-11-01T00:00:00Z"}
	if err = writeNew(plan.ReceiptFile, receipt); err != nil {
		t.Fatal(err)
	}
	original := o.VersionFile + ".original"
	if err = os.Rename(o.VersionFile, original); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(o.VersionFile, 0700); err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(o.VersionFile, "evidence")
	if err = os.WriteFile(child, []byte("preserve"), 0600); err != nil {
		t.Fatal(err)
	}
	// Exercise the real atomic publication failure after a synced receipt. Restore
	// only this isolated fixture's prior file, then retry the original operation.
	if err = s.replaceVersion(2); err == nil {
		t.Fatal("expected publication failure")
	}
	if err = os.Remove(child); err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(o.VersionFile); err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(original, o.VersionFile); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(plan.ReceiptFile)
	o.Apply = true
	recovered, err := s.RecoverVersion(ctx, o)
	if err != nil || !recovered.Applied || recovered.AlreadyRecovered {
		t.Fatal(recovered, err)
	}
	after, _ := os.ReadFile(plan.ReceiptFile)
	if string(before) != string(after) {
		t.Fatal("retry changed receipt")
	}
}

func TestRecoverVersionRejectsConflictingReceipt(t *testing.T) {
	s, o := recoveryFixture(t)
	plan, err := s.RecoverVersion(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(plan.ReceiptFile, []byte("{\"counter_id\":\"incomplete\"}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	o.Apply = true
	if _, err = s.RecoverVersion(context.Background(), o); err == nil {
		t.Fatal("accepted incomplete/conflicting receipt")
	}
	if err = checkNumberFile(o.VersionFile, 1, false); err != nil {
		t.Fatal(err)
	}
}
