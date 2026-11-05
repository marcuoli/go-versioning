package buildcounter

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	versioning "github.com/marcuoli/go-versioning"
)

const maximumRecordSize = 16 * 1024

// Exclusive creation avoids overwriting a reservation even after external state
// damage. Sync the complete file before acknowledging it. Interrupted writes
// remain visible and stop future allocation rather than silently reusing a number.
func writeNew(path string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return writeBytesNew(path, data)
}

func writeBytesNew(path string, data []byte) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(data)
	if writeErr == nil {
		writeErr = file.Sync()
	}
	return errors.Join(writeErr, file.Close())
}

func readJSON(path string, value any) error {
	stat, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !stat.Mode().IsRegular() || stat.Size() > maximumRecordSize {
		return errors.New("counter record must be a bounded regular file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if len(data) > maximumRecordSize {
		return errors.New("counter record exceeds supported size")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(value); err != nil {
		return fmt.Errorf("invalid counter record: %w", err)
	}
	var extra any
	if err = decoder.Decode(&extra); err != io.EOF {
		return errors.New("counter record has trailing data")
	}
	// State is immutable tool-written canonical JSON. Re-encoding rejects
	// duplicate keys and ambiguous alternate representations instead of silently
	// retaining only the decoder's last field. Preserve schema1 encoding on upgrade.
	canonical, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if !bytes.Equal(bytes.TrimSpace(data), canonical) {
		return errors.New("counter record is not canonical schema1 JSON")
	}
	return nil
}

func readHeader(path string) (header, error) {
	var h header
	if err := readJSON(filepath.Join(path, "counter.json"), &h); err != nil {
		return h, err
	}
	if h.Schema != 1 || !identityPattern.MatchString(h.Application) || !authorityPattern.MatchString(h.CounterID) || h.CreatedTime == "" {
		return h, errors.New("invalid counter authority metadata")
	}
	if _, err := versioning.Resolve(versioning.Info{BuildTime: h.CreatedTime}, versioning.Info{}); err != nil {
		return h, err
	}
	directory, err := os.Lstat(filepath.Join(path, "reservations"))
	if err != nil {
		return h, err
	}
	if !directory.IsDir() || directory.Mode()&os.ModeSymlink != 0 {
		return h, errors.New("counter reservations must be an actual directory")
	}
	return h, nil
}

func (s *Store) ledger(ctx context.Context) ([]Reservation, error) {
	entries, err := os.ReadDir(filepath.Join(s.path, "reservations"))
	if err != nil {
		return nil, err
	}
	issued, err := s.issued(ctx)
	if err != nil {
		return nil, err
	}
	if issued != uint64(len(entries)) {
		return nil, errors.New("issued sequence and reservation ledger disagree; preserve state for recovery")
	}
	records := make([]Reservation, 0, len(entries))
	requests := map[string]bool{}
	for index, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		number := uint64(index) + 1
		if entry.Name() != fmt.Sprintf("%020d.json", number) {
			return nil, errors.New("counter ledger has an unexpected entry or sequence gap")
		}
		var r Reservation
		if err := readJSON(filepath.Join(s.path, "reservations", entry.Name()), &r); err != nil {
			return nil, err
		}
		validated, err := versioning.Resolve(r.Info, versioning.Info{})
		if err != nil {
			return nil, err
		}
		if r.Application != s.application || r.Info.BuildCounterID != s.id || r.Info.OverallBuildNumber != number || !identityPattern.MatchString(r.RequestID) || requests[r.RequestID] || validated != r.Info {
			return nil, errors.New("counter ledger identity, request or metadata mismatch")
		}
		requests[r.RequestID] = true
		records = append(records, r)
	}
	return records, nil
}

// The independent append-only sequence is committed before the reservation.
// Losing either a record or sequence tail fails validation. A crash between
// these writes blocks allocation for recovery instead of reusing an issued ID.
func (s *Store) issue(number uint64) error {
	file, err := os.OpenFile(filepath.Join(s.path, "issued.log"), os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	data := []byte(fmt.Sprintf("%020d\n", number))
	n, writeErr := file.Write(data)
	if writeErr == nil && n != len(data) {
		writeErr = io.ErrShortWrite
	}
	if writeErr == nil {
		writeErr = file.Sync()
	}
	return errors.Join(writeErr, file.Close())
}

func (s *Store) issued(ctx context.Context) (uint64, error) {
	path := filepath.Join(s.path, "issued.log")
	stat, err := os.Lstat(path)
	if err != nil {
		return 0, err
	}
	if !stat.Mode().IsRegular() {
		return 0, errors.New("issued sequence must be a regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer file.Close()
	reader := bufio.NewReader(file)
	var last uint64
	for {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		// ReadSlice bounds malformed lines and avoids an unbounded corrupt record.
		data, err := reader.ReadSlice('\n')
		if err == io.EOF && len(data) == 0 {
			return last, nil
		}
		if err != nil {
			return 0, errors.New("issued sequence is incomplete or unreadable")
		}
		next, err := nextNumber(last)
		if err != nil {
			return 0, err
		}
		if string(data) != fmt.Sprintf("%020d\n", next) {
			return 0, errors.New("issued sequence is not canonical and contiguous")
		}
		last = next
	}
}
