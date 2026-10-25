package tasks

import (
	gtasks "google.golang.org/api/tasks/v1"

	"github.com/tr4m0ryp/oido-google-calander/internal/tasks/due"
)

// Status values as tools report them. Google's own wire values are
// "needsAction" and "completed".
const (
	StatusOpen      = "open"
	StatusCompleted = "completed"
)

const (
	apiOpen      = "needsAction"
	apiCompleted = "completed"
)

// TaskList is one list of tasks ("My Tasks" is the default).
type TaskList struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Updated string `json:"updated,omitempty"`
}

// Task is the compact view tools return. Due is a date (YYYY-MM-DD).
type Task struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Notes       string `json:"notes,omitempty"`
	Status      string `json:"status"`
	Due         string `json:"due,omitempty"`
	CompletedAt string `json:"completed_at,omitempty"`
	Parent      string `json:"parent,omitempty"`
	Position    string `json:"position,omitempty"`
	Updated     string `json:"updated,omitempty"`
	Hidden      bool   `json:"hidden,omitempty"`
	Link        string `json:"link,omitempty"`

	due due.Date
}

// DueDate is the parsed due date (zero when the task has none).
func (t Task) DueDate() due.Date { return t.due }

// Open reports whether the task still needs doing.
func (t Task) Open() bool { return t.Status == StatusOpen }

// WithDue returns a copy of t carrying d as its due date.
func (t Task) WithDue(d due.Date) Task {
	t.due, t.Due = d, d.String()
	return t
}

func fromAPI(t *gtasks.Task) Task {
	out := Task{
		ID: t.Id, Title: t.Title, Notes: t.Notes, Status: StatusOpen,
		Parent: t.Parent, Position: t.Position, Updated: t.Updated,
		Hidden: t.Hidden, Link: t.WebViewLink,
	}
	if t.Status == apiCompleted {
		out.Status = StatusCompleted
	}
	if t.Completed != nil {
		out.CompletedAt = *t.Completed
	}
	// An unreadable due value is reported as undated rather than failing
	// the whole listing; Google only ever sends RFC3339 here.
	if d, err := due.FromAPI(t.Due); err == nil {
		out = out.WithDue(d)
	}
	return out
}
