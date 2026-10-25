// Package tools defines the MCP tool surface: argument schemas, validation,
// multi-account fan-out and JSON results over the calendar package.
package tools

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tr4m0ryp/oido-google-calander/internal/account"
)

// Name is the MCP implementation name advertised to clients.
const Name = "oido-google-calendar"

// Count is the number of tools NewServer registers (shown on the dashboard).
const Count = 12

const instructions = `Google Calendar for one or more connected Google accounts.
Every tool takes an optional "account" (an email from g_calendar_list_accounts);
it may be omitted when only one account is connected. Read-only tools also accept
"all" to fan out across accounts. Times accept RFC3339 with an offset, or a local
"2026-05-15T09:00" read in "timezone" (IANA name; default: the calendar's own zone).
A bare date "2026-05-15" means an all-day event, or the whole day in a range.
Deleting requires confirm:true; call once without it to preview the event.`

// handlers binds tool handlers to an account provider.
type handlers struct {
	accounts account.Provider
}

// NewServer builds an MCP server exposing every calendar tool.
func NewServer(p account.Provider, version string) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: Name, Version: version},
		&mcp.ServerOptions{Instructions: instructions})
	h := &handlers{accounts: p}
	h.registerAccounts(s)
	h.registerCalendars(s)
	h.registerEvents(s)
	h.registerMutations(s)
	h.registerAvailability(s)
	return s
}
