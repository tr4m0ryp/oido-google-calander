package calendar

import (
	"context"
	"fmt"
	"strings"
	"time"

	gcal "google.golang.org/api/calendar/v3"

	"github.com/tr4m0ryp/oido-google-calander/internal/calendar/availability"
)

// maxFreeBusyItems is Google's per-request calendar limit for freeBusy.query.
const maxFreeBusyItems = 50

// CalendarBusy is the busy time of one calendar, or why it could not be read.
type CalendarBusy struct {
	CalendarID string                  `json:"calendar_id"`
	Busy       []availability.Interval `json:"busy"`
	Error      string                  `json:"error,omitempty"`
}

// FreeBusy reports busy blocks for the given calendars (default: primary).
// Only events marked busy (opaque) count, matching Google's own scheduling.
func (c *Client) FreeBusy(ctx context.Context, calendarIDs []string, window availability.Interval) (
	[]CalendarBusy, error) {
	if len(calendarIDs) == 0 {
		calendarIDs = []string{c.defaultCal}
	}
	if len(calendarIDs) > maxFreeBusyItems {
		return nil, fmt.Errorf("at most %d calendars per free/busy query", maxFreeBusyItems)
	}
	req := &gcal.FreeBusyRequest{
		TimeMin: window.Start.Format(time.RFC3339),
		TimeMax: window.End.Format(time.RFC3339),
	}
	for _, id := range calendarIDs {
		req.Items = append(req.Items, &gcal.FreeBusyRequestItem{Id: id})
	}
	res, err := c.svc.Freebusy.Query(req).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("free/busy query: %w", Explain(err))
	}
	out := make([]CalendarBusy, 0, len(calendarIDs))
	for _, id := range calendarIDs {
		entry := CalendarBusy{CalendarID: id, Busy: []availability.Interval{}}
		cal, ok := c.busyEntry(res.Calendars, id, len(calendarIDs))
		if !ok {
			entry.Error = "calendar missing from response"
			out = append(out, entry)
			continue
		}
		for _, e := range cal.Errors {
			entry.Error = e.Reason
		}
		for _, p := range cal.Busy {
			start, err1 := time.Parse(time.RFC3339, p.Start)
			end, err2 := time.Parse(time.RFC3339, p.End)
			if err1 == nil && err2 == nil {
				entry.Busy = append(entry.Busy, availability.Interval{Start: start, End: end})
			}
		}
		out = append(out, entry)
	}
	return out, nil
}

// busyEntry finds the response entry for a requested id. Google keys entries
// by the id as sent, but tolerate an alias ("primary" answered under the
// account email) so a missing key never silently reads as "free".
func (c *Client) busyEntry(m map[string]gcal.FreeBusyCalendar, id string, requested int) (
	gcal.FreeBusyCalendar, bool) {
	if cal, ok := m[id]; ok {
		return cal, true
	}
	if id == PrimaryID {
		for key, cal := range m {
			if strings.EqualFold(key, c.account) {
				return cal, true
			}
		}
	}
	if requested == 1 && len(m) == 1 {
		for _, cal := range m {
			return cal, true
		}
	}
	return gcal.FreeBusyCalendar{}, false
}
