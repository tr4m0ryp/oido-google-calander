package account

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"golang.org/x/oauth2"
	gcal "google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"
	gtasks "google.golang.org/api/tasks/v1"

	"github.com/tr4m0ryp/oido-google-calander/internal/calendar"
	"github.com/tr4m0ryp/oido-google-calander/internal/store"
)

// cached keeps one token source per account so access tokens are reused
// until they expire instead of being refreshed on every call. Calendar and
// Tasks share it; the Tasks service is built on first use.
type cached struct {
	refreshToken string
	ts           oauth2.TokenSource
	cal          *gcal.Service
	tasks        *gtasks.Service
}

// OAuth serves accounts connected through the /setup consent flow.
type OAuth struct {
	store       *store.Accounts
	cfg         *oauth2.Config
	defaultZone *time.Location

	// SetupURL is quoted in errors that ask the user to reconnect.
	SetupURL string
	// tokenInfoURL and httpClient are overridable in tests.
	tokenInfoURL string
	httpClient   *http.Client

	mu    sync.Mutex
	cache map[string]*cached
}

// NewOAuth builds a provider over the encrypted account store.
func NewOAuth(s *store.Accounts, cfg *oauth2.Config, defaultZone *time.Location) *OAuth {
	return &OAuth{store: s, cfg: cfg, defaultZone: defaultZone, cache: map[string]*cached{},
		tokenInfoURL: tokenInfoURL, httpClient: http.DefaultClient}
}

// Emails lists connected accounts.
func (o *OAuth) Emails() []string {
	accounts := o.store.List()
	out := make([]string, len(accounts))
	for i, a := range accounts {
		out[i] = a.Email
	}
	return out
}

// Forget drops the cached client after an account is removed or reconnected.
func (o *OAuth) Forget(email string) {
	o.mu.Lock()
	delete(o.cache, email)
	o.mu.Unlock()
}

// entry returns the account's cache entry, rebuilding it when the stored
// refresh token changed (a reconnect). Callers hold o.mu.
func (o *OAuth) entry(email string) (*cached, error) {
	token, err := o.store.RefreshToken(email)
	if err != nil {
		return nil, fmt.Errorf("account %s: %w", email, err)
	}
	c, ok := o.cache[email]
	if !ok || c.refreshToken != token {
		// The token source outlives this request, so it must not inherit ctx.
		src := o.cfg.TokenSource(context.Background(), &oauth2.Token{RefreshToken: token})
		c = &cached{refreshToken: token, ts: oauth2.ReuseTokenSource(nil, src)}
		o.cache[email] = c
	}
	return c, nil
}

// Client returns a calendar client for email.
func (o *OAuth) Client(_ context.Context, email string) (*calendar.Client, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	c, err := o.entry(email)
	if err != nil {
		return nil, err
	}
	if c.cal == nil {
		svc, err := gcal.NewService(context.Background(), option.WithTokenSource(c.ts))
		if err != nil {
			return nil, fmt.Errorf("calendar service for %s: %w", email, err)
		}
		c.cal = svc
	}
	return calendar.NewClient(c.cal, email, calendar.Options{DefaultZone: o.defaultZone}), nil
}

// tokenSource exposes an account's shared token source.
func (o *OAuth) tokenSource(email string) (oauth2.TokenSource, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	c, err := o.entry(email)
	if err != nil {
		return nil, err
	}
	return c.ts, nil
}
