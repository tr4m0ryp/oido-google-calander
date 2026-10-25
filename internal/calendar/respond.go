package calendar

import (
	"context"
	"fmt"
	"strings"

	gcal "google.golang.org/api/calendar/v3"
)

// RespondInput answers an invitation on the account's own attendee entry.
type RespondInput struct {
	CalendarID, EventID string
	Response            string // accepted, declined or tentative
	Comment             string
	SendUpdates         string
}

var responses = map[string]string{
	"accept": "accepted", "accepted": "accepted", "yes": "accepted",
	"decline": "declined", "declined": "declined", "no": "declined",
	"tentative": "tentative", "maybe": "tentative",
}

// Respond sets this account's RSVP on an event it was invited to.
func (c *Client) Respond(ctx context.Context, in RespondInput) (Event, error) {
	status, ok := responses[strings.ToLower(strings.TrimSpace(in.Response))]
	if !ok {
		return Event{}, fmt.Errorf("response must be accepted, declined or tentative")
	}
	if !sendUpdatesValues[in.SendUpdates] {
		return Event{}, fmt.Errorf("send_updates must be all, externalOnly or none")
	}
	calID := c.calendarID(in.CalendarID)
	current, err := c.rawEvent(ctx, calID, in.EventID)
	if err != nil {
		return Event{}, err
	}
	var self *gcal.EventAttendee
	for _, a := range current.Attendees {
		if a.Self || strings.EqualFold(a.Email, c.account) {
			self = a
		}
	}
	if self == nil {
		return Event{}, fmt.Errorf("%s is not on the guest list of this event", c.account)
	}
	self.ResponseStatus = status
	if in.Comment != "" {
		self.Comment = in.Comment
	}
	call := c.svc.Events.Patch(calID, in.EventID, &gcal.Event{Attendees: current.Attendees}).Context(ctx)
	if in.SendUpdates != "" {
		call = call.SendUpdates(in.SendUpdates)
	}
	updated, err := call.Do()
	if err != nil {
		return Event{}, fmt.Errorf("respond to event: %w", Explain(err))
	}
	return fromAPI(calID, updated, full), nil
}
