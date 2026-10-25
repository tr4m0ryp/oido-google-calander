// Package timeinput turns the loosely formatted times an agent supplies into
// unambiguous instants and Google Calendar date-times.
//
// Accepted forms: RFC3339 with an offset ("2026-05-15T09:00:00+02:00", "...Z"),
// naive wall-clock times ("2026-05-15T09:00", "2026-05-15 09:00:00") that are
// read in a caller-supplied zone, and bare dates ("2026-05-15") for all-day use.
package timeinput

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"google.golang.org/api/calendar/v3"
)

const dateLayout = "2006-01-02"

// Wall-clock layouts tried, in order, when the input carries no offset.
var naiveLayouts = []string{
	"2006-01-02T15:04:05",
	"2006-01-02T15:04",
	"2006-01-02 15:04:05",
	"2006-01-02 15:04",
}

// ErrEmpty reports a missing time value.
var ErrEmpty = errors.New("time value is empty")

// LoadZone resolves an IANA zone name, falling back to def when name is empty.
func LoadZone(name string, def *time.Location) (*time.Location, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		if def == nil {
			return time.UTC, nil
		}
		return def, nil
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, fmt.Errorf("unknown timezone %q (use an IANA name such as Europe/Amsterdam)", name)
	}
	return loc, nil
}

// IsDate reports whether s is a bare calendar date (YYYY-MM-DD).
func IsDate(s string) bool {
	_, err := time.Parse(dateLayout, strings.TrimSpace(s))
	return err == nil
}

// HasOffset reports whether s is a full RFC3339 timestamp, i.e. it pins its own zone.
func HasOffset(s string) bool {
	_, err := time.Parse(time.RFC3339, strings.TrimSpace(s))
	return err == nil
}

// Parse returns the instant s denotes. Naive times and bare dates (midnight)
// are read in loc; RFC3339 input keeps its own offset.
func Parse(s string, loc *time.Location) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, ErrEmpty
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	if loc == nil {
		loc = time.UTC
	}
	for _, layout := range naiveLayouts {
		if t, err := time.ParseInLocation(layout, s, loc); err == nil {
			return t, nil
		}
	}
	if t, err := time.ParseInLocation(dateLayout, s, loc); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf(
		"cannot parse time %q: use RFC3339 (2026-05-15T09:00:00+02:00), "+
			"local 2026-05-15T09:00 with a timezone, or a date 2026-05-15", s)
}

// ParseEnd is Parse for the end of a range: a bare date means "through the
// end of that day", so it resolves to midnight of the following day.
func ParseEnd(s string, loc *time.Location) (time.Time, error) {
	t, err := Parse(s, loc)
	if err != nil {
		return t, err
	}
	if IsDate(s) {
		return t.AddDate(0, 0, 1), nil
	}
	return t, nil
}

// EventSpan builds the start and end EventDateTime for a new or moved event.
//
// Bare dates produce an all-day event (end exclusive, defaulting to one day).
// Timed input is normalised to RFC3339 in loc and tagged with the zone name so
// recurring events expand in local time across DST changes. An empty end
// defaults to start + defaultDuration.
func EventSpan(start, end string, loc *time.Location, defaultDuration time.Duration) (
	*calendar.EventDateTime, *calendar.EventDateTime, error) {
	if loc == nil {
		loc = time.UTC
	}
	if IsDate(start) {
		return allDaySpan(start, end)
	}
	from, err := Parse(start, loc)
	if err != nil {
		return nil, nil, err
	}
	to := from.Add(defaultDuration)
	if strings.TrimSpace(end) != "" {
		if to, err = Parse(end, loc); err != nil {
			return nil, nil, err
		}
	}
	if !to.After(from) {
		return nil, nil, fmt.Errorf("end %s must be after start %s",
			to.Format(time.RFC3339), from.Format(time.RFC3339))
	}
	return timed(from, loc), timed(to, loc), nil
}

func allDaySpan(start, end string) (*calendar.EventDateTime, *calendar.EventDateTime, error) {
	from, _ := time.Parse(dateLayout, strings.TrimSpace(start))
	to := from.AddDate(0, 0, 1)
	if strings.TrimSpace(end) != "" {
		parsed, err := time.Parse(dateLayout, strings.TrimSpace(end))
		if err != nil {
			return nil, nil, fmt.Errorf("all-day event: end %q must also be a date (YYYY-MM-DD)", end)
		}
		to = parsed
	}
	if !to.After(from) {
		return nil, nil, fmt.Errorf("all-day end date is exclusive and must be after %s", start)
	}
	return &calendar.EventDateTime{Date: from.Format(dateLayout)},
		&calendar.EventDateTime{Date: to.Format(dateLayout)}, nil
}

func timed(t time.Time, loc *time.Location) *calendar.EventDateTime {
	return &calendar.EventDateTime{
		DateTime: t.In(loc).Format(time.RFC3339),
		TimeZone: loc.String(),
	}
}
