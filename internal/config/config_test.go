package config

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func envOf(m map[string]string) Getenv { return func(k string) string { return m[k] } }

func validHTTP() map[string]string {
	return map[string]string{
		"GOOGLE_CLIENT_ID": "id", "GOOGLE_CLIENT_SECRET": "secret",
		"ADMIN_PASSWORD": "correct-horse-battery", "ENCRYPTION_KEY": "0123456789abcdef0123",
		"MCP_API_KEY": "key", "SERVER_URL": "https://cal.example.test/",
	}
}

func TestLoadHTTPDefaults(t *testing.T) {
	c, err := LoadHTTP(envOf(validHTTP()))
	if err != nil {
		t.Fatal(err)
	}
	if c.ServerURL != "https://cal.example.test" || c.Addr != "0.0.0.0:3000" {
		t.Errorf("url/addr = %s %s", c.ServerURL, c.Addr)
	}
	want := LoginLimits{MaxFailures: 5, Window: 15 * time.Minute, Lockout: 15 * time.Minute, GlobalMaxFailures: 50}
	if c.Limits != want {
		t.Errorf("limits = %+v", c.Limits)
	}
	if c.DefaultZone != time.UTC || len(c.TrustedProxies) != 6 {
		t.Errorf("zone %v proxies %v", c.DefaultZone, c.TrustedProxies)
	}
}

func TestLoadHTTPErrors(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(map[string]string)
		errHas string
	}{
		{"missing", func(m map[string]string) { delete(m, "GOOGLE_CLIENT_ID"); delete(m, "ENCRYPTION_KEY") },
			"GOOGLE_CLIENT_ID, ENCRYPTION_KEY"},
		{"no mcp key", func(m map[string]string) { delete(m, "MCP_API_KEY") }, "MCP_API_KEY is required"},
		{"short password", func(m map[string]string) { m["ADMIN_PASSWORD"] = "short" }, "at least 12"},
		{"bad int", func(m map[string]string) { m["ADMIN_LOGIN_MAX_FAILURES"] = "0" }, "ADMIN_LOGIN_MAX_FAILURES"},
		{"bad zone", func(m map[string]string) { m["DEFAULT_TIMEZONE"] = "Nowhere/City" }, "DEFAULT_TIMEZONE"},
		{"bad cidr", func(m map[string]string) { m["TRUSTED_PROXIES"] = "10.0.0.0/33" }, "invalid CIDR"},
		{"bad url", func(m map[string]string) { m["SERVER_URL"] = "cal.example.test" }, "SERVER_URL"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := validHTTP()
			tc.mutate(m)
			_, err := LoadHTTP(envOf(m))
			if err == nil || !strings.Contains(err.Error(), tc.errHas) {
				t.Errorf("err = %v, want containing %q", err, tc.errHas)
			}
		})
	}
}

func TestLoadStdio(t *testing.T) {
	key := `{"type":"service_account"}`
	c, err := LoadStdio(envOf(map[string]string{
		"CALENDAR_SERVICE_ACCOUNT_B64": base64.StdEncoding.EncodeToString([]byte(key)),
		"CALENDAR_ID":                  "team@group.calendar.google.com",
		"CALENDAR_TIMEZONE":            "Europe/Amsterdam",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if c.ServiceAccountJSON != key || c.CalendarID != "team@group.calendar.google.com" ||
		c.DefaultZone.String() != "Europe/Amsterdam" {
		t.Errorf("stdio config = %+v", c)
	}
}

func TestLegacyJSONAndDotEnvDoNotOverride(t *testing.T) {
	set := map[string]string{"CALENDAR_ID": "from-env"}
	setenv := func(k, v string) error { set[k] = v; return nil }
	err := ApplyLegacyJSON(`{"CALENDAR_ID":"from-json","CALENDAR_TIMEZONE":"UTC"}`, setenv, envOf(set))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), ".env")
	_ = os.WriteFile(path, []byte("# c\nCALENDAR_TIMEZONE=Asia/Tokyo\nCALENDAR_MAX_RESULTS=\"5\"\n"), 0o600)
	if err := LoadDotEnv(path, setenv, envOf(set)); err != nil {
		t.Fatal(err)
	}
	if set["CALENDAR_ID"] != "from-env" || set["CALENDAR_TIMEZONE"] != "UTC" || set["CALENDAR_MAX_RESULTS"] != "5" {
		t.Errorf("env = %v", set)
	}
}
