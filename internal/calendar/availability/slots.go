package availability

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// SlotQuery describes what kind of free time to look for.
type SlotQuery struct {
	Window   Interval
	Duration time.Duration
	Location *time.Location
	// WorkStart and WorkEnd bound each day as offsets from local midnight.
	// Both zero means the whole day is usable.
	WorkStart, WorkEnd time.Duration
	// WorkDays limits the search to these weekdays; empty means every day.
	WorkDays []time.Weekday
	// Granularity aligns slot starts (e.g. to :00/:15/:30/:45). Zero disables.
	Granularity time.Duration
	// Buffer is kept free before and after every busy block.
	Buffer   time.Duration
	MaxSlots int
}

// ParseClock parses "HH:MM" into an offset from midnight. "24:00" is allowed
// as an end-of-day bound.
func ParseClock(s string) (time.Duration, error) {
	var h, m int
	if _, err := fmt.Sscanf(strings.TrimSpace(s), "%d:%d", &h, &m); err != nil {
		return 0, fmt.Errorf("invalid clock time %q (want HH:MM)", s)
	}
	if h < 0 || m < 0 || m > 59 || h > 24 || (h == 24 && m != 0) {
		return 0, fmt.Errorf("clock time %q out of range", s)
	}
	return time.Duration(h)*time.Hour + time.Duration(m)*time.Minute, nil
}

// ParseWeekdays parses names like "mon,tue,wed" (any case, 3+ letters).
func ParseWeekdays(names []string) ([]time.Weekday, error) {
	var days []time.Weekday
	for _, raw := range names {
		name := strings.ToLower(strings.TrimSpace(raw))
		found := false
		for d := time.Sunday; d <= time.Saturday; d++ {
			full := strings.ToLower(d.String())
			if len(name) >= 3 && strings.HasPrefix(full, name) {
				days = append(days, d)
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("unknown weekday %q", raw)
		}
	}
	return days, nil
}

func (q SlotQuery) validate() error {
	switch {
	case q.Duration <= 0:
		return errors.New("duration must be positive")
	case !q.Window.End.After(q.Window.Start):
		return errors.New("time_max must be after time_min")
	case q.WorkEnd < q.WorkStart:
		return errors.New("working hours end must not be before start (overnight shifts unsupported)")
	case q.WorkEnd == q.WorkStart && q.WorkStart != 0:
		return errors.New("working hours window is empty")
	}
	return nil
}

func (q SlotQuery) dayAllowed(d time.Weekday) bool {
	if len(q.WorkDays) == 0 {
		return true
	}
	for _, w := range q.WorkDays {
		if w == d {
			return true
		}
	}
	return false
}

// dayWindows yields the usable part of each local day that intersects the window.
func (q SlotQuery) dayWindows() []Interval {
	loc := q.Location
	start := q.Window.Start.In(loc)
	day := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, loc)
	var out []Interval
	for ; day.Before(q.Window.End); day = day.AddDate(0, 0, 1) {
		if !q.dayAllowed(day.Weekday()) {
			continue
		}
		from, to := day, day.AddDate(0, 0, 1)
		if q.WorkStart != 0 || q.WorkEnd != 0 {
			// Wall-clock arithmetic via time.Date keeps hours right across DST.
			from = atClock(day, q.WorkStart)
			to = atClock(day, q.WorkEnd)
		}
		if from.Before(q.Window.Start) {
			from = q.Window.Start
		}
		if to.After(q.Window.End) {
			to = q.Window.End
		}
		if to.After(from) {
			out = append(out, Interval{Start: from, End: to})
		}
	}
	return out
}

func atClock(day time.Time, offset time.Duration) time.Time {
	h, m := int(offset/time.Hour), int(offset%time.Hour/time.Minute)
	return time.Date(day.Year(), day.Month(), day.Day(), h, m, 0, 0, day.Location())
}

// alignUp rounds t up to the next multiple of step on the local wall clock,
// so 30-minute steps land on :00/:30 even in zones offset by half an hour.
func alignUp(t time.Time, step time.Duration) time.Time {
	if step <= 0 {
		return t
	}
	midnight := atClock(t, 0)
	since := t.Sub(midnight)
	rem := since % step
	if rem == 0 {
		return t
	}
	return midnight.Add(since - rem + step)
}

// FindSlots returns free ranges at least q.Duration long, each starting on
// the granularity grid, in chronological order, capped at q.MaxSlots.
func FindSlots(busy []Interval, q SlotQuery) ([]Interval, error) {
	if q.Location == nil {
		q.Location = time.UTC
	}
	if err := q.validate(); err != nil {
		return nil, err
	}
	padded := make([]Interval, len(busy))
	for i, b := range busy {
		padded[i] = Interval{Start: b.Start.Add(-q.Buffer), End: b.End.Add(q.Buffer)}
	}
	merged := Merge(padded)

	var slots []Interval
	for _, day := range q.dayWindows() {
		for _, gap := range Subtract(day, merged) {
			start := alignUp(gap.Start, q.Granularity)
			if gap.End.Sub(start) < q.Duration {
				continue
			}
			slots = append(slots, Interval{Start: start.In(q.Location), End: gap.End.In(q.Location)})
			if q.MaxSlots > 0 && len(slots) >= q.MaxSlots {
				return slots, nil
			}
		}
	}
	return slots, nil
}
