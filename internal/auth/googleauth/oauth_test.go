package googleauth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"golang.org/x/oauth2"
)

func TestAuthURL(t *testing.T) {
	f := NewFlow("client-id", "client-secret", "https://cal.example.test/oauth/callback")
	u, err := url.Parse(f.AuthURL("nonce123"))
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	checks := map[string]string{
		"client_id":    "client-id",
		"redirect_uri": "https://cal.example.test/oauth/callback",
		"state":        "nonce123",
		"access_type":  "offline",
	}
	for k, want := range checks {
		if got := q.Get(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
	if !strings.Contains(q.Get("scope"), "https://www.googleapis.com/auth/calendar") {
		t.Errorf("scope %q lacks calendar", q.Get("scope"))
	}
	if strings.Contains(u.String(), "client-secret") {
		t.Error("auth URL leaks the client secret")
	}
}

func TestExchange(t *testing.T) {
	google := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/token":
			_, _ = w.Write([]byte(`{"access_token":"at","token_type":"Bearer","expires_in":3600,` +
				`"refresh_token":"1//rt","scope":"openid https://www.googleapis.com/auth/calendar email"}`))
		case "/userinfo":
			if r.Header.Get("Authorization") != "Bearer at" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_, _ = w.Write([]byte(`{"email":"Person@Example.com","email_verified":true}`))
		}
	}))
	defer google.Close()

	f := NewFlow("id", "secret", "https://x.test/oauth/callback")
	f.cfg.Endpoint = oauth2.Endpoint{AuthURL: google.URL + "/auth", TokenURL: google.URL + "/token"}
	f.userinfoURL = google.URL + "/userinfo"

	g, err := f.Exchange(context.Background(), "code")
	if err != nil {
		t.Fatal(err)
	}
	if g.Email != "person@example.com" || g.RefreshToken != "1//rt" {
		t.Errorf("grant = %+v", g)
	}
}

func TestGrantedCalendar(t *testing.T) {
	tests := []struct {
		scope string
		want  bool
	}{
		{"", true},
		{"openid email https://www.googleapis.com/auth/calendar", true},
		{"openid email https://www.googleapis.com/auth/calendar.readonly", false},
	}
	for _, tc := range tests {
		tok := (&oauth2.Token{}).WithExtra(map[string]any{"scope": tc.scope})
		if got := grantedCalendar(tok); got != tc.want {
			t.Errorf("grantedCalendar(%q) = %v, want %v", tc.scope, got, tc.want)
		}
	}
}
