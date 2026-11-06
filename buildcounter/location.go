package buildcounter

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// versionOwner is immutable operational ownership at the designated main file.
// Paths are authority-host-native. Alternate OS mounts must invoke that host,
// never invent a second path binding or copy a live store.
type versionOwner struct {
	Application   string `json:"application"`
	CounterID     string `json:"counter_id"`
	StorePath     string `json:"store_path"`
	VersionFile   string `json:"version_file"`
	LocatedNumber uint64 `json:"located_number"`
	HadVersion    bool   `json:"had_version"`
}

func (s *Store) owner(h header) versionOwner {
	return versionOwner{s.application, s.id, s.path, h.VersionFile, h.LocatedNumber, h.HadVersion}
}

// InitializeAt creates an authority whose numeric VERSION is in the designated
// permanent main root. The journal directory remains external. The caller of
// this filesystem API verifies that root; the CLI verifies primary Git main.
func InitializeAt(ctx context.Context, path, app, versionFile string) (*Store, error) {
	if versionFile == "" {
		return nil, errors.New("main VERSION path is required")
	}
	return initialize(ctx, path, app, nil, versionFile)
}

// InitializeSeedAt preserves explicit baseline continuity at the main VERSION.
func InitializeSeedAt(ctx context.Context, path, app, versionFile string, seed Seed) (*Store, error) {
	if versionFile == "" {
		return nil, errors.New("main VERSION path is required")
	}
	if e := seed.validate(); e != nil {
		return nil, e
	}
	return initialize(ctx, path, app, &seed, versionFile)
}

