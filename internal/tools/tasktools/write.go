package tasktools

import (
	"context"
	"errors"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tr4m0ryp/oido-google-calander/internal/tasks"
	"github.com/tr4m0ryp/oido-google-calander/internal/tasks/due"
)

type createTaskArgs struct {
	Title    string `json:"title" jsonschema:"Task title"`
	Notes    string `json:"notes,omitempty" jsonschema:"Details, e.g. the mail it came from"`
	Due      string `json:"due,omitempty" jsonschema:"Due date: YYYY-MM-DD, today or tomorrow (time of day is ignored by Google)"`
	Parent   string `json:"parent,omitempty" jsonschema:"Parent task id to create a subtask"`
	List     string `json:"list,omitempty" jsonschema:"Task list id (default @default)"`
	Account  string `json:"account,omitempty" jsonschema:"Account email"`
	TimeZone string `json:"timezone,omitempty" jsonschema:"IANA zone for today/tomorrow (default Europe/Amsterdam)"`
}

type updateTaskArgs struct {
	TaskID   string  `json:"task_id"`
	Title    *string `json:"title,omitempty"`
	Notes    *string `json:"notes,omitempty" jsonschema:"New notes; empty string clears them"`
	Due      *string `json:"due,omitempty" jsonschema:"New due date; empty string removes it"`
	Status   string  `json:"status,omitempty" jsonschema:"open or completed"`
	List     string  `json:"list,omitempty"`
	Account  string  `json:"account,omitempty"`
	TimeZone string  `json:"timezone,omitempty"`
}

type taskRefArgs struct {
	TaskID  string `json:"task_id"`
	List    string `json:"list,omitempty" jsonschema:"Task list id (default @default)"`
	Account string `json:"account,omitempty"`
}

type deleteTaskArgs struct {
	TaskID  string `json:"task_id"`
	Confirm bool   `json:"confirm,omitempty" jsonschema:"Must be true to delete; without it the task is only previewed"`
	List    string `json:"list,omitempty"`
	Account string `json:"account,omitempty"`
}

type clearArgs struct {
	Confirm bool   `json:"confirm,omitempty" jsonschema:"Must be true to clear; without it only the count is returned"`
	List    string `json:"list,omitempty" jsonschema:"Task list id (default @default)"`
	Account string `json:"account,omitempty"`
}

func (h *handlers) registerWrite(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "g_tasks_create_task",
		Description: "Create a task, optionally with notes, a due date and a parent task (subtask).",
		Annotations: &mcp.ToolAnnotations{OpenWorldHint: ptr(true)},
	}, h.createTask)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "g_tasks_update_task",
		Description: "Change a task's title, notes, due date or status; omitted fields stay as they are.",
		Annotations: idempotent,
	}, h.updateTask)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "g_tasks_complete_task",
		Description: "Mark a task completed.",
		Annotations: idempotent,
	}, h.completeTask)
	mcp.AddTool(s, &mcp.Tool{
		Name: "g_tasks_delete_task",
		Description: "Delete a task and its subtasks. Without confirm:true it only previews the task; " +
			"confirm with the user before passing confirm:true.",
		Annotations: destructive,
	}, h.deleteTask)
	mcp.AddTool(s, &mcp.Tool{
		Name: "g_tasks_clear_completed",
		Description: "Hide every completed task of a list (Google's clear). Without confirm:true it only " +
			"reports how many would be cleared.",
		Annotations: destructive,
	}, h.clearCompleted)
}

func (h *handlers) createTask(ctx context.Context, _ *mcp.CallToolRequest, a createTaskArgs) (
	*mcp.CallToolResult, any, error) {
	loc, err := zone(a.TimeZone)
	if err != nil {
		return nil, nil, err
	}
	d, err := h.parseDue(a.Due, loc)
	if err != nil {
		return nil, nil, err
	}
	c, err := h.client(ctx, a.Account)
	if err != nil {
		return nil, nil, err
	}
	t, err := c.Create(ctx, tasks.NewTask{List: a.List, Title: a.Title, Notes: a.Notes, Due: d, Parent: a.Parent})
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{"account": c.Account(), "list": tasks.ListID(a.List), "created": t})
}

func (h *handlers) updateTask(ctx context.Context, _ *mcp.CallToolRequest, a updateTaskArgs) (
	*mcp.CallToolResult, any, error) {
	p := tasks.Patch{List: a.List, ID: a.TaskID, Title: a.Title, Notes: a.Notes}
	if a.Status != "" {
		status, err := tasks.NormaliseStatus(a.Status)
		if err != nil || status == tasks.StatusAll {
			return nil, nil, errors.New("status must be open or completed")
		}
		p.Status = status
	}
	if a.Due != nil {
		loc, err := zone(a.TimeZone)
		if err != nil {
			return nil, nil, err
		}
		var d due.Date
		if d, err = h.parseDue(*a.Due, loc); err != nil {
			return nil, nil, err
		}
		p.Due = &d
	}
	c, err := h.client(ctx, a.Account)
	if err != nil {
		return nil, nil, err
	}
	t, err := c.Update(ctx, p)
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{"account": c.Account(), "updated": t})
}

func (h *handlers) completeTask(ctx context.Context, _ *mcp.CallToolRequest, a taskRefArgs) (
	*mcp.CallToolResult, any, error) {
	if a.TaskID == "" {
		return nil, nil, errors.New("task_id is required")
	}
	c, err := h.client(ctx, a.Account)
	if err != nil {
		return nil, nil, err
	}
	t, err := c.Complete(ctx, a.List, a.TaskID)
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{"account": c.Account(), "completed": t})
}

func (h *handlers) deleteTask(ctx context.Context, _ *mcp.CallToolRequest, a deleteTaskArgs) (
	*mcp.CallToolResult, any, error) {
	if a.TaskID == "" {
		return nil, nil, errors.New("task_id is required")
	}
	c, err := h.client(ctx, a.Account)
	if err != nil {
		return nil, nil, err
	}
	t, err := c.Get(ctx, a.List, a.TaskID)
	if err != nil {
		return nil, nil, err
	}
	if !a.Confirm {
		return jsonResult(map[string]any{"deleted": false, "account": c.Account(), "task": t,
			"note": "Not deleted. Confirm with the user, then call again with confirm:true."})
	}
	if err := c.Delete(ctx, a.List, a.TaskID); err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{"deleted": true, "account": c.Account(), "task": t})
}

func (h *handlers) clearCompleted(ctx context.Context, _ *mcp.CallToolRequest, a clearArgs) (
	*mcp.CallToolResult, any, error) {
	c, err := h.client(ctx, a.Account)
	if err != nil {
		return nil, nil, err
	}
	all, err := c.Tasks(ctx, a.List, tasks.Visibility{Completed: true})
	if err != nil {
		return nil, nil, err
	}
	n := tasks.CountVisibleCompleted(all)
	out := map[string]any{"account": c.Account(), "list": tasks.ListID(a.List), "completed_tasks": n, "cleared": false}
	if !a.Confirm {
		out["note"] = "Nothing cleared. Confirm with the user, then call again with confirm:true."
		return jsonResult(out)
	}
	if n > 0 {
		if err := c.ClearCompleted(ctx, a.List); err != nil {
			return nil, nil, err
		}
	}
	out["cleared"] = true
	return jsonResult(out)
}
