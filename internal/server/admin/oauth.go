package admin

import (
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/tr4m0ryp/oido-google-calander/internal/auth/googleauth"
	"github.com/tr4m0ryp/oido-google-calander/internal/server/admin/access"
)

// The OAuth state is a single-use random nonce bound server-side to the
// admin session that started the flow, so the callback is CSRF-protected
// and no admin secret travels through Google's redirect chain.

const (
	stateTTL       = 10 * time.Minute
	maxPendingAuth = 256
)

type pendingState struct {
	sessionID string
	expires   time.Time
}

type stateTable struct {
	ttl time.Duration
	now func() time.Time

	mu sync.Mutex
	m  map[string]pendingState
}

func newStateTable(ttl time.Duration, now func() time.Time) *stateTable {
	if now == nil {
		now = time.Now
	}
	return &stateTable{ttl: ttl, now: now, m: map[string]pendingState{}}
}

// issue mints a nonce for sessionID, evicting expired (or, if full, all) entries.
func (t *stateTable) issue(sessionID string) string {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.now()
	for k, v := range t.m {
		if !v.expires.After(now) {
			delete(t.m, k)
		}
	}
	if len(t.m) >= maxPendingAuth {
		t.m = map[string]pendingState{}
	}
	nonce := access.Token()
	t.m[nonce] = pendingState{sessionID: sessionID, expires: now.Add(t.ttl)}
	return nonce
}

// consume returns the session bound to nonce and invalidates it.
func (t *stateTable) consume(nonce string) (string, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	st, ok := t.m[nonce]
	if !ok {
		return "", false
	}
	delete(t.m, nonce)
	if !st.expires.After(t.now()) {
		return "", false
	}
	return st.sessionID, true
}

func (a *Admin) oauthStart(w http.ResponseWriter, r *http.Request) {
	sess, ok := a.guard.Require(w, r)
	if !ok {
		return
	}
	// ?account= comes from a "Reconnect" button and preselects that account
	// on Google's chooser; only connected accounts are passed through.
	hint := r.URL.Query().Get("account")
	if !a.deps.Store.Has(hint) {
		hint = ""
	}
	http.Redirect(w, r, a.deps.Flow.AuthURL(a.states.issue(sess.ID), hint), http.StatusFound)
}

func (a *Admin) oauthCallback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	sessionID, ok := a.states.consume(q.Get("state"))
	sess := a.guard.Sessions().From(r)
	if !ok || sess == nil || sess.ID != sessionID {
		http.Error(w, "OAuth state is invalid or expired. Start again from /setup.", http.StatusBadRequest)
		return
	}
	sessions := a.guard.Sessions()
	back := func(f access.Flash) {
		sessions.SetFlash(sess, f)
		http.Redirect(w, r, "/setup", http.StatusSeeOther)
	}
	if e := q.Get("error"); e != "" {
		back(access.Flash{Text: "Google sign-in was not completed (" + e + ")"})
		return
	}
	code := q.Get("code")
	if code == "" {
		back(access.Flash{Text: "Google returned no authorization code"})
		return
	}
	grant, err := a.deps.Flow.Exchange(r.Context(), code)
	if err != nil {
		slog.Warn("oauth exchange failed", "err", err)
		back(access.Flash{Text: "Could not connect the account: " + err.Error()})
		return
	}
	reconnect := a.deps.Store.Has(grant.Email)
	if err := a.deps.Store.Put(grant.Email, grant.RefreshToken, grant.Scopes); err != nil {
		slog.Error("store account", "err", err)
		back(access.Flash{Text: "Could not save the account; see the server log"})
		return
	}
	a.deps.Accounts.Forget(grant.Email)
	slog.Info("account connected", "email", grant.Email, "reconnect", reconnect, "scopes", len(grant.Scopes))
	back(connectedFlash(grant, reconnect))
}

// connectedFlash confirms a connect, warning when the user unticked the
// Tasks permission on Google's granular consent screen.
func connectedFlash(g googleauth.Grant, reconnect bool) access.Flash {
	verb := "Connected "
	if reconnect {
		verb = "Reconnected "
	}
	if len(g.Scopes) > 0 && !googleauth.HasScope(g.Scopes, googleauth.TasksScope) {
		return access.Flash{Text: verb + g.Email + " without the Google Tasks permission. " +
			"Reconnect and tick the Tasks box to use the task tools."}
	}
	return access.Flash{OK: true, Text: verb + g.Email + "; calendar and tasks are enabled"}
}
