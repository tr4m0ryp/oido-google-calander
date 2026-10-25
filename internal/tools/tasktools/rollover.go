package tasktools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tr4m0ryp/oido-google-calander/internal/tasks/due"
	"github.com/tr4m0ryp/oido-google-calander/internal/tasks/rollover"
)

type rolloverArgs struct {
	Account        string `json:"account,omitempty" jsonschema:"Account email"`
	List           string `json:"list,omitempty" jsonschema:"Task list id (default @default)"`
	TimeZone       string `json:"timezone,omitempty" jsonschema:"IANA zone that decides what today is (default Europe/Amsterdam)"`
	IncludeUndated bool   `json:"include_undated,omitempty" jsonschema:"Also move open tasks without a due date to today"`
	ClearCompleted bool   `json:"clear_completed,omitempty" jsonschema:"Afterwards hide completed tasks (Google's clear)"`
	DryRun         bool   `json:"dry_run,omitempty" jsonschema:"Report what would change without changing anything"`
}

func (h *handlers) registerRollover(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "g_tasks_rollover_tasks",
		Description: "Daily reset of a task list: every open task due before today gets due=today " +
			"(optionally undated ones too), then completed tasks are optionally cleared. Returns counts, " +
			"the moves, and the resulting list of open tasks due today. Idempotent; use dry_run to preview.",
		Annotations: &mcp.ToolAnnotations{IdempotentHint: true, DestructiveHint: ptr(false), OpenWorldHint: ptr(true)},
	}, h.rollover)
}

func (h *handlers) rollover(ctx context.Context, _ *mcp.CallToolRequest, a rolloverArgs) (
	*mcp.CallToolResult, any, error) {
	loc, err := zone(a.TimeZone)
	if err != nil {
		return nil, nil, err
	}
	c, err := h.client(ctx, a.Account)
	if err != nil {
		return nil, nil, err
	}
	res, err := rollover.Run(ctx, c, rollover.Options{
		List: a.List, Today: due.Today(h.now(), loc), IncludeUndated: a.IncludeUndated,
		ClearCompleted: a.ClearCompleted, DryRun: a.DryRun,
	})
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{"account": c.Account(), "timezone": loc.String(), "rollover": res})
}
