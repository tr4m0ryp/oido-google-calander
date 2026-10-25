// Package due converts between user input, Google Tasks due values and
// calendar dates.
//
// The Tasks API stores a due DATE only. It returns RFC3339 values pinned to
// midnight UTC ("2026-10-25T00:00:00.000Z") and discards any time of day
// that is sent, so this package works with plain dates throughout.
package due

import (
	"fmt"
	"strings"
	"time"
)

// Date is a calendar date with no time of day and no zone.
type Date struct {
	Year  int
	Month time.Month
	Day   int
}

// Of returns the date of t as seen in t's own location.
func Of(t time.Time) Date {
	y, m, d := t.Date()
	return Date{y, m, d}
}

// Today is the current date in loc. Near midnight it differs from the UTC
// date, which is why every "today" goes through an explicit zone.
func Today(now time.Time, loc *time.Location) Date {
	return Of(now.In(loc))
}

// IsZero reports whether d is unset (a task without a due date).
func (d Date) IsZero() bool { return d == Date{} }

// String formats d as YYYY-MM-DD, or "" when unset.
func (d Date) String() string {
	if d.IsZero() {
		return ""
	}
	return fmt.Sprintf("%04d-%02d-%02d", d.Year, d.Month, d.Day)
}

// API formats d the way the Tasks API stores due dates.
func (d Date) API() string { return d.String() + "T00:00:00.000Z" }

// Before reports whether d is strictly earlier than o.
func (d Date) Before(o Date) bool { return d.midnight().Before(o.midnight()) }

// AddDays shifts d by n calendar days.
func (d Date) AddDays(n int) Date { return Of(d.midnight().AddDate(0, 0, n)) }

func (d Date) midnight() time.Time { return time.Date(d.Year, d.Month, d.Day, 0, 0, 0, 0, time.UTC) }

// FromAPI reads a due value returned by the Tasks API. The stored instant is
// midnight UTC, so its UTC date is the due date; converting it to a local
// zone first would shift it a day back west of Greenwich.
func FromAPI(s string) (Date, error) {
	if strings.TrimSpace(s) == "" {
		return Date{}, nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return Date{}, fmt.Errorf("unexpected due value %q from Google Tasks: %w", s, err)
	}
	return Of(t.UTC()), nil
}

var naiveLayouts = []string{"2006-01-02T15:04:05", "2006-01-02T15:04", "2006-01-02 15:04"}

// Parse reads a due date from user input, relative to now in loc:
//   - "today", "tomorrow", "yesterday"
//   - a date, "2026-10-25"
//   - a timestamp with offset, reduced to its date in loc
//   - a local timestamp without offset, reduced to the date as written
//
// Any time of day is dropped because Google Tasks cannot store it.
func Parse(s string, loc *time.Location, now time.Time) (Date, error) {
	in := strings.ToLower(strings.TrimSpace(s))
	today := Today(now, loc)
	switch in {
	case "":
		return Date{}, fmt.Errorf("due date is empty")
	case "today":
		return today, nil
	case "tomorrow":
		return today.AddDays(1), nil
	case "yesterday":
		return today.AddDays(-1), nil
	}
	if t, err := time.Parse("2006-01-02", in); err == nil {
		return Of(t), nil
	}
	up := strings.ToUpper(in) // undo the lowercasing of the "T" and "Z" markers
	if t, err := time.Parse(time.RFC3339, up); err == nil {
		return Of(t.In(loc)), nil
	}
	for _, layout := range naiveLayouts {
		if t, err := time.Parse(layout, up); err == nil {
			return Of(t), nil
		}
	}
	return Date{}, fmt.Errorf("cannot read due date %q; use YYYY-MM-DD, today or tomorrow", s)
}
