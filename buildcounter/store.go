// Package buildcounter allocates build-attempt reservations in an explicitly
// initialized external store. Applications never allocate through runtime metadata.
package buildcounter

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"time"

	versioning "github.com/marcuoli/go-versioning"
)

// Store is one application authority/epoch. All cooperating builders must use
// the same live filesystem store; copies and cloud-sync replicas are independent.
type Store struct {
	path        string
	application string
	id          string
}

// Request identifies one logical build invocation. Reusing ID only retries the
// allocation operation; recompiling after a failure requires a new ID.
type Request struct {
	ID      string
	Version string
	Commit  string
}

// Reservation is immutable provenance for an allocated build attempt, including
// its original allocation timestamp. It does not claim compilation succeeded.
type Reservation struct {
	Application string          `json:"application"`
	RequestID   string          `json:"request_id"`
	Info        versioning.Info `json:"info"`
}

// Status describes the tracked sequence since explicit initialization, not a
// reconstructed lifetime count of historical builds.
type Status struct {
	Application string `json:"application"`
	CounterID   string `json:"counter_id"`
	LastNumber  uint64 `json:"last_number"`
}
type header struct {
	Schema      int    `json:"schema"`
	Application string `json:"application"`
	CounterID   string `json:"counter_id"`
	CreatedTime string `json:"created_time"`
}

var identityPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
var authorityPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

func inputs(path, app string) (string, error) {
	if !filepath.IsAbs(path) || !identityPattern.MatchString(app) {
		return "", errors.New("an absolute store path and stable application ID are required")
	}
	return filepath.Clean(path), nil
}
func contextError(ctx context.Context) error {
	if ctx == nil {
		return errors.New("context is required")
	}
	return ctx.Err()
}

// Initialize creates a new authority starting at1. The parent directory must
// exist. Existing or partially initialized paths are never reset or adopted.
func Initialize(ctx context.Context, path, application string) (*Store, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	path, err := inputs(path, application)
	if err != nil {
		return nil, err
	}
	if err = os.Mkdir(path, 0700); err != nil {
		return nil, fmt.Errorf("initialize new counter directory: %w", err)
	}
	// On initialization failure retain the incomplete directory for inspection;
	// silently deleting it could conceal an uncertain first publication.
	if err = os.Mkdir(filepath.Join(path, "reservations"), 0700); err != nil {
		return nil, err
	}
	if err = writeBytesNew(filepath.Join(path, "issued.log"), nil); err != nil {
		return nil, err
	}

	var entropy [16]byte
	if _, err = rand.Read(entropy[:]); err != nil {
		return nil, err
	}
	h := header{Schema: 1, Application: application, CounterID: hex.EncodeToString(entropy[:]), CreatedTime: time.Now().UTC().Format(time.RFC3339Nano)}
	if err = writeNew(filepath.Join(path, "counter.json"), h); err != nil {
		return nil, err
	}
	return Open(path, application)
}

// Open reads an existing authority without creating state. It cannot silently
// infer a baseline from Git, PRs or previously produced artifacts.
func Open(path, application string) (*Store, error) {
	path, err := inputs(path, application)
	if err != nil {
		return nil, err
	}
	root, err := filepath.EvalSymlinks(path)
	if err != nil {
		return nil, err
	}
	h, err := readHeader(root)
	if err != nil {
		return nil, err
	}
	if h.Application != application {
		return nil, errors.New("counter application identity does not match")
	}
	return &Store{path: root, application: application, id: h.CounterID}, nil
}

func (s *Store) check() error {
	if s == nil || s.path == "" {
		return errors.New("counter store is not open")
	}
	h, err := readHeader(s.path)
	if err != nil {
		return err
	}
	if h.CounterID != s.id || h.Application != s.application {
		return errors.New("counter authority changed; refusing to reset sequence")
	}
	return nil
}

func (s *Store) lock(ctx context.Context) (func() error, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	if err := s.check(); err != nil {
		return nil, err
	}
	lock := filepath.Join(s.path, ".lock")
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		err := os.Mkdir(lock, 0700)
		if err == nil {
			return func() error { return os.Remove(lock) }, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("acquire counter lock: %w", err)
		}
		// A PID, timeout or wall clock cannot prove another machine stopped writing.
		// Never steal a lock; crashed writers require deliberate operator recovery.
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}

// Reserve persists the next number before returning it. Failed compilation
// consumes the reservation. A repeated ID with matching source metadata returns
// the same original reservation; changing its source metadata is an error.
// Any error must stop the build. Partial write evidence is retained, not reused.
func (s *Store) Reserve(ctx context.Context, req Request) (result Reservation, err error) {
	if !identityPattern.MatchString(req.ID) {
		return result, errors.New("a bounded nonempty build request ID is required")
	}
	meta, err := versioning.Resolve(versioning.Info{Version: req.Version, Commit: req.Commit}, versioning.Info{})
	if err != nil {
		return result, err
	}
	if meta.Commit == "" {
		return result, errors.New("a full source commit is required")
	}
	unlock, err := s.lock(ctx)
	if err != nil {
		return result, err
	}
	defer func() {
		if e := unlock(); e != nil {
			err = errors.Join(err, fmt.Errorf("release counter lock: %w", e))
		}
	}()
	if err = s.check(); err != nil {
		return result, err
	}
	records, err := s.ledger(ctx)
	if err != nil {
		return result, err
	}
	for _, r := range records {
		if r.RequestID == req.ID {
			if r.Info.Version != meta.Version || r.Info.Commit != meta.Commit {
				return result, errors.New("build request ID already belongs to different source metadata")
			}
			return r, nil
		}
	}
	number, err := nextNumber(uint64(len(records)))
	if err != nil {
		return result, err
	}
	if err = ctx.Err(); err != nil {
		return result, err
	}
	meta.OverallBuildNumber = number
	meta.BuildCounterID = s.id
	meta.BuildTime = time.Now().UTC().Format(time.RFC3339Nano)
	result = Reservation{Application: s.application, RequestID: req.ID, Info: meta}
	if err = s.issue(number); err != nil {
		return result, err
	}
	if err = writeNew(filepath.Join(s.path, "reservations", fmt.Sprintf("%020d.json", number)), result); err != nil {
		return result, err
	}
	return result, nil
}

// Status takes the same exclusive lock as allocation to read a consistent ledger.
// Only this explicit build-time package accesses counter storage; runtime Current
// never calls it. Missing/corrupt entries are errors, not an incomplete prefix.
func (s *Store) Status(ctx context.Context) (result Status, err error) {
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
	return Status{Application: s.application, CounterID: s.id, LastNumber: uint64(len(records))}, nil
}
func nextNumber(last uint64) (uint64, error) {
	if last == math.MaxUint64 {
		return 0, errors.New("build counter exhausted uint64")
	}
	return last + 1, nil
}
