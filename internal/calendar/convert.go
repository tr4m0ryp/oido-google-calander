package calendar

import (
	gcal "google.golang.org/api/calendar/v3"
)

// detail selects how much of an event to carry into the view.
type detail int

const (
	brief detail = iota // list/search results: no description, attendees capped
	full                // get/create/update results
)

const briefAttendeeLimit = 10

func fromAPI(calendarID string, e *gcal.Event, level detail) Event {
	ev := Event{
		ID:               e.Id,
		CalendarID:       calendarID,
		Summary:          e.Summary,
		Status:           e.Status,
		Location:         e.Location,
		HTMLLink:         e.HtmlLink,
		Recurrence:       e.Recurrence,
		RecurringEventID: e.RecurringEventId,
		MeetLink:         meetLink(e),
	}
	if e.Start != nil {
		ev.Start, ev.AllDay, ev.TimeZone = e.Start.DateTime, e.Start.DateTime == "", e.Start.TimeZone
		if ev.AllDay {
			ev.Start = e.Start.Date
		}
	}
	if e.End != nil {
		ev.End = e.End.DateTime
		if ev.End == "" {
			ev.End = e.End.Date
		}
	}
	if e.Organizer != nil {
		ev.Organizer = e.Organizer.Email
	}
	for _, a := range e.Attendees {
		if a.Self {
			ev.MyResponse = a.ResponseStatus
		}
		if level == brief && len(ev.Attendees) >= briefAttendeeLimit {
			continue
		}
		ev.Attendees = append(ev.Attendees, Attendee{
			Email: a.Email, Name: a.DisplayName, Response: a.ResponseStatus,
			Optional: a.Optional, Organizer: a.Organizer, Self: a.Self,
		})
	}
	if level == full {
		ev.Description = e.Description
		ev.Transparency = e.Transparency
		ev.Visibility = e.Visibility
		ev.Reminders = remindersFromAPI(e.Reminders)
	}
	return ev
}

func meetLink(e *gcal.Event) string {
	if e.HangoutLink != "" {
		return e.HangoutLink
	}
	if e.ConferenceData == nil {
		return ""
	}
	for _, ep := range e.ConferenceData.EntryPoints {
		if ep.EntryPointType == "video" {
			return ep.Uri
		}
	}
	return ""
}

func remindersFromAPI(r *gcal.EventReminders) *Reminders {
	if r == nil {
		return nil
	}
	out := &Reminders{UseDefault: r.UseDefault}
	for _, o := range r.Overrides {
		out.Overrides = append(out.Overrides, Reminder{Method: o.Method, Minutes: o.Minutes})
	}
	return out
}

func remindersToAPI(r *Reminders) *gcal.EventReminders {
	if r == nil {
		return nil
	}
	out := &gcal.EventReminders{UseDefault: r.UseDefault}
	if !r.UseDefault {
		// UseDefault=false must be sent explicitly or Google keeps the default.
		out.ForceSendFields = []string{"UseDefault"}
	}
	for _, o := range r.Overrides {
		method := o.Method
		if method == "" {
			method = "popup"
		}
		out.Overrides = append(out.Overrides, &gcal.EventReminder{
			Method: method, Minutes: o.Minutes, ForceSendFields: []string{"Minutes"},
		})
	}
	return out
}

func calendarFromAPI(c *gcal.CalendarListEntry) Calendar {
	return Calendar{
		ID: c.Id, Summary: firstNonEmpty(c.SummaryOverride, c.Summary),
		Description: c.Description, TimeZone: c.TimeZone, AccessRole: c.AccessRole,
		Primary: c.Primary, Selected: c.Selected, Color: c.BackgroundColor,
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
