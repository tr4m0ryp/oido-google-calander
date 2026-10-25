package tools

import (
	"context"
	"errors"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tr4m0ryp/oido-google-calander/internal/account"
	"github.com/tr4m0ryp/oido-google-calander/internal/calendar"
)

type reminderArgs struct {
	UseDefault bool                `json:"use_default,omitempty" jsonschema:"Use the calendar's default reminders"`
	Overrides  []calendar.Reminder `json:"overrides,omitempty" jsonschema:"Custom reminders: method popup|email, minutes before start"`
}

func (r *reminderArgs) toCalendar() *calendar.Reminders {
	if r == nil {
		return nil
	}
	return &calendar.Reminders{UseDefault: r.UseDefault, Overrides: r.Overrides}
}

type createEventArgs struct {
	Summary      string        `json:"summary" jsonschema:"Event title"`
	Start        string        `json:"start_time" jsonschema:"Start: RFC3339, local 2026-05-15T09:00 (read in timezone), or a date for all-day"`
	End          string        `json:"end_time,omitempty" jsonschema:"End (default: start + 1h, or one day for all-day; all-day end is exclusive)"`
	Account      string        `json:"account,omitempty" jsonschema:"Account email to create the event as"`
	CalendarID   string        `json:"calendar_id,omitempty" jsonschema:"Calendar id (default: primary)"`
	TimeZone     string        `json:"timezone,omitempty" jsonschema:"IANA zone for naive times and recurrence (default: calendar's zone)"`
	Description  string        `json:"description,omitempty"`
	Location     string        `json:"location,omitempty"`
	Attendees    []string      `json:"attendees,omitempty" jsonschema:"Guest emails"`
	Optional     []string      `json:"optional_attendees,omitempty" jsonschema:"Optional guest emails"`
	Recurrence   []string      `json:"recurrence,omitempty" jsonschema:"RFC5545 lines, e.g. RRULE:FREQ=WEEKLY;BYDAY=MO;COUNT=10"`
	Reminders    *reminderArgs `json:"reminders,omitempty"`
	AddMeet      bool          `json:"add_meet,omitempty" jsonschema:"Create a Google Meet link"`
	Transparency string        `json:"transparency,omitempty" jsonschema:"opaque (busy, default) or transparent (free)"`
	Visibility   string        `json:"visibility,omitempty" jsonschema:"default, public, private or confidential"`
	SendUpdates  string        `json:"send_updates,omitempty" jsonschema:"Who gets invitation emails: all, externalOnly or none"`
}

type updateEventArgs struct {
	EventID         string        `json:"event_id"`
	Account         string        `json:"account,omitempty"`
	CalendarID      string        `json:"calendar_id,omitempty"`
	Summary         *string       `json:"summary,omitempty"`
	Description     *string       `json:"description,omitempty" jsonschema:"New description; empty string clears it"`
	Location        *string       `json:"location,omitempty" jsonschema:"New location; empty string clears it"`
	Start           string        `json:"start_time,omitempty" jsonschema:"New start (give end_time too)"`
	End             string        `json:"end_time,omitempty" jsonschema:"New end (give start_time too)"`
	TimeZone        string        `json:"timezone,omitempty"`
	Attendees       []string      `json:"attendees,omitempty" jsonschema:"Replace the whole guest list"`
	AddAttendees    []string      `json:"add_attendees,omitempty"`
	RemoveAttendees []string      `json:"remove_attendees,omitempty"`
	Recurrence      []string      `json:"recurrence,omitempty" jsonschema:"Replace recurrence rules"`
	Reminders       *reminderArgs `json:"reminders,omitempty"`
	AddMeet         bool          `json:"add_meet,omitempty" jsonschema:"Attach a Google Meet link if missing"`
	Transparency    string        `json:"transparency,omitempty"`
	Visibility      string        `json:"visibility,omitempty"`
	SendUpdates     string        `json:"send_updates,omitempty" jsonschema:"all, externalOnly or none"`
}

type deleteEventArgs struct {
	EventID     string `json:"event_id"`
	Confirm     bool   `json:"confirm,omitempty" jsonschema:"Must be true to delete; without it the event is only previewed"`
	Account     string `json:"account,omitempty"`
	CalendarID  string `json:"calendar_id,omitempty"`
	SendUpdates string `json:"send_updates,omitempty" jsonschema:"Who gets cancellation emails: all, externalOnly or none"`
}

