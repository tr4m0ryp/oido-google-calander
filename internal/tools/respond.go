package tools

import (
	"context"
	"errors"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tr4m0ryp/oido-google-calander/internal/account"
	"github.com/tr4m0ryp/oido-google-calander/internal/calendar"
)

type respondArgs struct {
	EventID     string `json:"event_id"`
	Response    string `json:"response" jsonschema:"accepted, declined or tentative"`
	Comment     string `json:"comment,omitempty" jsonschema:"Optional note to the organizer"`
	Account     string `json:"account,omitempty" jsonschema:"The invited account"`
	CalendarID  string `json:"calendar_id,omitempty"`
	SendUpdates string `json:"send_updates,omitempty" jsonschema:"Notify the organizer: all, externalOnly or none"`
}

func (h *handlers) registerRespond(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "g_calendar_respond_to_event",
		Description: "Accept, decline or tentatively accept an invitation as the given account.",
		Annotations: &mcp.ToolAnnotations{IdempotentHint: true, OpenWorldHint: ptr(true)},
	}, h.respond)
}

func (h *handlers) respond(ctx context.Context, _ *mcp.CallToolRequest, a respondArgs) (
	*mcp.CallToolResult, any, error) {
	if a.EventID == "" {
		return nil, nil, errors.New("event_id is required")
	}
	c, err := account.One(ctx, h.accounts, a.Account)
	if err != nil {
		return nil, nil, err
	}
	ev, err := c.Respond(ctx, calendar.RespondInput{
		CalendarID: a.CalendarID, EventID: a.EventID, Response: a.Response,
		Comment: a.Comment, SendUpdates: a.SendUpdates,
	})
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{"account": c.Account(), "my_response": ev.MyResponse, "event": ev})
}
