// Package googleauth runs the server side of Google's OAuth 2.0 web flow:
// building the consent URL, exchanging the code, and identifying the account.
package googleauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	gcal "google.golang.org/api/calendar/v3"
)

const userinfoURL = "https://openidconnect.googleapis.com/v1/userinfo"

// Flow holds the OAuth client configuration.
type Flow struct {
	cfg         *oauth2.Config
	userinfoURL string
}

// NewFlow configures the flow; redirectURL must be registered on the client.
func NewFlow(clientID, clientSecret, redirectURL string) *Flow {
	return &Flow{
		cfg: &oauth2.Config{
			ClientID: clientID, ClientSecret: clientSecret,
			RedirectURL: redirectURL, Scopes: Scopes, Endpoint: google.Endpoint,
		},
		userinfoURL: userinfoURL,
	}
}

// Config exposes the oauth2 configuration for building token sources.
func (f *Flow) Config() *oauth2.Config { return f.cfg }

// AuthURL is the consent URL. Offline access plus forced consent makes
// Google return a refresh token even for a previously approved account, and
// one carrying every requested scope, so reconnecting upgrades an old grant.
// loginHint (an email, optional) preselects the account when reconnecting.
func (f *Flow) AuthURL(state, loginHint string) string {
	opts := []oauth2.AuthCodeOption{
		oauth2.AccessTypeOffline,
		oauth2.SetAuthURLParam("prompt", "consent select_account"),
		oauth2.SetAuthURLParam("include_granted_scopes", "true"),
	}
	if hint := strings.TrimSpace(loginHint); hint != "" {
		opts = append(opts, oauth2.SetAuthURLParam("login_hint", hint))
	}
	return f.cfg.AuthCodeURL(state, opts...)
}

// Grant is the result of a completed consent.
type Grant struct {
	Email        string
	RefreshToken string
	// Scopes Google reports as granted; nil when it did not say.
	Scopes []string
}

// Exchange trades the authorization code for tokens and resolves the
// account's verified email address.
func (f *Flow) Exchange(ctx context.Context, code string) (Grant, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	tok, err := f.cfg.Exchange(ctx, code)
	if err != nil {
		return Grant{}, fmt.Errorf("exchange code: %w", err)
	}
	if tok.RefreshToken == "" {
		return Grant{}, errors.New("google returned no refresh token; remove the app under " +
			"myaccount.google.com/permissions and connect again")
	}
	if !grantedCalendar(tok) {
		return Grant{}, errors.New("calendar access was not granted; tick the calendar " +
			"permission on the consent screen")
	}
	email, err := f.email(ctx, tok)
	if err != nil {
		return Grant{}, err
	}
	scope, _ := tok.Extra("scope").(string)
	return Grant{Email: email, RefreshToken: tok.RefreshToken, Scopes: ParseScopes(scope)}, nil
}

// grantedCalendar checks the granted scope list when Google reports one.
func grantedCalendar(tok *oauth2.Token) bool {
	scope, _ := tok.Extra("scope").(string)
	if scope == "" {
		return true
	}
	return HasScope(ParseScopes(scope), gcal.CalendarScope)
}

func (f *Flow) email(ctx context.Context, tok *oauth2.Token) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.userinfoURL, nil)
	if err != nil {
		return "", err
	}
	res, err := f.cfg.Client(ctx, tok).Do(req)
	if err != nil {
		return "", fmt.Errorf("userinfo: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("userinfo: HTTP %d", res.StatusCode)
	}
	var info struct {
		Email    string `json:"email"`
		Verified bool   `json:"email_verified"`
	}
	if err := json.NewDecoder(res.Body).Decode(&info); err != nil {
		return "", fmt.Errorf("userinfo: %w", err)
	}
	if info.Email == "" || !info.Verified {
		return "", errors.New("google did not return a verified email for this account")
	}
	return strings.ToLower(info.Email), nil
}
