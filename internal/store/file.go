package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"

	"github.com/tr4m0ryp/oido-google-calander/internal/auth/crypt"
)

// Open loads (or initialises) the store in dir.
func Open(dir string, box *crypt.Box) (*Accounts, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	s := &Accounts{path: filepath.Join(dir, fileName), box: box, recs: map[string]record{}}
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read account store: %w", err)
	}
	var f fileFormat
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parse account store: %w", err)
	}
	for _, r := range f.Accounts {
		s.recs[normalise(r.Email)] = r
	}
	slog.Info("account store loaded", "accounts", len(s.recs))
	return s, nil
}

// flush writes the store atomically; callers hold the write lock.
func (s *Accounts) flush() error {
	f := fileFormat{Accounts: make([]record, 0, len(s.recs))}
	for _, r := range s.recs {
		f.Accounts = append(f.Accounts, r)
	}
	sort.Slice(f.Accounts, func(i, j int) bool { return f.Accounts[i].Email < f.Accounts[j].Email })
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".accounts-*.json")
	if err != nil {
		return fmt.Errorf("write account store: %w", err)
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write account store: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), s.path)
}
