package tasktools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tr4m0ryp/oido-google-calander/internal/tasks"
)

type listTaskListsArgs struct {
	Account string `json:"account,omitempty" jsonschema:"Account email (default: the only connected account)"`
}

type listTasksArgs struct {
	Account          string `json:"account,omitempty" jsonschema:"Account email (default: the only connected account)"`
	List             string `json:"list,omitempty" jsonschema:"Task list id (default: @default, the account's My Tasks)"`
	Status           string `json:"status,omitempty" jsonschema:"open (default), completed or all"`
	DueBefore        string `json:"due_before,omitempty" jsonschema:"Only tasks due strictly before this date (YYYY-MM-DD, today, tomorrow)"`
	DueAfter         string `json:"due_after,omitempty" jsonschema:"Only tasks due strictly after this date"`
	DueOn            string `json:"due_on,omitempty" jsonschema:"Only tasks due on this date, e.g. today"`
	IncludeCompleted bool   `json:"include_completed,omitempty" jsonschema:"Shorthand for status all"`
	IncludeHidden    bool   `json:"include_hidden,omitempty" jsonschema:"Also return completed tasks hidden by clear_completed"`
	TimeZone         string `json:"timezone,omitempty" jsonschema:"IANA zone for today/tomorrow (default Europe/Amsterdam)"`
}

func (h *handlers) registerRead(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "g_tasks_list_tasklists",
		Description: "List an account's Google Tasks lists (id and title). Use the id as list elsewhere.",
		Annotations: readOnly,
	}, h.listTaskLists)
	mcp.AddTool(s, &mcp.Tool{
		Name: "g_tasks_list_tasks",
		Description: "List tasks of one task list in Google's order, filtered by status and due date. " +
			"Due values are dates; a task without a due date never matches a due filter.",
		Annotations: readOnly,
	}, h.listTasks)
}

func (h *handlers) listTaskLists(ctx context.Context, _ *mcp.CallToolRequest, a listTaskListsArgs) (
	*mcp.CallToolResult, any, error) {
	c, err := h.client(ctx, a.Account)
	if err != nil {
		return nil, nil, err
	}
	lists, err := c.TaskLists(ctx)
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{"account": c.Account(), "tasklists": lists})
}

func (h *handlers) listTasks(ctx context.Context, _ *mcp.CallToolRequest, a listTasksArgs) (
	*mcp.CallToolResult, any, error) {
	f, err := h.filter(a)
	if err != nil {
		return nil, nil, err
	}
	c, err := h.client(ctx, a.Account)
	if err != nil {
		return nil, nil, err
	}
	all, err := c.Tasks(ctx, a.List, f.Visibility(a.IncludeHidden))
	if err != nil {
		return nil, nil, err
	}
	matched := f.Apply(all)
	return jsonResult(map[string]any{"account": c.Account(), "list": tasks.ListID(a.List),
		"count": len(matched), "tasks": matched})
}

func (h *handlers) filter(a listTasksArgs) (tasks.Filter, error) {
	status, err := tasks.NormaliseStatus(a.Status)
	if err != nil {
		return tasks.Filter{}, err
	}
	if a.IncludeCompleted && a.Status == "" {
		status = tasks.StatusAll
	}
	loc, err := zone(a.TimeZone)
	if err != nil {
		return tasks.Filter{}, err
	}
	f := tasks.Filter{Status: status}
	if f.DueBefore, err = h.parseDue(a.DueBefore, loc); err != nil {
		return tasks.Filter{}, err
	}
	if f.DueAfter, err = h.parseDue(a.DueAfter, loc); err != nil {
		return tasks.Filter{}, err
	}
	if f.DueOn, err = h.parseDue(a.DueOn, loc); err != nil {
		return tasks.Filter{}, err
	}
	return f, nil
}
