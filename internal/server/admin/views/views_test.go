package views

import (
	"strings"
	"testing"
)

func TestRenderLogin(t *testing.T) {
	r, err := New()
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	if err := r.Login(&b, "n0nce", Login{Message: "Incorrect <password>", Locked: true}); err != nil {
		t.Fatal(err)
	}
	html := b.String()
	for _, want := range []string{"<!DOCTYPE html>", `<style nonce="n0nce">`, "--brand-500",
		"Incorrect &lt;password&gt;", `action="/setup/login"`, "disabled"} {
		if !strings.Contains(html, want) {
			t.Errorf("login page lacks %q", want)
		}
	}
	if strings.Contains(html, "<script") {
		t.Error("login page should not carry the dashboard script")
	}
}

func TestRenderDashboard(t *testing.T) {
	r, err := New()
	if err != nil {
		t.Fatal(err)
	}
	data := Dashboard{
		CSRF:  "tok\"en",
		Flash: &Flash{OK: true, Text: "Connected a@example.com"},
		Accounts: []Account{
			{Email: "a@example.com", Initial: "A", AddedText: "Oct 25, 2026", TimeZone: "Europe/Amsterdam",
				Calendars: []Chip{{Name: "Work <team>", Color: "#9fe1e7", Tag: "Primary"}}, CalendarCount: 1,
				Agenda: []AgendaItem{{Day: "Today", Time: "09:00", Title: "Standup", IsToday: true}}, TasksEnabled: true},
			{Email: "b@example.com", Initial: "B", Error: "Reconnect needed"},
			{Email: "c@example.com", Initial: "C", TasksNeedsReconnect: true},
		},
		Connection: Connection{MCPURL: "https://cal.example.test/mcp", BearerAuth: true, DefaultZone: "UTC", ToolCount: 12},
	}
	var b strings.Builder
	if err := r.Dashboard(&b, "n0nce", data); err != nil {
		t.Fatal(err)
	}
	html := b.String()
	for _, want := range []string{`<script nonce="n0nce">`, "a<wbr>@example.com", "Work &lt;team&gt;",
		`fill="#9fe1e7"`, "Standup", "Reconnect needed", `value="tok&#34;en"`,
		"https://cal.example.test/mcp", "Connected a@example.com", "12 tools",
		`<span class="badge badge-ok">Tasks</span>`, `<span class="badge badge-warn">Tasks off</span>`,
		"Reconnect to enable Tasks", `href="/oauth/start?account=c%40example.com"`} {
		if !strings.Contains(html, want) {
			t.Errorf("dashboard lacks %q", want)
		}
	}
}

func TestRenderEmptyDashboard(t *testing.T) {
	r, _ := New()
	var b strings.Builder
	if err := r.Dashboard(&b, "n", Dashboard{Connection: Connection{DefaultZone: "UTC"}}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "No Google accounts yet") || !strings.Contains(b.String(), "Anyone with the URL") {
		t.Error("empty state or auth warning missing")
	}
}

func TestTasksReconnectOnlyWhenNeeded(t *testing.T) {
	r, _ := New()
	var b strings.Builder
	data := Dashboard{Accounts: []Account{{Email: "a@example.com", Initial: "A", TasksEnabled: true}},
		Connection: Connection{DefaultZone: "UTC"}}
	if err := r.Dashboard(&b, "n", data); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(b.String(), "Reconnect to enable Tasks") {
		t.Error("reconnect prompt shown for an account that already has Tasks")
	}
}
