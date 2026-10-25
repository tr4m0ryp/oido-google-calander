// Package config reads settings from the environment and fails fast on
// anything missing or malformed.
package config

import (
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/tr4m0ryp/oido-google-calander/internal/calendar/timeinput"
)

// LoginLimits throttles failed admin logins.
type LoginLimits struct {
	MaxFailures       int           // per-IP failures inside Window that trigger a lockout
	Window            time.Duration // sliding window for counting failures
	Lockout           time.Duration // how long a locked IP must wait
	GlobalMaxFailures int           // failures across all IPs that block everyone (0 = off)
}

// HTTP is the configuration of the multi-account HTTP service.
type HTTP struct {
	Addr               string
	ServerURL          string
	GoogleClientID     string
	GoogleClientSecret string
	AdminPassword      string
	EncryptionKey      string
	MCPAPIKey          string
	AllowNoAuth        bool
	DataDir            string
	DefaultZone        *time.Location
	Limits             LoginLimits
	TrustedProxies     []netip.Prefix
	SessionTTL         time.Duration
}

// Private and loopback ranges: the app binds locally behind Caddy, and
// Docker's port proxy connects from the bridge gateway.
const defaultTrustedProxies = "127.0.0.0/8,::1/128,10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,fc00::/7"

// Getenv matches os.Getenv; tests pass a map lookup.
type Getenv func(string) string

// LoadHTTP reads the HTTP-mode configuration.
func LoadHTTP(env Getenv) (HTTP, error) {
	r := reader{env: env}
	c := HTTP{
		GoogleClientID:     r.required("GOOGLE_CLIENT_ID"),
		GoogleClientSecret: r.required("GOOGLE_CLIENT_SECRET"),
		AdminPassword:      r.required("ADMIN_PASSWORD"),
		EncryptionKey:      r.required("ENCRYPTION_KEY"),
		MCPAPIKey:          env("MCP_API_KEY"),
		AllowNoAuth:        env("MCP_ALLOW_UNAUTHENTICATED") == "true",
		DataDir:            r.str("DATA_DIR", "./data"),
		Limits: LoginLimits{
			MaxFailures:       r.integer("ADMIN_LOGIN_MAX_FAILURES", 5, 1),
			Window:            time.Duration(r.integer("ADMIN_LOGIN_WINDOW_MINUTES", 15, 1)) * time.Minute,
			Lockout:           time.Duration(r.integer("ADMIN_LOGIN_LOCKOUT_MINUTES", 15, 1)) * time.Minute,
			GlobalMaxFailures: r.integer("ADMIN_LOGIN_GLOBAL_MAX_FAILURES", 50, 0),
		},
		SessionTTL: time.Duration(r.integer("ADMIN_SESSION_HOURS", 12, 1)) * time.Hour,
	}
	port := r.integer("PORT", 3000, 1)
	c.Addr = fmt.Sprintf("%s:%d", r.str("BIND", "0.0.0.0"), port)
	c.ServerURL = strings.TrimRight(r.str("SERVER_URL", fmt.Sprintf("http://localhost:%d", port)), "/")
	if u, err := url.Parse(c.ServerURL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		r.errs = append(r.errs, fmt.Errorf("SERVER_URL must be an absolute http(s) URL"))
	}
	if len(c.AdminPassword) > 0 && len(c.AdminPassword) < 12 {
		r.errs = append(r.errs, errors.New("ADMIN_PASSWORD must be at least 12 characters"))
	}
	if c.MCPAPIKey == "" && !c.AllowNoAuth {
		r.errs = append(r.errs, errors.New("MCP_API_KEY is required (or set MCP_ALLOW_UNAUTHENTICATED=true "+
			"for a private network only)"))
	}
	c.DefaultZone = r.zone("DEFAULT_TIMEZONE")
	c.TrustedProxies = r.prefixes("TRUSTED_PROXIES", defaultTrustedProxies)
	return c, r.err()
}

// reader accumulates every configuration error so one start reports them all.
type reader struct {
	env     Getenv
	missing []string
	errs    []error
}

func (r *reader) required(name string) string {
	v := strings.TrimSpace(r.env(name))
	if v == "" {
		r.missing = append(r.missing, name)
	}
	return v
}

func (r *reader) str(name, def string) string {
	if v := strings.TrimSpace(r.env(name)); v != "" {
		return v
	}
	return def
}

func (r *reader) integer(name string, def, min int) int {
	raw := strings.TrimSpace(r.env(name))
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < min {
		r.errs = append(r.errs, fmt.Errorf("%s must be an integer >= %d (got %q)", name, min, raw))
		return def
	}
	return n
}

func (r *reader) zone(name string) *time.Location {
	loc, err := timeinput.LoadZone(r.env(name), time.UTC)
	if err != nil {
		r.errs = append(r.errs, fmt.Errorf("%s: %w", name, err))
		return time.UTC
	}
	return loc
}

func (r *reader) prefixes(name, def string) []netip.Prefix {
	var out []netip.Prefix
	for _, part := range strings.Split(r.str(name, def), ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		p, err := netip.ParsePrefix(part)
		if err != nil {
			r.errs = append(r.errs, fmt.Errorf("%s: invalid CIDR %q", name, part))
			continue
		}
		out = append(out, p.Masked())
	}
	return out
}

func (r *reader) err() error {
	errs := r.errs
	if len(r.missing) > 0 {
		errs = append([]error{fmt.Errorf("missing required environment variables: %s",
			strings.Join(r.missing, ", "))}, errs...)
	}
	return errors.Join(errs...)
}
