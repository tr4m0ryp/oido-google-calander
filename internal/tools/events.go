package tools

import (
	"context"
	"errors"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tr4m0ryp/oido-google-calander/internal/account"
	"github.com/tr4m0ryp/oido-google-calander/internal/calendar"
)

type listEventsArgs struct {
	Account    string `json:"account,omitempty" jsonschema:"Account email, or \"all\" (default: the only connected account)"`
	CalendarID string `json:"calendar_id,omitempty" jsonschema:"Calendar id (default: primary)"`
	TimeMin    string `json:"time_min,omitempty" jsonschema:"Range start (default: now). RFC3339, local 2026-05-15T09:00, or date"`
	TimeMax    string `json:"time_max,omitempty" jsonschema:"Range end; a bare date includes that whole day"`
	Query      string `json:"query,omitempty" jsonschema:"Free-text filter over title, description, location and attendees"`
	TimeZone   string `json:"timezone,omitempty" jsonschema:"IANA zone for naive inputs and returned times (default: calendar's zone)"`
	MaxResults int64  `json:"max_results,omitempty" jsonschema:"Maximum events per account (default 25, max 250)"`
}

type searchEventsArgs struct {
	Query      string `json:"query" jsonschema:"Text to match in title, description, location or attendees"`
	Account    string `json:"account,omitempty" jsonschema:"Account email, or \"all\""`
	CalendarID string `json:"calendar_id,omitempty" jsonschema:"Calendar id (default: primary)"`
	TimeMin    string `json:"time_min,omitempty" jsonschema:"Range start (default: now)"`
	TimeMax    string `json:"time_max,omitempty" jsonschema:"Range end (optional)"`
	TimeZone   string `json:"timezone,omitempty" jsonschema:"IANA zone for naive inputs and returned times"`
	MaxResults int64  `json:"max_results,omitempty" jsonschema:"Maximum events per account (default 25)"`
}

type getEventArgs struct {
	EventID    string `json:"event_id" jsonschema:"Event id from a list or search result"`
	Account    string `json:"account,omitempty" jsonschema:"Account email that owns the event"`
	CalendarID string `json:"calendar_id,omitempty" jsonschema:"Calendar id (default: primary)"`
	TimeZone   string `json:"timezone,omitempty" jsonschema:"IANA zone to render times in"`
}

func (h *handlers) registerEvents(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "g_calendar_list_events",
		Description: "List events in a time range, recurring events expanded, ordered by start. " +
			"Supports account \"all\" to merge an agenda across accounts.",
		Annotations: readOnly,
	}, h.listEvents)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "g_calendar_search_events",
		Description: "Search events by text (title, description, location, attendees), from now onward unless time_min is set.",
		Annotations: readOnly,
	}, h.searchEvents)
	mcp.AddTool(s, &mcp.Tool{
		Name: "g_calendar_get_event",
		Description: "Get one event in full: description, attendees and their responses, " +
			"Meet link, recurrence and reminders.",
		Annotations: readOnly,
	}, h.getEvent)
}

func (h *handlers) listEvents(ctx context.Context, _ *mcp.CallToolRequest, a listEventsArgs) (
	*mcp.CallToolResult, any, error) {
	q := calendar.ListQuery{CalendarID: a.CalendarID, TimeMin: a.TimeMin, TimeMax: a.TimeMax,
		Query: a.Query, TimeZone: a.TimeZone, MaxResults: a.MaxResults}
	return h.runList(ctx, a.Account, q)
}

func (h *handlers) searchEvents(ctx context.Context, _ *mcp.CallToolRequest, a searchEventsArgs) (
	*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(a.Query) == "" {
		return nil, nil, errors.New("query is required")
	}
	q := calendar.ListQuery{CalendarID: a.CalendarID, TimeMin: a.TimeMin, TimeMax: a.TimeMax,
		Query: a.Query, TimeZone: a.TimeZone, MaxResults: a.MaxResults}
	return h.runList(ctx, a.Account, q)
}

func (h *handlers) runList(ctx context.Context, acct string, q calendar.ListQuery) (
	*mcp.CallToolResult, any, error) {
	res, err := fanOut(ctx, h.accounts, acct,
		func(ctx context.Context, c *calendar.Client) ([]calendar.Event, error) {
			return c.ListEvents(ctx, q)
		})
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{"results": res})
}

func (h *handlers) getEvent(ctx context.Context, _ *mcp.CallToolRequest, a getEventArgs) (
	*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(a.EventID) == "" {
		return nil, nil, errors.New("event_id is required")
	}
	c, err := account.One(ctx, h.accounts, a.Account)
	if err != nil {
		return nil, nil, err
	}
	ev, err := c.GetEvent(ctx, a.CalendarID, a.EventID, a.TimeZone)
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{"account": c.Account(), "event": ev})
}
