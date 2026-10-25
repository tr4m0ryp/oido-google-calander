package calendar

import (
	"context"
	"fmt"
	"strings"

	gcal "google.golang.org/api/calendar/v3"

	"github.com/tr4m0ryp/oido-google-calander/internal/calendar/timeinput"
)

// EventPatch changes selected fields; nil pointers and empty slices are left alone.
type EventPatch struct {
	CalendarID      string
	EventID         string
	Summary         *string
	Description     *string
	Location        *string
	Start, End      string // both required when moving an event
	TimeZone        string
	Attendees       []string // replaces the guest list when non-nil
	AddAttendees    []string
	RemoveAttendees []string
	Recurrence      []string
	Reminders       *Reminders
	AddMeet         bool
	Transparency    string
	Visibility      string
	SendUpdates     string
}

// Update applies a patch and returns the updated event.
func (c *Client) Update(ctx context.Context, p EventPatch) (Event, error) {
	if !sendUpdatesValues[p.SendUpdates] {
		return Event{}, fmt.Errorf("send_updates must be all, externalOnly or none")
	}
	calID := c.calendarID(p.CalendarID)
	current, err := c.rawEvent(ctx, calID, p.EventID)
	if err != nil {
		return Event{}, err
	}
	patch := &gcal.Event{Reminders: remindersToAPI(p.Reminders), Transparency: p.Transparency,
		Visibility: p.Visibility, Recurrence: normaliseRecurrence(p.Recurrence)}
	setString(&patch.Summary, &patch.ForceSendFields, "Summary", p.Summary)
	setString(&patch.Description, &patch.ForceSendFields, "Description", p.Description)
	setString(&patch.Location, &patch.ForceSendFields, "Location", p.Location)

	if p.Start != "" || p.End != "" {
		if p.Start == "" || p.End == "" {
			return Event{}, fmt.Errorf("moving an event needs both start_time and end_time")
		}
		loc, err := c.zoneFor(ctx, calID, firstNonEmpty(p.TimeZone, zoneOf(current)), p.Start, p.End)
		if err != nil {
			return Event{}, err
		}
		if patch.Start, patch.End, err = timeinput.EventSpan(p.Start, p.End, loc, 0); err != nil {
			return Event{}, err
		}
	}
	if p.Attendees != nil || len(p.AddAttendees) > 0 || len(p.RemoveAttendees) > 0 {
		patch.Attendees = mergeAttendees(current.Attendees, p)
		if len(patch.Attendees) == 0 {
			patch.NullFields = append(patch.NullFields, "Attendees")
		}
	}
	call := c.svc.Events.Patch(calID, p.EventID, patch).Context(ctx)
	if p.AddMeet && meetLink(current) == "" {
		patch.ConferenceData = meetRequest()
		call = call.ConferenceDataVersion(1)
	}
	if p.SendUpdates != "" {
		call = call.SendUpdates(p.SendUpdates)
	}
	updated, err := call.Do()
	if err != nil {
		return Event{}, fmt.Errorf("update event: %w", Explain(err))
	}
	return fromAPI(calID, updated, full), nil
}

// Delete removes an event. Callers gate this behind explicit confirmation.
func (c *Client) Delete(ctx context.Context, calID, eventID, sendUpdates string) (Event, error) {
	if !sendUpdatesValues[sendUpdates] {
		return Event{}, fmt.Errorf("send_updates must be all, externalOnly or none")
	}
	calID = c.calendarID(calID)
	current, err := c.rawEvent(ctx, calID, eventID)
	if err != nil {
		return Event{}, err
	}
	call := c.svc.Events.Delete(calID, eventID).Context(ctx)
	if sendUpdates != "" {
		call = call.SendUpdates(sendUpdates)
	}
	if err := call.Do(); err != nil {
		return Event{}, fmt.Errorf("delete event: %w", Explain(err))
	}
	return fromAPI(calID, current, brief), nil
}

// Preview returns the brief view of an event, used to show what a
// destructive call would affect before it is confirmed.
func (c *Client) Preview(ctx context.Context, calID, eventID string) (Event, error) {
	calID = c.calendarID(calID)
	e, err := c.rawEvent(ctx, calID, eventID)
	if err != nil {
		return Event{}, err
	}
	return fromAPI(calID, e, brief), nil
}

func setString(dst *string, force *[]string, field string, v *string) {
	if v == nil {
		return
	}
	*dst = *v
	if *v == "" {
		*force = append(*force, field)
	}
}

func zoneOf(e *gcal.Event) string {
	if e.Start != nil {
		return e.Start.TimeZone
	}
	return ""
}

func mergeAttendees(current []*gcal.EventAttendee, p EventPatch) []*gcal.EventAttendee {
	list := current
	if p.Attendees != nil {
		list = attendeesToAPI(p.Attendees, nil)
	}
	remove := map[string]bool{}
	for _, e := range p.RemoveAttendees {
		remove[strings.ToLower(strings.TrimSpace(e))] = true
	}
	out := make([]*gcal.EventAttendee, 0, len(list)+len(p.AddAttendees))
	seen := map[string]bool{}
	for _, a := range list {
		key := strings.ToLower(a.Email)
		if !remove[key] && !seen[key] {
			seen[key] = true
			out = append(out, a)
		}
	}
	for _, a := range attendeesToAPI(p.AddAttendees, nil) {
		if key := strings.ToLower(a.Email); !seen[key] && !remove[key] {
			seen[key] = true
			out = append(out, a)
		}
	}
	return out
}
