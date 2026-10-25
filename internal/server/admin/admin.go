// Package admin serves the password-protected /setup dashboard and the
// Google OAuth connect flow (/oauth/start, /oauth/callback).
package admin

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/tr4m0ryp/oido-google-calander/internal/account"
	"github.com/tr4m0ryp/oido-google-calander/internal/auth/googleauth"
	"github.com/tr4m0ryp/oido-google-calander/internal/config"
	"github.com/tr4m0ryp/oido-google-calander/internal/server/admin/access"
	"github.com/tr4m0ryp/oido-google-calander/internal/server/admin/views"
	"github.com/tr4m0ryp/oido-google-calander/internal/store"
)

// Deps are the admin handlers' collaborators.
type Deps struct {
	Config    config.HTTP
	Store     *store.Accounts
	Accounts  *account.OAuth
	Flow      *googleauth.Flow
	ClientIP  func(*http.Request) string
	ToolCount int
}

// Admin holds admin-page state: sessions, throttle and pending OAuth states.
type Admin struct {
	deps   Deps
	guard  *access.Guard
	views  *views.Renderer
	states *stateTable
}

// New builds the admin handlers.
func New(d Deps) (*Admin, error) {
	r, err := views.New()
	if err != nil {
		return nil, err
	}
	a := &Admin{deps: d, views: r, states: newStateTable(stateTTL, nil)}
	secure := strings.HasPrefix(d.Config.ServerURL, "https://")
	a.guard = access.NewGuard(d.Config.AdminPassword,
		access.NewLimiter(d.Config.Limits, nil),
		access.NewSessions(d.Config.SessionTTL, secure, nil),
		d.ClientIP, a.renderLogin)
	return a, nil
}

// Register mounts the admin routes.
func (a *Admin) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/setup", http.StatusFound)
	})
	mux.Handle("GET /setup", a.page(a.dashboard))
	mux.Handle("POST /setup/login", a.page(a.guard.Login))
	mux.Handle("POST /setup/logout", a.page(a.logout))
	mux.Handle("POST /setup/remove", a.page(a.remove))
	mux.Handle("GET /oauth/start", a.page(a.oauthStart))
	mux.Handle("GET /oauth/callback", a.page(a.oauthCallback))
}

type nonceKey struct{}

// page wraps an admin handler with a per-response CSP nonce and the
// headers every admin page carries.
func (a *Admin) page(h http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nonce := access.Token()
		hdr := w.Header()
		hdr.Set("Content-Security-Policy", "default-src 'none'; style-src 'nonce-"+nonce+"'; "+
			"script-src 'nonce-"+nonce+"'; img-src 'self' data:; form-action 'self'; "+
			"frame-ancestors 'none'; base-uri 'none'")
		hdr.Set("Cache-Control", "no-store")
		hdr.Set("Referrer-Policy", "no-referrer")
		hdr.Set("X-Content-Type-Options", "nosniff")
		hdr.Set("X-Frame-Options", "DENY")
		r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
		h(w, r.WithContext(context.WithValue(r.Context(), nonceKey{}, nonce)))
	})
}

func nonceOf(r *http.Request) string {
	n, _ := r.Context().Value(nonceKey{}).(string)
	return n
}

func (a *Admin) renderLogin(w http.ResponseWriter, r *http.Request, status int, v access.LoginView) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := a.views.Login(w, nonceOf(r), views.Login{Message: loginMessage(v), Locked: v.LockedSeconds > 0}); err != nil {
		slog.Error("render login", "err", err)
	}
}

const warnAtRemaining = 2

func loginMessage(v access.LoginView) string {
	switch {
	case v.LockedSeconds > 0:
		return "Too many failed attempts. Sign-in is locked; try again in " + describeWait(v.LockedSeconds) + "."
	case !v.Failed:
		return ""
	case v.Remaining <= warnAtRemaining:
		return "Incorrect password. " + plural(v.Remaining, "attempt") +
			" left before sign-in is temporarily locked."
	}
	return "Incorrect password. Try again."
}

func describeWait(seconds int) string {
	if seconds < 60 {
		return plural(seconds, "second")
	}
	return plural((seconds+59)/60, "minute")
}

func plural(n int, unit string) string {
	s := strconv.Itoa(n) + " " + unit
	if n != 1 {
		s += "s"
	}
	return s
}
