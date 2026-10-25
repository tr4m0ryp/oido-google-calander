package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tr4m0ryp/oido-google-calander/internal/calendar"
)

type listAccountsArgs struct{}

type listCalendarsArgs struct {
	Account string `json:"account,omitempty" jsonschema:"Account email, or \"all\" (default: the only connected account)"`
}

func (h *handlers) registerAccounts(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "g_calendar_list_accounts",
		Description: "List the Google accounts this server can access. Use these emails as the account parameter.",
		Annotations: readOnly,
	}, h.listAccounts)
}

func (h *handlers) listAccounts(context.Context, *mcp.CallToolRequest, listAccountsArgs) (
	*mcp.CallToolResult, any, error) {
	return jsonResult(map[string]any{"accounts": h.accounts.Emails()})
}

func (h *handlers) registerCalendars(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "g_calendar_list_calendars",
		Description: "List the calendars each account can see (own, shared, subscribed) with id, " +
			"access role, time zone and whether it is primary. Use the id as calendar_id elsewhere.",
		Annotations: readOnly,
	}, h.listCalendars)
}

func (h *handlers) listCalendars(ctx context.Context, _ *mcp.CallToolRequest, args listCalendarsArgs) (
	*mcp.CallToolResult, any, error) {
	res, err := fanOut(ctx, h.accounts, args.Account,
		func(ctx context.Context, c *calendar.Client) ([]calendar.Calendar, error) {
			return c.ListCalendars(ctx)
		})
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{"results": res})
}
