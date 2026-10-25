// Package tasktools defines the Google Tasks MCP tools: task lists, tasks,
// and the daily rollover. They are registered only for providers that can
// reach Google Tasks (OAuth accounts, not the stdio service account).
package tasktools

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tr4m0ryp/oido-google-calander/internal/account"
	"github.com/tr4m0ryp/oido-google-calander/internal/calendar/timeinput"
	"github.com/tr4m0ryp/oido-google-calander/internal/tasks"
	"github.com/tr4m0ryp/oido-google-calander/internal/tasks/due"
)

// Count is the number of tools Register adds.
const Count = 8

// DefaultZone decides what "today" means when a call names no timezone.
const DefaultZone = "Europe/Amsterdam"

// Instructions is appended to the server instructions when tasks are on.
const Instructions = `
Google Tasks: g_tasks_* tools act on one account's task lists ("list" defaults to
"@default", the account's "My Tasks"). Google stores a due DATE only: any time of day
is ignored. Due accepts YYYY-MM-DD, "today" or "tomorrow"; "today" is computed in
"timezone" (default Europe/Amsterdam). Daily routine: g_tasks_rollover_tasks moves
open overdue tasks (optionally undated ones too) to today and can clear completed
ones; it is idempotent and supports dry_run. Deleting a task and clearing completed
tasks require confirm:true. An account connected before Tasks support returns an
error asking the user to reconnect it on /setup.`

var (
	readOnly    = &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: ptr(true)}
	idempotent  = &mcp.ToolAnnotations{IdempotentHint: true, OpenWorldHint: ptr(true)}
	destructive = &mcp.ToolAnnotations{DestructiveHint: ptr(true), OpenWorldHint: ptr(true)}
)

func ptr[T any](v T) *T { return &v }

type handlers struct {
	accounts account.TasksProvider
	now      func() time.Time
}

// Register adds every Tasks tool to s.
func Register(s *mcp.Server, p account.TasksProvider) { register(s, p, time.Now) }

func register(s *mcp.Server, p account.TasksProvider, now func() time.Time) {
	h := &handlers{accounts: p, now: now}
	h.registerRead(s)
	h.registerWrite(s)
	h.registerRollover(s)
}

func (h *handlers) client(ctx context.Context, arg string) (*tasks.Client, error) {
	return account.OneTasks(ctx, h.accounts, arg)
}

// zone loads the timezone argument, defaulting to DefaultZone.
func zone(name string) (*time.Location, error) {
	if strings.TrimSpace(name) == "" {
		name = DefaultZone
	}
	return timeinput.LoadZone(name, nil)
}

// parseDue reads an optional due argument; "" yields the zero date.
func (h *handlers) parseDue(in string, loc *time.Location) (due.Date, error) {
	if strings.TrimSpace(in) == "" {
		return due.Date{}, nil
	}
	return due.Parse(in, loc, h.now())
}

// jsonResult renders v as indented JSON text content.
func jsonResult(v any) (*mcp.CallToolResult, any, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(data)}}}, nil, nil
}
