package calendar

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	gcal "google.golang.org/api/calendar/v3"

	"github.com/tr4m0ryp/oido-google-calander/internal/calendar/timeinput"
)

const defaultEventDuration = time.Hour

// SendUpdates values accepted by Google: who gets notification emails.
var sendUpdatesValues = map[string]bool{"": true, "all": true, "externalOnly": true, "none": true}

// NewEvent describes an event to create.
type NewEvent struct {
	CalendarID   string
	Summary      string
	Start, End   string // see timeinput.EventSpan; a bare date makes an all-day event
	TimeZone     string
	Description  string
	Location     string
	Attendees    []string
	Optional     []string
	Recurrence   []string // RRULE/EXDATE/RDATE lines, e.g. "RRULE:FREQ=WEEKLY;COUNT=5"
	Reminders    *Reminders
	AddMeet      bool
	Transparency string // "opaque" (busy) or "transparent" (free)
	Visibility   string
	SendUpdates  string
}

// Create inserts an event and returns it as stored by Google.
func (c *Client) Create(ctx context.Context, in NewEvent) (Event, error) {
	if strings.TrimSpace(in.Summary) == "" {
		return Event{}, fmt.Errorf("summary is required")
	}
	if !sendUpdatesValues[in.SendUpdates] {
		return Event{}, fmt.Errorf("send_updates must be all, externalOnly or none")
	}
	calID := c.calendarID(in.CalendarID)
	loc, err := c.zoneFor(ctx, calID, in.TimeZone, in.Start, in.End)
	if err != nil {
		return Event{}, err
	}
	start, end, err := timeinput.EventSpan(in.Start, in.End, loc, defaultEventDuration)
	if err != nil {
		return Event{}, err
	}
	ev := &gcal.Event{
		Summary: in.Summary, Description: in.Description, Location: in.Location,
		Start: start, End: end, Recurrence: normaliseRecurrence(in.Recurrence),
		Attendees:    attendeesToAPI(in.Attendees, in.Optional),
		Reminders:    remindersToAPI(in.Reminders),
		Transparency: in.Transparency, Visibility: in.Visibility,
	}
	call := c.svc.Events.Insert(calID, ev).Context(ctx)
	if in.AddMeet {
		ev.ConferenceData = meetRequest()
		call = call.ConferenceDataVersion(1)
	}
	if in.SendUpdates != "" {
		call = call.SendUpdates(in.SendUpdates)
	}
	created, err := call.Do()
	if err != nil {
		return Event{}, fmt.Errorf("create event: %w", Explain(err))
	}
	return fromAPI(calID, created, full), nil
}

// QuickAdd creates an event from natural language ("Lunch with Sam Friday 1pm").
// Google parses the text in the calendar's own zone.
func (c *Client) QuickAdd(ctx context.Context, calID, text, sendUpdates string) (Event, error) {
	if strings.TrimSpace(text) == "" {
		return Event{}, fmt.Errorf("text is required")
	}
	if !sendUpdatesValues[sendUpdates] {
		return Event{}, fmt.Errorf("send_updates must be all, externalOnly or none")
	}
	calID = c.calendarID(calID)
	call := c.svc.Events.QuickAdd(calID, text).Context(ctx)
	if sendUpdates != "" {
		call = call.SendUpdates(sendUpdates)
	}
	created, err := call.Do()
	if err != nil {
		return Event{}, fmt.Errorf("quick add: %w", Explain(err))
	}
	return fromAPI(calID, created, full), nil
}

func attendeesToAPI(required, optional []string) []*gcal.EventAttendee {
	var out []*gcal.EventAttendee
	seen := map[string]bool{}
	add := func(email string, opt bool) {
		email = strings.TrimSpace(email)
		key := strings.ToLower(email)
		if email == "" || seen[key] {
			return
		}
		seen[key] = true
		out = append(out, &gcal.EventAttendee{Email: email, Optional: opt})
	}
	for _, e := range required {
		add(e, false)
	}
	for _, e := range optional {
		add(e, true)
	}
	return out
}

// normaliseRecurrence accepts bare "FREQ=..." rules and prefixes "RRULE:".
func normaliseRecurrence(lines []string) []string {
	var out []string
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		if strings.HasPrefix(strings.ToUpper(l), "FREQ=") {
			l = "RRULE:" + l
		}
		out = append(out, l)
	}
	return out
}

func meetRequest() *gcal.ConferenceData {
	return &gcal.ConferenceData{
		CreateRequest: &gcal.CreateConferenceRequest{
			RequestId:             randomID(),
			ConferenceSolutionKey: &gcal.ConferenceSolutionKey{Type: "hangoutsMeet"},
		},
	}
}

func randomID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
