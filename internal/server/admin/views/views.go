// Package views renders the admin pages: self-contained HTML documents with
// inline CSS and script (nonce-tagged for the CSP), system fonts only and
// no external requests. Secrets never reach these templates.
package views

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"io"
	"strings"
)

//go:embed templates/*.html styles/*.css templates/*.js
var assets embed.FS

// Stylesheet layers, concatenated in cascade order.
var styleLayers = []string{"tokens", "base", "components", "calendar"}

// Page carries what every document needs.
type Page struct {
	Title  string
	Nonce  string
	CSS    template.CSS
	Script template.JS
}

// Login is the sign-in page model.
type Login struct {
	Page
	Message string
	Locked  bool
}

// Flash is a one-shot status message.
type Flash struct {
	OK   bool
	Text string
}

// Chip is one calendar on an account.
type Chip struct {
	Name  string
	Color string // validated #rrggbb or empty
	Tag   string // short qualifier such as "Primary" or "Read only"
}

// AgendaItem is one upcoming event line.
type AgendaItem struct {
	Day     string
	Time    string
	Title   string
	IsToday bool
}

// Account is one connected account with its calendar snapshot.
type Account struct {
	Email     string
	Initial   string
	AddedISO  string
	AddedText string
	TimeZone  string
	Calendars []Chip
	// MoreCount calendars were left out of Calendars to keep the card compact.
	MoreCount     int
	CalendarCount int
	Agenda        []AgendaItem
	Error         string
	// TasksEnabled: the grant includes Google Tasks. TasksNeedsReconnect:
	// it was checked and lacks it. Both false means unknown.
	TasksEnabled        bool
	TasksNeedsReconnect bool
}

// Connection describes how clients reach the MCP endpoint.
type Connection struct {
	MCPURL      string
	BearerAuth  bool
	DefaultZone string
	ToolCount   int
}

// Dashboard is the /setup page model.
type Dashboard struct {
	Page
	CSRF       string
	Flash      *Flash
	Accounts   []Account
	Connection Connection
}

// Renderer holds the parsed templates and the inlined assets.
type Renderer struct {
	login, dashboard *template.Template
	css              template.CSS
	script           template.JS
}

// New parses the embedded templates.
func New() (*Renderer, error) {
	var css strings.Builder
	for _, layer := range styleLayers {
		b, err := assets.ReadFile("styles/" + layer + ".css")
		if err != nil {
			return nil, err
		}
		css.Write(b)
		css.WriteByte('\n')
	}
	script, err := assets.ReadFile("templates/dashboard.js")
	if err != nil {
		return nil, err
	}
	funcs := template.FuncMap{"icon": icon, "emailBreak": emailBreak}
	parse := func(files ...string) (*template.Template, error) {
		paths := []string{"templates/layout.html"}
		for _, f := range files {
			paths = append(paths, "templates/"+f)
		}
		return template.New("layout").Funcs(funcs).ParseFS(assets, paths...)
	}
	r := &Renderer{css: template.CSS(css.String()), script: template.JS(script)}
	if r.login, err = parse("login.html"); err != nil {
		return nil, fmt.Errorf("parse login: %w", err)
	}
	if r.dashboard, err = parse("dashboard.html", "accounts.html", "connection.html"); err != nil {
		return nil, fmt.Errorf("parse dashboard: %w", err)
	}
	return r, nil
}

// Login renders the sign-in page.
func (r *Renderer) Login(w io.Writer, nonce string, d Login) error {
	d.Page = Page{Title: "Sign in · Calendar MCP", Nonce: nonce, CSS: r.css}
	return render(w, r.login, d)
}

// Dashboard renders /setup.
func (r *Renderer) Dashboard(w io.Writer, nonce string, d Dashboard) error {
	d.Page = Page{Title: "Calendar MCP · Setup", Nonce: nonce, CSS: r.css, Script: r.script}
	return render(w, r.dashboard, d)
}

// render buffers so a template error never leaves a half-written page.
func render(w io.Writer, t *template.Template, data any) error {
	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, "layout", data); err != nil {
		return err
	}
	_, err := buf.WriteTo(w)
	return err
}

// emailBreak inserts a line-break opportunity before the "@".
func emailBreak(email string) template.HTML {
	at := strings.LastIndex(email, "@")
	if at <= 0 {
		return template.HTML(template.HTMLEscapeString(email))
	}
	return template.HTML(template.HTMLEscapeString(email[:at]) + "<wbr>" +
		template.HTMLEscapeString(email[at:]))
}
