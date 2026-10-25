// Package store persists connected Google accounts. Refresh tokens are
// sealed with crypt.Box before they touch disk; the file is written
// atomically with 0600 permissions.
package store

import (
	"errors"
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
	// Scopes granted to the stored refresh token. Empty for accounts saved
	// before scopes were recorded; the provider backfills them on first use.
	Scopes []string `json:"scopes,omitempty"`
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

func normalise(email string) string { return strings.ToLower(strings.TrimSpace(email)) }

// Put adds an account, or updates it in place on reconnect: the email is
// the key, so the token and scopes are replaced and the original connection
// time is kept.
func (s *Accounts) Put(email, refreshToken string, scopes []string) error {
	key := normalise(email)
	sealed, err := s.box.Seal(refreshToken, key)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	prev, existed := s.recs[key]
	added := time.Now().UTC()
	if existed {
		added = prev.AddedAt
	}
	s.recs[key] = record{Account: Account{Email: key, AddedAt: added, Scopes: scopes}, SealedToken: sealed}
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

// SetScopes records the granted scopes of an existing account.
func (s *Accounts) SetScopes(email string, scopes []string) error {
	key := normalise(email)
	s.mu.Lock()
	defer s.mu.Unlock()
	prev, ok := s.recs[key]
	if !ok {
		return ErrNotFound
	}
	next := prev
	next.Scopes = append([]string(nil), scopes...)
	s.recs[key] = next
	if err := s.flush(); err != nil {
		s.recs[key] = prev
		return err
	}
	return nil
}

// Get returns one account's metadata.
func (s *Accounts) Get(email string) (Account, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.recs[normalise(email)]
	if !ok {
		return Account{}, false
	}
	a := r.Account
	a.Scopes = append([]string(nil), a.Scopes...)
	return a, true
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
