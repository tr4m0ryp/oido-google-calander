package account

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"github.com/tr4m0ryp/oido-google-calander/internal/auth/crypt"
	"github.com/tr4m0ryp/oido-google-calander/internal/store"
	"github.com/tr4m0ryp/oido-google-calander/internal/tasks"
)

const (
	calScope   = "https://www.googleapis.com/auth/calendar"
	tasksScope = "https://www.googleapis.com/auth/tasks"
)

// fakeTokenServer answers refreshes with the given scope string (omitted
// when empty) and tokeninfo with infoScope.
func fakeTokenServer(t *testing.T, refreshScope, infoScope string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var infoCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/token":
			body := `{"access_token":"at","token_type":"Bearer","expires_in":3600`
			if refreshScope != "" {
				body += `,"scope":"` + refreshScope + `"`
			}
			_, _ = w.Write([]byte(body + "}"))
		case "/tokeninfo":
			infoCalls.Add(1)
			if r.Method != http.MethodPost || r.FormValue("access_token") != "at" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			_, _ = w.Write([]byte(`{"scope":"` + infoScope + `"}`))
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &infoCalls
}

func newProvider(t *testing.T, srv *httptest.Server, scopes []string) (*OAuth, *store.Accounts) {
	t.Helper()
	box, err := crypt.New("account-test-secret")
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(t.TempDir(), box)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Put("me@example.com", "1//rt", scopes); err != nil {
		t.Fatal(err)
	}
	cfg := &oauth2.Config{ClientID: "id", ClientSecret: "s", Endpoint: oauth2.Endpoint{TokenURL: srv.URL + "/token"}}
	o := NewOAuth(st, cfg, time.UTC)
	o.SetupURL = "https://cal.example.test/setup"
	o.tokenInfoURL = srv.URL + "/tokeninfo"
	o.httpClient = srv.Client()
	return o, st
}

func TestTasksScopeCheck(t *testing.T) {
	tests := []struct {
		name         string
		stored       []string
		refreshScope string
		infoScope    string
		wantTasks    bool
		wantInfo     int32
	}{
		{"stored grant with tasks", []string{calScope, tasksScope}, "", "", true, 0},
		{"stored legacy grant", []string{"openid", calScope}, calScope + " " + tasksScope, "", false, 0},
		{"unrecorded legacy, refresh reports scopes", nil, "openid " + calScope, "", false, 0},
		{"unrecorded upgraded, refresh reports scopes", nil, calScope + " " + tasksScope, "", true, 0},
		{"unrecorded, tokeninfo fallback", nil, "", calScope + " " + tasksScope, true, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv, infoCalls := fakeTokenServer(t, tc.refreshScope, tc.infoScope)
			o, st := newProvider(t, srv, tc.stored)
			c, err := o.Tasks(context.Background(), "me@example.com")
			if tc.wantTasks && (err != nil || c == nil) {
				t.Fatalf("Tasks = %v, want a client", err)
			}
			if !tc.wantTasks {
				if !errors.Is(err, tasks.ErrScopeMissing) {
					t.Fatalf("Tasks error = %v, want ErrScopeMissing", err)
				}
				for _, want := range []string{"https://cal.example.test/setup", "Reconnect to enable Tasks", "me@example.com"} {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("error %q lacks %q", err, want)
					}
				}
			}
			if infoCalls.Load() != tc.wantInfo {
				t.Errorf("tokeninfo calls = %d, want %d", infoCalls.Load(), tc.wantInfo)
			}
			// Learned scopes are recorded so later checks skip Google.
			if acct, _ := st.Get("me@example.com"); len(acct.Scopes) == 0 {
				t.Error("scopes were not recorded")
			}
		})
	}
}
