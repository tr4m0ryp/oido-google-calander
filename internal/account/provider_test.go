package account

import (
	"context"
	"strings"
	"testing"

	"github.com/tr4m0ryp/oido-google-calander/internal/calendar"
)

type fakeProvider []string

func (f fakeProvider) Emails() []string { return f }

func (f fakeProvider) Client(context.Context, string) (*calendar.Client, error) { return nil, nil }

func TestResolve(t *testing.T) {
	two := fakeProvider{"a@example.com", "b@example.com"}
	one := fakeProvider{"solo@example.com"}
	tests := []struct {
		name     string
		p        Provider
		arg      string
		allowAll bool
		want     string
		errHas   string
	}{
		{"none connected", fakeProvider{}, "", true, "", "no Google accounts"},
		{"single implicit", one, "", false, "solo@example.com", ""},
		{"ambiguous implicit", two, "", false, "", "several accounts"},
		{"case-insensitive", two, "B@Example.com", false, "b@example.com", ""},
		{"all allowed", two, "ALL", true, "a@example.com,b@example.com", ""},
		{"all refused", two, "all", false, "", "one account"},
		{"unknown", two, "c@example.com", true, "", "not connected"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Resolve(tc.p, tc.arg, tc.allowAll)
			if tc.errHas != "" {
				if err == nil || !strings.Contains(err.Error(), tc.errHas) {
					t.Fatalf("err = %v, want containing %q", err, tc.errHas)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if s := strings.Join(got, ","); s != tc.want {
				t.Errorf("Resolve = %s, want %s", s, tc.want)
			}
		})
	}
}

func TestNewServiceAccountValidates(t *testing.T) {
	if _, err := NewServiceAccount("{not json", "", nil); err == nil {
		t.Error("want error for invalid JSON")
	}
	if _, err := NewServiceAccount(`{"type":"authorized_user"}`, "", nil); err == nil {
		t.Error("want error for non-service-account key")
	}
	sa, err := NewServiceAccount(`{"type":"service_account","client_email":"Bot@P.iam.gserviceaccount.com"}`, "team@group.calendar.google.com", nil)
	if err != nil {
		t.Fatal(err)
	}
	if e := sa.Emails(); len(e) != 1 || e[0] != "bot@p.iam.gserviceaccount.com" {
		t.Errorf("Emails = %v", e)
	}
}
