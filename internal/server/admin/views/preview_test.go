package views

import (
	"os"
	"path/filepath"
	"testing"
)

// TestWritePreviews renders fixture pages for visual review. It only runs
// when VIEWS_PREVIEW_DIR is set:
//
//	VIEWS_PREVIEW_DIR=/tmp/preview go test ./internal/server/admin/views -run Preview
func TestWritePreviews(t *testing.T) {
	dir := os.Getenv("VIEWS_PREVIEW_DIR")
	if dir == "" {
		t.Skip("VIEWS_PREVIEW_DIR not set")
	}
	r, err := New()
	if err != nil {
		t.Fatal(err)
	}
	conn := Connection{MCPURL: "https://calendar.example.com/mcp", BearerAuth: true,
		DefaultZone: "UTC", ToolCount: 12}
	pages := map[string]func(*os.File) error{
		"login.html": func(f *os.File) error { return r.Login(f, "", Login{}) },
		"login-error.html": func(f *os.File) error {
			return r.Login(f, "", Login{Message: "Incorrect password. 2 attempts left before sign-in is temporarily locked."})
		},
		"empty.html": func(f *os.File) error { return r.Dashboard(f, "", Dashboard{Connection: conn}) },
		"dashboard.html": func(f *os.File) error {
			return r.Dashboard(f, "", Dashboard{Flash: &Flash{OK: true, Text: "Connected uva.cavum@gmail.com"},
				Accounts: previewAccounts(), Connection: conn})
		},
	}
	for name, render := range pages {
		f, err := os.Create(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := render(f); err != nil {
			t.Fatal(err)
		}
		f.Close()
	}
}

func previewAccounts() []Account {
	return []Account{
		{
			Email: "moussa.rahmouni@gmail.com", Initial: "M", AddedText: "Oct 25, 2026", TimeZone: "Europe/Amsterdam",
			Calendars: []Chip{
				{Name: "Primary calendar", Color: "#9fe1e7"},
				{Name: "University", Color: "#7bd148"}, {Name: "Training", Color: "#fa573c"},
				{Name: "Holidays in the Netherlands", Color: "#16a765"}, {Name: "Birthdays", Color: "#92e1c0", Tag: "Read only"},
				{Name: "Family", Color: "#f691b2"},
			},
			MoreCount: 2, CalendarCount: 8, TasksEnabled: true,
			Agenda: []AgendaItem{
				{Day: "Today", Time: "14:00", Title: "Thesis supervision meeting", IsToday: true},
				{Day: "Tomorrow", Time: "All day", Title: "Hackathon prep"},
				{Day: "Tue 28 Oct", Time: "09:30", Title: "Weekly sync with the research group about the new evaluation plan"},
				{Day: "Thu 30 Oct", Time: "18:00", Title: "Gym"},
			},
		},
		{
			Email: "uva.cavum@gmail.com", Initial: "U", AddedText: "Oct 12, 2026", TimeZone: "Europe/Amsterdam",
			Calendars:     []Chip{{Name: "Primary calendar", Color: "#4986e7"}, {Name: "Team rota", Color: "#b99aff", Tag: "Shared"}},
			CalendarCount: 2, TasksNeedsReconnect: true,
		},
		{Email: "a.very.long.mailbox.name.for.testing@googlemail.com", Initial: "A", AddedText: "Sep 1, 2026",
			Error: "Google no longer accepts this account's sign-in. Reconnect it to restore access."},
	}
}
