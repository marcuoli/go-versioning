package buildcounter

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	versioning "github.com/marcuoli/go-versioning"
)

// Seed is a deliberately verified one-time continuity baseline, not an inferred
// count of historical build attempts. Method is legacy or git-commits. Evidence
// describes the inspected legacy computation, or its absence for the Git fallback.
// Commit, Command and CheckedTime record the exact source and verification.
type Seed struct {
	Number      uint64 `json:"number"`
	Method      string `json:"method"`
	Commit      string `json:"commit"`
	Command     string `json:"command"`
	Evidence    string `json:"evidence"`
	CheckedTime string `json:"checked_time"`
}

type baseline struct {
	Seed          Seed   `json:"seed"`
	CounterID     string `json:"counter_id"`
	PriorAttempts uint64 `json:"prior_attempts"`
	AppliedTime   string `json:"applied_time"`
}

func (seed Seed) validate() error {
	if seed.Method != "legacy" && seed.Method != "git-commits" {
		return errors.New("seed method must be legacy or git-commits")
	}
	info, err := versioning.Resolve(versioning.Info{Commit: seed.Commit, BuildTime: seed.CheckedTime}, versioning.Info{})
	if err != nil || info.Commit == "" || seed.CheckedTime == "" || len(seed.CheckedTime) > 64 {
		return errors.New("seed requires full commit and RFC3339 check time")
	}
	for _, v := range []string{seed.Command, seed.Evidence} {
		if strings.TrimSpace(v) == "" || len(v) > 4096 || strings.ContainsRune(v, 0) {
			return errors.New("seed requires bounded command and investigation evidence")
		}
	}
	if seed.Method == "git-commits" && seed.Number == 0 {
		return errors.New("commit-count seed must be positive")
	}
	// JSON escaping may expand otherwise bounded text; prove the complete record
	// fits the reader before creating any authority or migration intent.
	_, err = recordBytes(baseline{Seed: seed, CounterID: strings.Repeat("0", 32), PriorAttempts: math.MaxUint64, AppliedTime: "2000-01-01T00:00:00.123456789Z"})
	return err
}

// InitializeSeed creates a new authority with an explicit evidence-backed seed.
// Initialize without a seed remains available for a deliberately new sequence.
// An I/O failure retains partial state for inspection and never resets a store.
func InitializeSeed(ctx context.Context, path, application string, seed Seed) (*Store, error) {
	if err := seed.validate(); err != nil {
		return nil, err
	}
	return initialize(ctx, path, application, &seed, "")
}

// Migrate adds a one-time baseline to every tracked attempt present under the
// authority lock. Earlier receipts and their retries retain original numbers.
// New numbers are baseline + total tracked attempts. The same Seed is idempotent
// even after further builds; changing it is forbidden. Stop old builders before
// upgrade: schema2 makes old allocators fail closed instead of ignoring the seed.
func (s *Store) Migrate(ctx context.Context, seed Seed) (result Status, err error) {
	if err = seed.validate(); err != nil {
		return result, err
	}
	unlock, err := s.lock(ctx)
	if err != nil {
		return result, err
	}
	defer func() { err = errors.Join(err, unlock()) }()
	if err = s.check(); err != nil {
		return result, err
	}
	records, err := s.ledger(ctx)
	if err != nil {
		return result, err
	}
	h, err := readHeader(s.path)
	if err != nil {
		return result, err
	}
	prior, err := s.readBaseline(h)
	if err != nil {
		return result, err
	}
	if h.BaselineSHA256 != "" {
		// Rechecking the same Git snapshot can produce a later check time. Keep
		// the original receipt/time; identity is all the other evidence fields.
		comparable := seed
		comparable.CheckedTime = prior.Seed.CheckedTime
		if prior.Seed != comparable {
			return result, errors.New("authority already has a different one-time baseline")
		}
		last, e := lastNumber(prior.Seed.Number, uint64(len(records)))
		return s.status(last, prior, uint64(len(records)), h.VersionFile), e
	}
	last, err := lastNumber(seed.Number, uint64(len(records)))
	if err != nil {
		return result, err
	}
	// A legacy external VERSION may have unrelated release semantics. Admit only
	// an absent file or the exact old numeric high-water; never overwrite identity.
	if h.Schema == 1 {
		if err = s.checkVersion(uint64(len(records)), true); err != nil {
			return result, err
		}
	}
	if err = ctx.Err(); err != nil {
		return result, err
	}
	receipt := baseline{Seed: seed, CounterID: s.id, PriorAttempts: uint64(len(records)), AppliedTime: time.Now().UTC().Format(time.RFC3339Nano)}
	data, err := recordBytes(receipt)
	if err != nil {
		return result, err
	}
	digest := sha256.Sum256(data)
	// Publish intent first. Old schema1 writers now reject the authority, and
	// current writers require this exact receipt plus matching VERSION. Any
	// interrupted step fails closed, even if a pre-upgrade process resumes.
	if h.Schema < 3 {
		h.Schema = 2
	}
	h.BaselineSHA256 = hex.EncodeToString(digest[:])
	if err = replaceJSON(filepath.Join(s.path, "counter.json"), h); err != nil {
		return result, err
	}
	if err = writeBytesNew(filepath.Join(s.path, "baseline.json"), data); err != nil {
		return result, err
	}
	if err = s.replaceVersion(last); err != nil {
		return result, err
	}

	return s.status(last, receipt, uint64(len(records)), h.VersionFile), nil
}
func (s *Store) status(last uint64, b baseline, attempts uint64, versionFile string) Status {
	status := Status{Application: s.application, CounterID: s.id, LastNumber: last, TrackedAttempts: attempts, VersionFile: versionFile, LocationCompliant: versionFile != ""}
	if b.Seed.Method != "" {
		seed := b.Seed
		status.Baseline = &seed
	}
	return status
}
func lastNumber(seed, attempts uint64) (uint64, error) {
	if attempts > math.MaxUint64-seed {
		return 0, errors.New("build baseline plus attempts exceeds uint64")
	}
	return seed + attempts, nil
}
func (b baseline) number(index uint64) (uint64, error) {
	if index <= b.PriorAttempts {
		return index, nil
	}
	return lastNumber(b.Seed.Number, index)
}