type quickAddArgs struct {
	Text        string `json:"text" jsonschema:"Natural language, e.g. \"Lunch with Sam Friday 1pm at Cafe Nero\""`
	Account     string `json:"account,omitempty"`
	CalendarID  string `json:"calendar_id,omitempty"`
	SendUpdates string `json:"send_updates,omitempty" jsonschema:"all, externalOnly or none"`
}

func (h *handlers) registerMutations(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "g_calendar_create_event",
		Description: "Create an event with optional guests, reminders, recurrence and a Google Meet link. " +
			"A date-only start makes an all-day event.",
		Annotations: &mcp.ToolAnnotations{OpenWorldHint: ptr(true)},
	}, h.createEvent)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "g_calendar_update_event",
		Description: "Change fields of an existing event; omitted fields stay as they are.",
		Annotations: &mcp.ToolAnnotations{IdempotentHint: true, OpenWorldHint: ptr(true)},
	}, h.updateEvent)
	mcp.AddTool(s, &mcp.Tool{
		Name: "g_calendar_delete_event",
		Description: "Delete an event. Without confirm:true it only previews the event; " +
			"confirm with the user before passing confirm:true.",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: ptr(true), OpenWorldHint: ptr(true)},
	}, h.deleteEvent)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "g_calendar_quick_add",
		Description: "Create an event from a natural-language sentence using Google's quick-add parser.",
		Annotations: &mcp.ToolAnnotations{OpenWorldHint: ptr(true)},
	}, h.quickAdd)
}

func (h *handlers) createEvent(ctx context.Context, _ *mcp.CallToolRequest, a createEventArgs) (
	*mcp.CallToolResult, any, error) {
	c, err := account.One(ctx, h.accounts, a.Account)
	if err != nil {
		return nil, nil, err
	}
	ev, err := c.Create(ctx, calendar.NewEvent{
		CalendarID: a.CalendarID, Summary: a.Summary, Start: a.Start, End: a.End,
		TimeZone: a.TimeZone, Description: a.Description, Location: a.Location,
		Attendees: a.Attendees, Optional: a.Optional, Recurrence: a.Recurrence,
		Reminders: a.Reminders.toCalendar(), AddMeet: a.AddMeet,
		Transparency: a.Transparency, Visibility: a.Visibility, SendUpdates: a.SendUpdates,
	})
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{"account": c.Account(), "created": ev})
}

func (h *handlers) updateEvent(ctx context.Context, _ *mcp.CallToolRequest, a updateEventArgs) (
	*mcp.CallToolResult, any, error) {
	if a.EventID == "" {
		return nil, nil, errors.New("event_id is required")
	}
	c, err := account.One(ctx, h.accounts, a.Account)
	if err != nil {
		return nil, nil, err
	}
	ev, err := c.Update(ctx, calendar.EventPatch{
		CalendarID: a.CalendarID, EventID: a.EventID, Summary: a.Summary,
		Description: a.Description, Location: a.Location, Start: a.Start, End: a.End,
		TimeZone: a.TimeZone, Attendees: a.Attendees, AddAttendees: a.AddAttendees,
		RemoveAttendees: a.RemoveAttendees, Recurrence: a.Recurrence,
		Reminders: a.Reminders.toCalendar(), AddMeet: a.AddMeet,
		Transparency: a.Transparency, Visibility: a.Visibility, SendUpdates: a.SendUpdates,
	})
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{"account": c.Account(), "updated": ev})
}

func (h *handlers) deleteEvent(ctx context.Context, _ *mcp.CallToolRequest, a deleteEventArgs) (
	*mcp.CallToolResult, any, error) {
	if a.EventID == "" {
		return nil, nil, errors.New("event_id is required")
	}
	c, err := account.One(ctx, h.accounts, a.Account)
	if err != nil {
		return nil, nil, err
	}
	if !a.Confirm {
		ev, err := c.Preview(ctx, a.CalendarID, a.EventID)
		if err != nil {
			return nil, nil, err
		}
		return jsonResult(map[string]any{
			"deleted": false, "account": c.Account(), "event": ev,
			"note": "Not deleted. Confirm with the user, then call again with confirm:true.",
		})
	}
	ev, err := c.Delete(ctx, a.CalendarID, a.EventID, a.SendUpdates)
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{"deleted": true, "account": c.Account(), "event": ev})
}

func (h *handlers) quickAdd(ctx context.Context, _ *mcp.CallToolRequest, a quickAddArgs) (
	*mcp.CallToolResult, any, error) {
	c, err := account.One(ctx, h.accounts, a.Account)
	if err != nil {
		return nil, nil, err
	}
	ev, err := c.QuickAdd(ctx, a.CalendarID, a.Text, a.SendUpdates)
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{"account": c.Account(), "created": ev})
}
