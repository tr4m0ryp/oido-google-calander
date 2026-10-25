// Package account maps account names from tool calls to authenticated
// calendar clients, for both OAuth-connected accounts (HTTP mode) and a
// single service account (upstream's stdio mode).
package account

import (
	"context"
	"fmt"
	"strings"

	"github.com/tr4m0ryp/oido-google-calander/internal/calendar"
)

// All selects every connected account in tools that support fan-out.
const All = "all"

// Provider resolves account emails to calendar clients.
type Provider interface {
	// Emails lists connected accounts in a stable order.
	Emails() []string
	// Client returns a ready client for one connected account.
	Client(ctx context.Context, email string) (*calendar.Client, error)
}

// Resolve expands an account argument into the accounts to act on.
// Empty means "the only account" when exactly one is connected; "all" is
// accepted only when allowAll is set.
func Resolve(p Provider, arg string, allowAll bool) ([]string, error) {
	emails := p.Emails()
	if len(emails) == 0 {
		return nil, fmt.Errorf("no Google accounts are connected; add one on the /setup page")
	}
	arg = strings.ToLower(strings.TrimSpace(arg))
	switch {
	case arg == All && allowAll:
		return emails, nil
	case arg == All:
		return nil, fmt.Errorf("this tool acts on one account; pick one of: %s", strings.Join(emails, ", "))
	case arg == "" && len(emails) == 1:
		return emails, nil
	case arg == "":
		hint := strings.Join(emails, ", ")
		if allowAll {
			hint += `, or "all"`
		}
		return nil, fmt.Errorf("several accounts are connected; set account to one of: %s", hint)
	}
	for _, e := range emails {
		if strings.EqualFold(e, arg) {
			return []string{e}, nil
		}
	}
	return nil, fmt.Errorf("account %q is not connected; connected: %s", arg, strings.Join(emails, ", "))
}

// One resolves a single-account argument straight to its client.
func One(ctx context.Context, p Provider, arg string) (*calendar.Client, error) {
	emails, err := Resolve(p, arg, false)
	if err != nil {
		return nil, err
	}
	return p.Client(ctx, emails[0])
}