func canonicalVersionFile(file string) (string, error) {
	if !filepath.IsAbs(file) || filepath.Base(file) != "VERSION" || len(file) > 4096 {
		return "", errors.New("VERSION must be an absolute authority-host path named VERSION at permanent main")
	}
	parent, e := filepath.EvalSymlinks(filepath.Dir(file))
	if e != nil {
		return "", e
	}
	info, e := os.Stat(parent)
	if e != nil {
		return "", e
	}
	if !info.IsDir() {
		return "", errors.New("VERSION parent must be a directory")
	}
	return filepath.Join(parent, "VERSION"), nil
}
func separateLocation(store, file string) error {
	parent, e := filepath.EvalSymlinks(filepath.Dir(store))
	if e != nil {
		return e
	}
	store = filepath.Join(parent, filepath.Base(store))
	contains := func(a, b string) bool {
		rel, e := filepath.Rel(a, b)
		return e == nil && (rel == "." || rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
	}
	root := filepath.Dir(file)
	if contains(root, store) || contains(store, root) {
		return errors.New("journals must be external to the permanent main VERSION root")
	}
	return nil
}

// BoundVersionFile reports the persisted location without creating or allocating
// state. Empty means a legacy authority still needs explicit location migration.
func (s *Store) BoundVersionFile() (string, error) {
	if e := s.check(); e != nil {
		return "", e
	}
	h, e := readHeader(s.path)
	if e != nil {
		return "", e
	}
	if h.Schema < 3 {
		return "", nil
	}
	return h.VersionFile, nil
}
func (s *Store) checkOwner(h header) error {
	var owner versionOwner
	if e := readJSON(h.VersionFile+".authority.json", &owner); e != nil {
		return e
	}
	if owner != s.owner(h) {
		return errors.New("main VERSION belongs to a different authority or journal location")
	}
	return nil
}
func (s *Store) claimOwner(h header, resume bool) error {
	file := h.VersionFile + ".authority.json"
	if _, e := os.Lstat(file); e == nil {
		if !resume {
			return errors.New("main VERSION is already claimed; preserve its authority")
		}
		return s.checkOwner(h)
	} else if !errors.Is(e, os.ErrNotExist) {
		return e
	}
	return writeNew(file, s.owner(h))
}
func createNumber(file string, number uint64) error {
	if e := checkNumberFile(file, number, true); e != nil {
		return e
	}
	if _, e := os.Lstat(file); e == nil {
		return nil
	}
	return writeBytesNew(file, []byte(fmt.Sprintf("%d\n", number)))
}

// RelocateVersion changes only the file location. It never seeds or reserves.
// Header intent precedes dependent writes, blocking old allocators and all normal
// operations until complete. An exact same-target retry resumes known steps;
// corrupt/partial bytes remain evidence and require deliberate operator recovery.
func (s *Store) RelocateVersion(ctx context.Context, file string) (result Status, err error) {
	file, err = canonicalVersionFile(file)
	if err != nil {
		return result, err
	}
	if s == nil {
		return result, errors.New("counter store is not open")
	}
	if err = separateLocation(s.path, file); err != nil {
		return result, err
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
	if h.Schema == 3 && h.VersionFile != file {
		return result, errors.New("authority is bound to another main VERSION; alternate copies are forbidden")
	}
	if h.Schema == 3 && !h.VersionPending {
		records, e := s.ledger(ctx)
		if e != nil {
			return result, e
		}
		b, e := s.readBaseline(h)
		if e != nil {
			return result, e
		}
		last, e := lastNumber(b.Seed.Number, uint64(len(records)))
		return s.status(last, b, uint64(len(records)), h.VersionFile), e
	}
	var records []Reservation
	if h.Schema < 3 {
		records, err = s.ledger(ctx)
	} else {
		records, err = s.ledgerState(ctx, false)
	}
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
	if h.Schema < 3 {
		if err = checkNumberFile(filepath.Join(s.path, "VERSION"), last, h.Schema == 1); err != nil {
			return result, err
		}
		if err = checkNumberFile(file, last, true); err != nil {
			return result, err
		}
		if _, e := os.Lstat(file + ".authority.json"); !errors.Is(e, os.ErrNotExist) {
			return result, errors.New("target VERSION ownership exists or is unreadable; refusing to claim")
		}
		_, e := os.Lstat(filepath.Join(s.path, "VERSION"))
		h.HadVersion = e == nil
		h.LocatedNumber = last
		h.Schema = 3
		h.VersionFile = file
		h.JournalPath = s.path
		h.VersionPending = true
		if err = ctx.Err(); err != nil {
			return result, err
		}
		if err = replaceJSON(filepath.Join(s.path, "counter.json"), h); err != nil {
			return result, err
		}
	} else if last != h.LocatedNumber {
		return result, errors.New("journal changed during pending VERSION relocation")
	}
	// Once intent is published finish these bounded writes or preserve the actual
	// I/O error. Cancellation cannot roll back synced state or interrupt file sync.
	if err = s.claimOwner(h, true); err != nil {
		return result, err
	}
	if err = createNumber(file, last); err != nil {
		return result, err
	}
	if err = s.retireVersion(h); err != nil {
		return result, err
	}
	h.VersionPending = false
	if err = replaceJSON(filepath.Join(s.path, "counter.json"), h); err != nil {
		return result, err
	}
	if _, err = s.ledger(ctx); err != nil {
		return result, err
	}
	return s.status(last, b, uint64(len(records)), h.VersionFile), nil
}
func (s *Store) retireVersion(h header) error {
	old := filepath.Join(s.path, "VERSION")
	retired := filepath.Join(s.path, "VERSION.retired")
	if !h.HadVersion {
		for _, f := range []string{old, retired} {
			if _, e := os.Lstat(f); !errors.Is(e, os.ErrNotExist) {
				return errors.New("unexpected legacy VERSION evidence")
			}
		}
		return nil
	}
	if _, e := os.Lstat(old); errors.Is(e, os.ErrNotExist) {
		return checkNumberFile(retired, h.LocatedNumber, false)
	} else if e != nil {
		return e
	}
	if e := checkNumberFile(old, h.LocatedNumber, false); e != nil {
		return e
	}
	data, e := os.ReadFile(old)
	if e != nil {
		return e
	}
	if _, e = os.Lstat(retired); errors.Is(e, os.ErrNotExist) {
		if e = writeBytesNew(retired, data); e != nil {
			return e
		}
	} else if e != nil {
		return e
	}
	if e = checkNumberFile(retired, h.LocatedNumber, false); e != nil {
		return e
	}
	return os.Remove(old)
}
func (s *Store) checkLocation(h header, last uint64) error {
	if h.VersionPending {
		return errors.New("VERSION relocation is incomplete; retry counter-relocate with the same main root")
	}
	if e := s.checkOwner(h); e != nil {
		return e
	}
	if e := checkNumberFile(h.VersionFile, last, false); e != nil {
		return e
	}
	if _, e := os.Lstat(filepath.Join(s.path, "VERSION")); !errors.Is(e, os.ErrNotExist) {
		return errors.New("legacy live VERSION remains in journal; preserve relocation evidence")
	}
	if h.HadVersion {
		return checkNumberFile(filepath.Join(s.path, "VERSION.retired"), h.LocatedNumber, false)
	}
	return nil
}

// CheckLocation validates one shared placement/ownership/continuity convention
// without allocating or editing. The expected file must name real permanent main
// on the allocation host; the CLI derives/verifies that designation through Git.
func (s *Store) CheckLocation(ctx context.Context, expectedFile string) (Status, error) {
	expected, e := canonicalVersionFile(expectedFile)
	if e != nil {
		return Status{}, e
	}
	status, e := s.Status(ctx)
	if e != nil {
		return Status{}, e
	}
	if !status.LocationCompliant {
		return status, errors.New("legacy VERSION layout is not compliant; upgrade builders and run counter-relocate, without seeding or allocating")
	}
	if status.VersionFile != expected {
		return status, errors.New("VERSION is bound to a different main root; use the real --main-repo on the designated authority host, not a checkout copy")
	}
	return status, nil
}
