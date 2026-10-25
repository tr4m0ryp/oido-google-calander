// Package store persists connected Google accounts. Refresh tokens are
// sealed with crypt.Box before they touch disk; the file is written
// atomically with 0600 permissions.
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/tr4m0ryp/oido-google-calander/internal/auth/crypt"
)

const fileName = "accounts.json"

// ErrNotFound reports an unknown account.
var ErrNotFound = errors.New("account not connected")

// Account is the public metadata of a connected account (no secrets).
type Account struct {
	Email   string    `json:"email"`
	AddedAt time.Time `json:"added_at"`
}

type record struct {
	Account
	SealedToken string `json:"refresh_token"`
}

type fileFormat struct {
	Accounts []record `json:"accounts"`
}

// Accounts is a concurrency-safe, file-backed account registry.
type Accounts struct {
	mu   sync.RWMutex
	path string
	box  *crypt.Box
	recs map[string]record
}

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

func normalise(email string) string { return strings.ToLower(strings.TrimSpace(email)) }

// Put adds or replaces an account's refresh token.
func (s *Accounts) Put(email, refreshToken string) error {
	key := normalise(email)
	sealed, err := s.box.Seal(refreshToken, key)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	prev, existed := s.recs[key]
	s.recs[key] = record{Account: Account{Email: key, AddedAt: time.Now().UTC()}, SealedToken: sealed}
	if err := s.flush(); err != nil {
		if existed {
			s.recs[key] = prev
		} else {
			delete(s.recs, key)
		}
		return err
	}
	return nil
}

// Remove forgets an account.
func (s *Accounts) Remove(email string) error {
	key := normalise(email)
	s.mu.Lock()
	defer s.mu.Unlock()
	prev, ok := s.recs[key]
	if !ok {
		return ErrNotFound
	}
	delete(s.recs, key)
	if err := s.flush(); err != nil {
		s.recs[key] = prev
		return err
	}
	return nil
}

// RefreshToken returns the decrypted token for email.
func (s *Accounts) RefreshToken(email string) (string, error) {
	key := normalise(email)
	s.mu.RLock()
	r, ok := s.recs[key]
	s.mu.RUnlock()
	if !ok {
		return "", ErrNotFound
	}
	return s.box.Open(r.SealedToken, key)
}

// Has reports whether email is connected.
func (s *Accounts) Has(email string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.recs[normalise(email)]
	return ok
}

// List returns connected accounts ordered by connection time.
func (s *Accounts) List() []Account {
	s.mu.RLock()
	out := make([]Account, 0, len(s.recs))
	for _, r := range s.recs {
		out = append(out, r.Account)
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool {
		if out[i].AddedAt.Equal(out[j].AddedAt) {
			return out[i].Email < out[j].Email
		}
		return out[i].AddedAt.Before(out[j].AddedAt)
	})
	return out
}

// Len is the number of connected accounts.
func (s *Accounts) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.recs)
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
