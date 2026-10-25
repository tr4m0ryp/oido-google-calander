package server

import (
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/tr4m0ryp/oido-google-calander/internal/account"
	"github.com/tr4m0ryp/oido-google-calander/internal/auth/crypt"
	"github.com/tr4m0ryp/oido-google-calander/internal/auth/googleauth"
	"github.com/tr4m0ryp/oido-google-calander/internal/config"
	"github.com/tr4m0ryp/oido-google-calander/internal/server/admin"
	"github.com/tr4m0ryp/oido-google-calander/internal/server/clientip"
	"github.com/tr4m0ryp/oido-google-calander/internal/store"
	"github.com/tr4m0ryp/oido-google-calander/internal/tools"
)

const (
	adminPassword = "correct-horse-battery-staple"
	apiKey        = "test-api-key"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	cfg := config.HTTP{
		ServerURL: "https://cal.example.test", AdminPassword: adminPassword, MCPAPIKey: apiKey,
		DefaultZone: time.UTC, SessionTTL: time.Hour,
		Limits: config.LoginLimits{MaxFailures: 5, Window: 15 * time.Minute,
			Lockout: 15 * time.Minute, GlobalMaxFailures: 50},
		TrustedProxies: []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")},
	}
	box, err := crypt.New("encryption-key-for-tests")
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(t.TempDir(), box)
	if err != nil {
		t.Fatal(err)
	}
	flow := googleauth.NewFlow("client-123.apps.googleusercontent.com", "secret",
		cfg.ServerURL+"/oauth/callback")
	provider := account.NewOAuth(st, flow.Config(), time.UTC)
	adm, err := admin.New(admin.Deps{Config: cfg, Store: st, Accounts: provider, Flow: flow,
		ClientIP: clientip.New(cfg.TrustedProxies).Of, ToolCount: tools.Count})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(New(Options{MCP: tools.NewServer(provider, "test"), APIKey: apiKey,
		Admin: adm, AccountCount: st.Len, Version: "test"}))
	t.Cleanup(srv.Close)
	return srv
}

func noRedirect(jar http.CookieJar) *http.Client {
	return &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
}

func TestSetupRequiresPassword(t *testing.T) {
	srv := newTestServer(t)
	res, _ := http.Get(srv.URL + "/setup")
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anonymous /setup = %d", res.StatusCode)
	}
	if csp := res.Header.Get("Content-Security-Policy"); !strings.Contains(csp, "default-src 'none'") {
		t.Errorf("CSP = %q", csp)
	}
	req, _ := http.NewRequest("GET", srv.URL+"/setup", nil)
	req.Header.Set("X-Admin-Key", adminPassword)
	res, _ = http.DefaultClient.Do(req)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("/setup with key = %d", res.StatusCode)
	}
}

func TestLoginLockout(t *testing.T) {
	srv := newTestServer(t)
	client := noRedirect(nil)
	post := func(pw, ip string) *http.Response {
		req, _ := http.NewRequest("POST", srv.URL+"/setup/login", strings.NewReader(url.Values{"password": {pw}}.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("X-Forwarded-For", ip)
		res, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	for i := 1; i <= 4; i++ {
		if res := post("wrong", "198.51.100.1"); res.StatusCode != http.StatusUnauthorized {
			t.Fatalf("attempt %d = %d", i, res.StatusCode)
		}
	}
	res := post("wrong", "198.51.100.1")
	if res.StatusCode != http.StatusTooManyRequests || res.Header.Get("Retry-After") == "" {
		t.Fatalf("5th failure = %d retry-after %q", res.StatusCode, res.Header.Get("Retry-After"))
	}
	if res := post(adminPassword, "198.51.100.1"); res.StatusCode != http.StatusTooManyRequests {
		t.Errorf("correct password while locked = %d, want 429", res.StatusCode)
	}
	if res := post(adminPassword, "198.51.100.2"); res.StatusCode != http.StatusSeeOther {
		t.Errorf("other client login = %d, want 303", res.StatusCode)
	}
}

func TestOAuthStartAndStateIsSingleUse(t *testing.T) {
	srv := newTestServer(t)
	jar, _ := cookiejar.New(nil)
	client := noRedirect(jar)
	form := strings.NewReader(url.Values{"password": {adminPassword}}.Encode())
	res, _ := client.Post(srv.URL+"/setup/login", "application/x-www-form-urlencoded", form)
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("login = %d", res.StatusCode)
	}
	res, _ = client.Get(srv.URL + "/oauth/start")
	loc, _ := url.Parse(res.Header.Get("Location"))
	if res.StatusCode != http.StatusFound || loc.Host != "accounts.google.com" {
		t.Fatalf("oauth/start = %d %s", res.StatusCode, loc)
	}
	q := loc.Query()
	if q.Get("client_id") != "client-123.apps.googleusercontent.com" ||
		q.Get("redirect_uri") != "https://cal.example.test/oauth/callback" || q.Get("state") == "" {
		t.Fatalf("auth params = %v", q)
	}
	cb := srv.URL + "/oauth/callback?error=access_denied&state=" + url.QueryEscape(q.Get("state"))
	if res, _ = client.Get(cb); res.StatusCode != http.StatusSeeOther {
		t.Fatalf("first callback = %d", res.StatusCode)
	}
	if res, _ = client.Get(cb); res.StatusCode != http.StatusBadRequest {
		t.Errorf("replayed state = %d, want 400", res.StatusCode)
	}
	if res, _ = http.Get(srv.URL + "/oauth/start"); res.StatusCode != http.StatusUnauthorized {
		t.Errorf("anonymous oauth/start = %d", res.StatusCode)
	}
}

func TestRemoveNeedsCSRF(t *testing.T) {
	srv := newTestServer(t)
	jar, _ := cookiejar.New(nil)
	client := noRedirect(jar)
	_, _ = client.Post(srv.URL+"/setup/login", "application/x-www-form-urlencoded",
		strings.NewReader(url.Values{"password": {adminPassword}}.Encode()))
	res, _ := client.Post(srv.URL+"/setup/remove", "application/x-www-form-urlencoded",
		strings.NewReader(url.Values{"email": {"a@example.com"}}.Encode()))
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("remove without csrf = %d, want 403", res.StatusCode)
	}
}

func TestMCPBearer(t *testing.T) {
	srv := newTestServer(t)
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`
	send := func(auth string) *http.Response {
		req, _ := http.NewRequest("POST", srv.URL+"/mcp", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		if auth != "" {
			req.Header.Set("Authorization", auth)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	if res := send(""); res.StatusCode != http.StatusUnauthorized {
		t.Errorf("no bearer = %d", res.StatusCode)
	}
	if res := send("Bearer nope"); res.StatusCode != http.StatusUnauthorized {
		t.Errorf("wrong bearer = %d", res.StatusCode)
	}
	res := send("Bearer " + apiKey)
	buf := new(strings.Builder)
	_, _ = io.Copy(buf, res.Body)
	if res.StatusCode != http.StatusOK || !strings.Contains(buf.String(), "g_calendar_find_free_slots") {
		t.Errorf("tools/list = %d %s", res.StatusCode, buf.String())
	}
}
