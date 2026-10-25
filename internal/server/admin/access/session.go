// Package access guards the admin pages: password check with constant-time
// comparison, failed-login throttling, and short-lived server-side sessions
// carried in an HttpOnly cookie (so the password never sits in a URL).
package access

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"sync"
	"time"
)

const cookieName = "gcal_admin"

// Flash is a one-shot message shown on the next dashboard render.
type Flash struct {
	OK   bool
	Text string
}

// Session is an authenticated admin browser session.
type Session struct {
	ID      string
	CSRF    string
	expires time.Time
	flash   *Flash
}

// Sessions is an in-memory session table; a restart signs everyone out.
type Sessions struct {
	ttl    time.Duration
	secure bool
	now    func() time.Time

	mu sync.Mutex
	m  map[string]*Session
}

// NewSessions builds the table. secure marks cookies HTTPS-only.
func NewSessions(ttl time.Duration, secure bool, now func() time.Time) *Sessions {
	if now == nil {
		now = time.Now
	}
	return &Sessions{ttl: ttl, secure: secure, now: now, m: map[string]*Session{}}
}

// Token returns a URL-safe random string with 256 bits of entropy.
func Token() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// SafeEqual compares secrets in constant time; hashing first hides length.
func SafeEqual(a, b string) bool {
	ha, hb := sha256.Sum256([]byte(a)), sha256.Sum256([]byte(b))
	return subtle.ConstantTimeCompare(ha[:], hb[:]) == 1
}

// Create starts a session and sets its cookie.
func (s *Sessions) Create(w http.ResponseWriter) *Session {
	sess := &Session{ID: Token(), CSRF: Token(), expires: s.now().Add(s.ttl)}
	s.mu.Lock()
	for id, old := range s.m {
		if !old.expires.After(s.now()) {
			delete(s.m, id)
		}
	}
	s.m[sess.ID] = sess
	s.mu.Unlock()
	http.SetCookie(w, s.cookie(sess.ID, int(s.ttl.Seconds())))
	return sess
}

// From returns the live session named by the request cookie, if any.
func (s *Sessions) From(r *http.Request) *Session {
	c, err := r.Cookie(cookieName)
	if err != nil || c.Value == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.m[c.Value]
	if !ok {
		return nil
	}
	if !sess.expires.After(s.now()) {
		delete(s.m, c.Value)
		return nil
	}
	return sess
}

// Lookup returns the live session with this id (used by the OAuth callback).
func (s *Sessions) Lookup(id string) *Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.m[id]
	if !ok || !sess.expires.After(s.now()) {
		return nil
	}
	return sess
}

// Destroy ends the session and clears the cookie.
func (s *Sessions) Destroy(w http.ResponseWriter, sess *Session) {
	if sess != nil {
		s.mu.Lock()
		delete(s.m, sess.ID)
		s.mu.Unlock()
	}
	http.SetCookie(w, s.cookie("", -1))
}

// SetFlash queues a message for the session's next page view.
func (s *Sessions) SetFlash(sess *Session, f Flash) {
	s.mu.Lock()
	sess.flash = &f
	s.mu.Unlock()
}

// TakeFlash returns and clears the queued message.
func (s *Sessions) TakeFlash(sess *Session) *Flash {
	s.mu.Lock()
	defer s.mu.Unlock()
	f := sess.flash
	sess.flash = nil
	return f
}

func (s *Sessions) cookie(value string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name: cookieName, Value: value, Path: "/", MaxAge: maxAge,
		HttpOnly: true, Secure: s.secure,
		// Lax (not Strict) so the cookie survives the top-level redirect back
		// from Google's consent screen; POSTs stay protected by the CSRF token.
		SameSite: http.SameSiteLaxMode,
	}
}
