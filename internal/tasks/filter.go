package tasks

import (
	"fmt"
	"strings"

	"github.com/tr4m0ryp/oido-google-calander/internal/tasks/due"
)

// StatusAll selects open and completed tasks in a Filter.
const StatusAll = "all"

// Filter narrows a listing. Due bounds are dates; a task without a due date
// never matches a filter that sets any due bound.
type Filter struct {
	Status    string   // StatusOpen, StatusCompleted or StatusAll
	DueBefore due.Date // strictly before
	DueAfter  due.Date // strictly after
	DueOn     due.Date // exactly on
}

// NormaliseStatus validates a status argument, mapping Google's
// "needsAction" and the empty default to StatusOpen.
func NormaliseStatus(s string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "open", "needsaction":
		return StatusOpen, nil
	case "completed", "done":
		return StatusCompleted, nil
	case "all", "any":
		return StatusAll, nil
	}
	return "", fmt.Errorf("status must be open, completed or all, not %q", s)
}

// Visibility is the narrowest listing that can satisfy f.
func (f Filter) Visibility(includeHidden bool) Visibility {
	return Visibility{Completed: f.Status != StatusOpen, Hidden: includeHidden}
}

// Match reports whether t passes f.
func (f Filter) Match(t Task) bool {
	if f.Status != StatusAll && f.Status != "" && t.Status != f.Status {
		return false
	}
	d := t.DueDate()
	bounded := !f.DueBefore.IsZero() || !f.DueAfter.IsZero() || !f.DueOn.IsZero()
	if bounded && d.IsZero() {
		return false
	}
	switch {
	case !f.DueBefore.IsZero() && !d.Before(f.DueBefore):
		return false
	case !f.DueAfter.IsZero() && !f.DueAfter.Before(d):
		return false
	case !f.DueOn.IsZero() && d != f.DueOn:
		return false
	}
	return true
}

// Apply keeps the tasks that match f, preserving order.
func (f Filter) Apply(ts []Task) []Task {
	out := make([]Task, 0, len(ts))
	for _, t := range ts {
		if f.Match(t) {
			out = append(out, t)
		}
	}
	return out
}
