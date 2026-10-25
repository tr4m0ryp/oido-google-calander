package access

import (
	"log/slog"
	"math"
	"net/http"
	"strconv"
)

// HeaderKey lets scripts authenticate with the admin password in a header.
const HeaderKey = "X-Admin-Key"

// LoginView is what the login page needs to explain the current state.
type LoginView struct {
	Failed        bool
	Remaining     int
	LockedSeconds int
}

// LoginPage renders the login form with the given status code.
type LoginPage func(w http.ResponseWriter, r *http.Request, status int, v LoginView)

// Guard authenticates admin requests.
type Guard struct {
	password string
	limiter  *Limiter
	sessions *Sessions
	clientIP func(*http.Request) string
	page     LoginPage
}

// NewGuard wires the guard's collaborators; one limiter is shared by every
// password-checking path so failures count together.
func NewGuard(password string, l *Limiter, s *Sessions, clientIP func(*http.Request) string,
	page LoginPage) *Guard {
	return &Guard{password: password, limiter: l, sessions: s, clientIP: clientIP, page: page}
}

// Sessions exposes the session table.
func (g *Guard) Sessions() *Sessions { return g.sessions }

// Require returns the caller's session, authenticating via cookie or the
// X-Admin-Key header. On failure it has already written the response.
func (g *Guard) Require(w http.ResponseWriter, r *http.Request) (*Session, bool) {
	if sess := g.sessions.From(r); sess != nil {
		return sess, true
	}
	key := r.Header.Get(HeaderKey)
	if key == "" {
		g.page(w, r, http.StatusUnauthorized, LoginView{})
		return nil, false
	}
	if !g.attempt(w, r, key) {
		return nil, false
	}
	return g.sessions.Create(w), true
}

// Login handles the login form POST.
func (g *Guard) Login(w http.ResponseWriter, r *http.Request) {
	if g.attempt(w, r, r.PostFormValue("password")) {
		g.sessions.Create(w)
		http.Redirect(w, r, "/setup", http.StatusSeeOther)
	}
}

// attempt checks one password try against the throttle and the secret,
// writing the failure response itself.
func (g *Guard) attempt(w http.ResponseWriter, r *http.Request, key string) bool {
	ip := g.clientIP(r)
	if st := g.limiter.Check(ip); st.Blocked {
		g.locked(w, r, st)
		return false
	}
	if key != "" && SafeEqual(key, g.password) {
		g.limiter.Success(ip)
		return true
	}
	st := g.limiter.Failure(ip)
	if st.Blocked {
		slog.Warn("admin login locked after repeated failures", "ip", ip)
		g.locked(w, r, st)
		return false
	}
	g.page(w, r, http.StatusUnauthorized, LoginView{Failed: true, Remaining: st.Remaining})
	return false
}

func (g *Guard) locked(w http.ResponseWriter, r *http.Request, st Status) {
	secs := int(math.Ceil(st.RetryAfter.Seconds()))
	if secs < 1 {
		secs = 1
	}
	w.Header().Set("Retry-After", strconv.Itoa(secs))
	g.page(w, r, http.StatusTooManyRequests, LoginView{LockedSeconds: secs})
}
