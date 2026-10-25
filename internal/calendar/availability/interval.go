// Package availability computes free time from busy intervals: merging busy
// blocks across calendars and accounts, and carving bookable slots out of a
// window under working-hours constraints. It is pure and has no Google types.
package availability

import (
	"sort"
	"time"
)

// Interval is a half-open time range [Start, End).
type Interval struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// Duration is the length of the interval.
func (iv Interval) Duration() time.Duration { return iv.End.Sub(iv.Start) }

// Merge sorts intervals and coalesces overlapping or touching ones. Empty and
// inverted intervals are dropped. The input slice is not modified.
func Merge(in []Interval) []Interval {
	busy := make([]Interval, 0, len(in))
	for _, iv := range in {
		if iv.End.After(iv.Start) {
			busy = append(busy, iv)
		}
	}
	sort.Slice(busy, func(i, j int) bool { return busy[i].Start.Before(busy[j].Start) })

	out := busy[:0:0]
	for _, iv := range busy {
		last := len(out) - 1
		if last >= 0 && !iv.Start.After(out[last].End) {
			if iv.End.After(out[last].End) {
				out[last].End = iv.End
			}
			continue
		}
		out = append(out, iv)
	}
	return out
}

// Subtract returns the parts of window not covered by busy, which must be
// merged (see Merge).
func Subtract(window Interval, busy []Interval) []Interval {
	var free []Interval
	cursor := window.Start
	for _, b := range busy {
		if !b.End.After(cursor) {
			continue
		}
		if !b.Start.Before(window.End) {
			break
		}
		if b.Start.After(cursor) {
			free = append(free, Interval{Start: cursor, End: b.Start})
		}
		cursor = b.End
	}
	if window.End.After(cursor) {
		free = append(free, Interval{Start: cursor, End: window.End})
	}
	return free
}
