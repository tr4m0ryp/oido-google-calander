package account

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2/google"
	gcal "google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"

	"github.com/tr4m0ryp/oido-google-calander/internal/calendar"
)

// ServiceAccount is upstream's original mode: one Google service account
// acting on a calendar shared with it (CALENDAR_ID).
type ServiceAccount struct {
	email       string
	keyJSON     []byte
	calendarID  string
	defaultZone *time.Location

	once sync.Once
	svc  *gcal.Service
	err  error
}

// NewServiceAccount validates the key JSON; the service is built lazily.
func NewServiceAccount(keyJSON, calendarID string, defaultZone *time.Location) (*ServiceAccount, error) {
	raw := []byte(strings.TrimSpace(keyJSON))
	var meta struct {
		Type        string `json:"type"`
		ClientEmail string `json:"client_email"`
	}
	if err := json.Unmarshal(raw, &meta); err != nil {
		return nil, fmt.Errorf("service account key is not valid JSON (try CALENDAR_SERVICE_ACCOUNT_B64): %w", err)
	}
	if meta.Type != "service_account" || meta.ClientEmail == "" {
		return nil, fmt.Errorf("key JSON is not a service account key")
	}
	return &ServiceAccount{email: strings.ToLower(meta.ClientEmail), keyJSON: raw,
		calendarID: calendarID, defaultZone: defaultZone}, nil
}

// Emails returns the single service account identity.
func (s *ServiceAccount) Emails() []string { return []string{s.email} }

// Client returns the service-account calendar client.
func (s *ServiceAccount) Client(_ context.Context, email string) (*calendar.Client, error) {
	if !strings.EqualFold(email, s.email) {
		return nil, fmt.Errorf("account %q is not configured", email)
	}
	s.once.Do(func() {
		conf, err := google.JWTConfigFromJSON(s.keyJSON, gcal.CalendarScope)
		if err != nil {
			s.err = fmt.Errorf("parse service account key: %w", err)
			return
		}
		s.svc, s.err = gcal.NewService(context.Background(),
			option.WithHTTPClient(conf.Client(context.Background())))
	})
	if s.err != nil {
		return nil, s.err
	}
	return calendar.NewClient(s.svc, s.email, calendar.Options{
		DefaultCalendar: s.calendarID, DefaultZone: s.defaultZone,
	}), nil
}
