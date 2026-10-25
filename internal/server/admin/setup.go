package admin

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/tr4m0ryp/oido-google-calander/internal/server/admin/access"
	"github.com/tr4m0ryp/oido-google-calander/internal/server/admin/views"
	"github.com/tr4m0ryp/oido-google-calander/internal/store"
)

// overviewTimeout bounds the Google calls made to render the dashboard.
const overviewTimeout = 8 * time.Second

func (a *Admin) dashboard(w http.ResponseWriter, r *http.Request) {
	sess, ok := a.guard.Require(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), overviewTimeout)
	defer cancel()
	data := views.Dashboard{
		CSRF:     sess.CSRF,
		Accounts: a.overview(ctx, time.Now()),
		Connection: views.Connection{
			MCPURL:      a.deps.Config.ServerURL + "/mcp",
			BearerAuth:  a.deps.Config.MCPAPIKey != "",
			DefaultZone: a.deps.Config.DefaultZone.String(),
			ToolCount:   a.deps.ToolCount,
		},
	}
	if f := a.guard.Sessions().TakeFlash(sess); f != nil {
		data.Flash = &views.Flash{OK: f.OK, Text: f.Text}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := a.views.Dashboard(w, nonceOf(r), data); err != nil {
		slog.Error("render dashboard", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}

// postSession authenticates a state-changing form post: a live session and
// its CSRF token are both required.
func (a *Admin) postSession(w http.ResponseWriter, r *http.Request) (*access.Session, bool) {
	sess, ok := a.guard.Require(w, r)
	if !ok {
		return nil, false
	}
	if !access.SafeEqual(r.PostFormValue("csrf"), sess.CSRF) {
		http.Error(w, "invalid or missing CSRF token; reload /setup and try again", http.StatusForbidden)
		return nil, false
	}
	return sess, true
}

func (a *Admin) remove(w http.ResponseWriter, r *http.Request) {
	sess, ok := a.postSession(w, r)
	if !ok {
		return
	}
	email := r.PostFormValue("email")
	flash := access.Flash{OK: true, Text: "Removed " + email}
	switch err := a.deps.Store.Remove(email); {
	case errors.Is(err, store.ErrNotFound):
		flash = access.Flash{Text: "Account not found"}
	case err != nil:
		slog.Error("remove account", "err", err)
		flash = access.Flash{Text: "Could not remove the account; see the server log"}
	default:
		a.deps.Accounts.Forget(email)
		slog.Info("account removed", "email", email)
	}
	a.guard.Sessions().SetFlash(sess, flash)
	http.Redirect(w, r, "/setup", http.StatusSeeOther)
}

func (a *Admin) logout(w http.ResponseWriter, r *http.Request) {
	sess, ok := a.postSession(w, r)
	if !ok {
		return
	}
	a.guard.Sessions().Destroy(w, sess)
	http.Redirect(w, r, "/setup", http.StatusSeeOther)
}
