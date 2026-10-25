package tasks

import (
	"context"
	"errors"
	"fmt"
	"strings"

	gtasks "google.golang.org/api/tasks/v1"

	"github.com/tr4m0ryp/oido-google-calander/internal/tasks/due"
)

// NewTask describes a task to create.
type NewTask struct {
	List   string
	Title  string
	Notes  string
	Due    due.Date // zero for no due date
	Parent string   // parent task id, making this a subtask
}

// Create inserts a task at the top of its list (or of its parent's subtasks).
func (c *Client) Create(ctx context.Context, n NewTask) (Task, error) {
	if strings.TrimSpace(n.Title) == "" {
		return Task{}, errors.New("title is required")
	}
	t := &gtasks.Task{Title: n.Title, Notes: n.Notes, Status: apiOpen}
	if !n.Due.IsZero() {
		t.Due = n.Due.API()
	}
	call := c.svc.Tasks.Insert(ListID(n.List), t).Context(ctx)
	if p := strings.TrimSpace(n.Parent); p != "" {
		call = call.Parent(p)
	}
	created, err := call.Do()
	if err != nil {
		return Task{}, fmt.Errorf("create task: %w", Explain(err))
	}
	return fromAPI(created), nil
}

// Patch changes selected fields of a task; nil fields stay as they are.
type Patch struct {
	List   string
	ID     string
	Title  *string
	Notes  *string   // "" clears the notes
	Due    *due.Date // zero date clears the due date
	Status string    // StatusOpen, StatusCompleted or "" to leave it
}

// Update applies p and returns the task as Google stored it.
func (c *Client) Update(ctx context.Context, p Patch) (Task, error) {
	if strings.TrimSpace(p.ID) == "" {
		return Task{}, errors.New("task_id is required")
	}
	t := &gtasks.Task{}
	if p.Title != nil {
		if strings.TrimSpace(*p.Title) == "" {
			return Task{}, errors.New("title cannot be empty")
		}
		t.Title = *p.Title
	}
	if p.Notes != nil {
		t.Notes = *p.Notes
		t.ForceSendFields = append(t.ForceSendFields, "Notes")
	}
	if p.Due != nil {
		if p.Due.IsZero() {
			t.NullFields = append(t.NullFields, "Due")
		} else {
			t.Due = p.Due.API()
		}
	}
	switch p.Status {
	case "":
	case StatusCompleted:
		t.Status = apiCompleted
	case StatusOpen:
		// Reopening must also drop the completion time.
		t.Status = apiOpen
		t.NullFields = append(t.NullFields, "Completed")
	default:
		return Task{}, fmt.Errorf("status must be %q or %q", StatusOpen, StatusCompleted)
	}
	updated, err := c.svc.Tasks.Patch(ListID(p.List), p.ID, t).Context(ctx).Do()
	if err != nil {
		return Task{}, fmt.Errorf("update task: %w", Explain(err))
	}
	return fromAPI(updated), nil
}

// Complete marks a task completed.
func (c *Client) Complete(ctx context.Context, listID, taskID string) (Task, error) {
	return c.Update(ctx, Patch{List: listID, ID: taskID, Status: StatusCompleted})
}

// SetDue moves a task to date d.
func (c *Client) SetDue(ctx context.Context, listID, taskID string, d due.Date) (Task, error) {
	return c.Update(ctx, Patch{List: listID, ID: taskID, Due: &d})
}

// Delete removes a task (and, on Google's side, its subtasks).
func (c *Client) Delete(ctx context.Context, listID, taskID string) error {
	if err := c.svc.Tasks.Delete(ListID(listID), taskID).Context(ctx).Do(); err != nil {
		return fmt.Errorf("delete task: %w", Explain(err))
	}
	return nil
}

// ClearCompleted hides every completed task of a list (Google's tasks.clear).
// Hidden tasks are not deleted and stay listable with include_hidden.
func (c *Client) ClearCompleted(ctx context.Context, listID string) error {
	if err := c.svc.Tasks.Clear(ListID(listID)).Context(ctx).Do(); err != nil {
		return fmt.Errorf("clear completed tasks: %w", Explain(err))
	}
	return nil
}

// CountVisibleCompleted counts completed tasks that a clear would hide.
func CountVisibleCompleted(ts []Task) int {
	n := 0
	for _, t := range ts {
		if t.Status == StatusCompleted && !t.Hidden {
			n++
		}
	}
	return n
}