func fileDigest(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
func (s *Store) readBaseline(h header) (baseline, error) {
	var b baseline
	path := filepath.Join(s.path, "baseline.json")
	if h.BaselineSHA256 == "" {
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			return b, errors.New("unexpected baseline receipt; preserve incomplete migration for recovery")
		}
		return b, nil
	}
	if h.Schema != 2 && h.Schema != 3 {
		return b, errors.New("legacy header cannot contain a baseline")
	}
	if err := readJSON(path, &b); err != nil {
		return b, err
	}
	digest, err := fileDigest(path)
	if err != nil {
		return b, err
	}
	if digest != h.BaselineSHA256 {
		return b, errors.New("baseline receipt digest mismatch")
	}
	if b.CounterID != s.id || b.AppliedTime == "" {
		return b, errors.New("baseline authority or time mismatch")
	}
	if _, err = time.Parse(time.RFC3339, b.AppliedTime); err != nil {
		return b, err
	}
	return b, b.Seed.validate()
}
func (s *Store) checkVersion(last uint64, optional bool) error {
	h, err := readHeader(s.path)
	if err != nil {
		return err
	}
	path := filepath.Join(s.path, "VERSION")
	if h.Schema == 3 {
		path = h.VersionFile
	}
	return checkNumberFile(path, last, optional)
}
func checkNumberFile(path string, last uint64, optional bool) error {
	stat, err := os.Lstat(path)
	if optional && errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !stat.Mode().IsRegular() || stat.Size() > 21 {
		return errors.New("VERSION must be a bounded regular decimal-number file; preserve existing content")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if !bytes.Equal(data, []byte(fmt.Sprintf("%d\n", last))) {
		return errors.New("VERSION disagrees with authority history; preserve state for recovery")
	}
	return nil
}
func (s *Store) replaceVersion(last uint64) error {
	h, err := readHeader(s.path)
	if err != nil {
		return err
	}
	path := filepath.Join(s.path, "VERSION")
	if h.Schema == 3 {
		path = h.VersionFile
	}
	return replaceBytes(path, []byte(fmt.Sprintf("%d\n", last)))
}

// Stage a synced file beside its destination, then publish by rename under the
// authority lock. Supported filesystems must provide atomic replacement/visibility.
// A failed replacement retains the staged evidence and the caller stops building.
func replaceBytes(path string, data []byte) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".publish-*")
	if err != nil {
		return err
	}
	n, e := file.Write(data)
	if e == nil && n != len(data) {
		e = errors.New("short counter publication write")
	}
	if e == nil {
		e = file.Sync()
	}
	e = errors.Join(e, file.Close())
	if e != nil {
		return e
	}
	if err = os.Rename(file.Name(), path); err != nil {
		return err
	}
	return nil
}
