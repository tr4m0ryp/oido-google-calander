package account

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/tr4m0ryp/oido-google-calander/internal/auth/googleauth"
	"github.com/tr4m0ryp/oido-google-calander/internal/calendar"
)

const tokenInfoURL = "https://oauth2.googleapis.com/tokeninfo"

// GrantedScopes returns the scopes an account's refresh token carries.
// Accounts connected since scopes were recorded answer from the store. For
// older ones the scopes are learned once, from the refresh response or else
// Google's tokeninfo endpoint, and written back to the store.
func (o *OAuth) GrantedScopes(ctx context.Context, email string) ([]string, error) {
	acct, ok := o.store.Get(email)
	if !ok {
		return nil, fmt.Errorf("account %s is not connected", email)
	}
	if len(acct.Scopes) > 0 {
		return acct.Scopes, nil
	}
	ts, err := o.tokenSource(email)
	if err != nil {
		return nil, err
	}
	tok, err := ts.Token()
	if err != nil {
		return nil, fmt.Errorf("account %s: %w", email, calendar.Explain(err))
	}
	scope, _ := tok.Extra("scope").(string)
	if scope == "" {
		if scope, err = o.tokenInfo(ctx, tok.AccessToken); err != nil {
			return nil, err
		}
	}
	scopes := googleauth.ParseScopes(scope)
	if len(scopes) > 0 {
		if err := o.store.SetScopes(email, scopes); err != nil {
			slog.Warn("record granted scopes", "email", email, "err", err)
		}
	}
	return scopes, nil
}

// HasTasks reports whether email granted the Google Tasks scope.
func (o *OAuth) HasTasks(ctx context.Context, email string) (bool, error) {
	scopes, err := o.GrantedScopes(ctx, email)
	if err != nil {
		return false, err
	}
	return googleauth.HasScope(scopes, googleauth.TasksScope), nil
}

// tokenInfo asks Google which scopes an access token carries.
func (o *OAuth) tokenInfo(ctx context.Context, accessToken string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.tokenInfoURL,
		strings.NewReader(url.Values{"access_token": {accessToken}}.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := o.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("tokeninfo: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("tokeninfo: HTTP %d", res.StatusCode)
	}
	var info struct {
		Scope string `json:"scope"`
	}
	if err := json.NewDecoder(res.Body).Decode(&info); err != nil {
		return "", fmt.Errorf("tokeninfo: %w", err)
	}
	return info.Scope, nil
}
