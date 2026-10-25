package calendar

import (
	"context"
	"fmt"
	"time"

	gcal "google.golang.org/api/calendar/v3"

	"github.com/tr4m0ryp/oido-google-calander/internal/calendar/timeinput"
)

const (
	defaultMaxResults = 25
	maxMaxResults     = 250
)

// ListQuery selects events in a time range, optionally filtered by text.
type ListQuery struct {
	CalendarID string
	TimeMin    string // default: now
	TimeMax    string // optional; a bare date includes that whole day
	Query      string // free text over title, description, location, attendees
	TimeZone   string // zone for naive inputs and for rendered times
	MaxResults int64
}

// ListEvents expands recurring events and returns them ordered by start.
func (c *Client) ListEvents(ctx context.Context, q ListQuery) ([]Event, error) {
	calID := c.calendarID(q.CalendarID)
	loc, err := c.zoneFor(ctx, calID, q.TimeZone, q.TimeMin, q.TimeMax)
	if err != nil {
		return nil, err
	}
	min := time.Now()
	if q.TimeMin != "" {
		if min, err = timeinput.Parse(q.TimeMin, loc); err != nil {
			return nil, err
		}
	}
	call := c.svc.Events.List(calID).Context(ctx).
		SingleEvents(true).OrderBy("startTime").
		TimeMin(min.Format(time.RFC3339)).
		MaxResults(clampResults(q.MaxResults))
	if q.TimeMax != "" {
		max, err := timeinput.ParseEnd(q.TimeMax, loc)
		if err != nil {
			return nil, err
		}
		if !max.After(min) {
			return nil, fmt.Errorf("time_max must be after time_min")
		}
		call = call.TimeMax(max.Format(time.RFC3339))
	}
	if q.Query != "" {
		call = call.Q(q.Query)
	}
	if q.TimeZone != "" {
		call = call.TimeZone(loc.String())
	}
	res, err := call.Do()
	if err != nil {
		return nil, fmt.Errorf("list events: %w", Explain(err))
	}
	out := make([]Event, 0, len(res.Items))
	for _, item := range res.Items {
		out = append(out, fromAPI(calID, item, brief))
	}
	return out, nil
}

// GetEvent fetches one event with full detail. A non-empty zone renders its
// times in that zone.
func (c *Client) GetEvent(ctx context.Context, calID, eventID, zone string) (Event, error) {
	calID = c.calendarID(calID)
	call := c.svc.Events.Get(calID, eventID).Context(ctx)
	if zone != "" {
		loc, err := timeinput.LoadZone(zone, nil)
		if err != nil {
			return Event{}, err
		}
		call = call.TimeZone(loc.String())
	}
	e, err := call.Do()
	if err != nil {
		return Event{}, fmt.Errorf("get event: %w", Explain(err))
	}
	return fromAPI(calID, e, full), nil
}

func (c *Client) rawEvent(ctx context.Context, calID, eventID string) (*gcal.Event, error) {
	e, err := c.svc.Events.Get(calID, eventID).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("get event: %w", Explain(err))
	}
	return e, nil
}

func clampResults(n int64) int64 {
	switch {
	case n <= 0:
		return defaultMaxResults
	case n > maxMaxResults:
		return maxMaxResults
	}
	return n
}
