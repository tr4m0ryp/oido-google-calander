package account

import (
	"context"
	"fmt"
	"sync"
	"time"

	"golang.org/x/oauth2"
	gcal "google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"

	"github.com/tr4m0ryp/oido-google-calander/internal/calendar"
	"github.com/tr4m0ryp/oido-google-calander/internal/store"
)

// cached keeps one token source per account so access tokens are reused
// until they expire instead of being refreshed on every call.
type cached struct {
	refreshToken string
	svc          *gcal.Service
}

// OAuth serves accounts connected through the /setup consent flow.
type OAuth struct {
	store       *store.Accounts
	cfg         *oauth2.Config
	defaultZone *time.Location

	mu    sync.Mutex
	cache map[string]cached
}

// NewOAuth builds a provider over the encrypted account store.
func NewOAuth(s *store.Accounts, cfg *oauth2.Config, defaultZone *time.Location) *OAuth {
	return &OAuth{store: s, cfg: cfg, defaultZone: defaultZone, cache: map[string]cached{}}
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

// Client returns a calendar client for email.
func (o *OAuth) Client(ctx context.Context, email string) (*calendar.Client, error) {
	token, err := o.store.RefreshToken(email)
	if err != nil {
		return nil, fmt.Errorf("account %s: %w", email, err)
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	c, ok := o.cache[email]
	if !ok || c.refreshToken != token {
		// The token source outlives this request, so it must not inherit ctx.
		src := o.cfg.TokenSource(context.Background(), &oauth2.Token{RefreshToken: token})
		svc, err := gcal.NewService(context.Background(),
			option.WithTokenSource(oauth2.ReuseTokenSource(nil, src)))
		if err != nil {
			return nil, fmt.Errorf("calendar service for %s: %w", email, err)
		}
		c = cached{refreshToken: token, svc: svc}
		o.cache[email] = c
	}
	return calendar.NewClient(c.svc, email, calendar.Options{DefaultZone: o.defaultZone}), nil
}
