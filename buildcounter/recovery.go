package buildcounter

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// RecoveryOptions pins operator-verified identity, location and numbers. Apply
// must be explicit. The caller must stop other writers, including cloud sync,
// and preserve the complete authority before recovery; a lock cannot fence them.
type RecoveryOptions struct {
	CounterID     string
	VersionFile   string
	CurrentNumber uint64
	LastNumber    uint64
	Apply         bool
}

// Recovery reports a verified repair plan or its application, without allocating.
type Recovery struct {
	CounterID        string `json:"counter_id"`
	VersionFile      string `json:"version_file"`
	PreviousNumber   uint64 `json:"previous_number"`
	LastNumber       uint64 `json:"last_number"`
	ReceiptFile      string `json:"receipt_file"`
	Applied          bool   `json:"applied"`
	AlreadyRecovered bool   `json:"already_recovered"`
}

type recoveryReceipt struct {
	Application     string `json:"application"`
	CounterID       string `json:"counter_id"`
	VersionFile     string `json:"version_file"`
	PreviousNumber  uint64 `json:"previous_number"`
	LastNumber      uint64 `json:"last_number"`
	OriginalVersion string `json:"original_version"`
	CreatedTime     string `json:"created_time"`
}

// RecoverVersion repairs only a lagging schema-3 VERSION from a complete ledger.
// All normal operations still fail closed on disagreement. A synced immutable
// receipt preserves the original bytes before publication; an exact retry can
// finish a failed publication, but never infer or truncate missing history.
func (s *Store) RecoverVersion(ctx context.Context, o RecoveryOptions) (result Recovery, err error) {
	file, err := canonicalVersionFile(o.VersionFile)
	if err != nil {
		return result, err
	}
	if s == nil || o.CounterID != s.id || o.CurrentNumber >= o.LastNumber {
		return result, errors.New("recovery requires the expected authority and a strictly lagging VERSION")
	}
	unlock, err := s.lock(ctx)
	if err != nil {
		return result, err
	}
	defer func() { err = errors.Join(err, unlock()) }()
	h, err := readHeader(s.path)
	if err != nil {
		return result, err
	}
	if h.Schema != 3 || h.VersionFile != file {
		return result, errors.New("recovery requires the bound permanent main VERSION")
	}
	if err = s.checkLocationEvidence(h); err != nil {
		return result, err
	}
	records, err := s.ledgerState(ctx, false)
	if err != nil {
		return result, err
	}
	b, err := s.readBaseline(h)
	if err != nil {
		return result, err
	}
	last, err := lastNumber(b.Seed.Number, uint64(len(records)))
	if err != nil {
		return result, err
	}
	if last != o.LastNumber {
		return result, errors.New("recovery high-water expectation differs from complete authority history")
	}
	result = Recovery{CounterID: s.id, VersionFile: file, PreviousNumber: o.CurrentNumber, LastNumber: last,
		ReceiptFile: filepath.Join(s.path, fmt.Sprintf("version-recovery-%d-%d.json", o.CurrentNumber, last))}
	want := recoveryReceipt{Application: s.application, CounterID: s.id, VersionFile: file, PreviousNumber: o.CurrentNumber,
		LastNumber: last, OriginalVersion: fmt.Sprintf("%d\n", o.CurrentNumber)}
	var receipt recoveryReceipt
	exists := false
	if _, e := os.Lstat(result.ReceiptFile); e == nil {
		if err = readJSON(result.ReceiptFile, &receipt); err != nil {
			return result, err
		}
		stamp := receipt.CreatedTime
		receipt.CreatedTime = ""
		if receipt != want {
			return result, errors.New("recovery receipt conflicts with expected repair; preserve evidence")
		}
		if _, err = time.Parse(time.RFC3339, stamp); err != nil {
			return result, err
		}
		exists = true
	} else if !errors.Is(e, os.ErrNotExist) {
		return result, e
	}
	if checkNumberFile(file, last, false) == nil {
		if !exists {
			return result, errors.New("VERSION already advanced without this recovery receipt")
		}
		result.AlreadyRecovered = true
		result.Applied = o.Apply
		return result, nil
	}
	if err = checkNumberFile(file, o.CurrentNumber, false); err != nil {
		return result, err
	}
	if !o.Apply {
		return result, nil
	}
	if err = ctx.Err(); err != nil {
		return result, err
	}
	if !exists {
		want.CreatedTime = time.Now().UTC().Format(time.RFC3339)
		if err = writeNew(result.ReceiptFile, want); err != nil {
			return result, err
		}
	}
	// A previous attempt may have left complete receipt bytes after Sync/Close
	// failed. Reading valid JSON alone does not prove its durability; sync again
	// before allowing the VERSION publication on an exact retry.
	if exists {
		receiptFile, e := os.OpenFile(result.ReceiptFile, os.O_RDWR, 0)
		if e != nil {
			return result, e
		}
		if e = errors.Join(receiptFile.Sync(), receiptFile.Close()); e != nil {
			return result, e
		}
	}
	// Recheck after recording intent: unexpected non-cooperating writers must not
	// be silently overwritten. Operators still need to fence cloud-sync clients.
	if err = checkNumberFile(file, o.CurrentNumber, false); err != nil {
		return result, err
	}
	if err = s.replaceVersion(last); err != nil {
		return result, err
	}
	if err = s.checkLocation(h, last); err != nil {
		return result, err
	}
	result.Applied = true
	return result, nil
}
