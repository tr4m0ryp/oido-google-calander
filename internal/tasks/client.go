// Package tasks wraps the Google Tasks API for one account: task lists,
// tasks, and compact JSON views of both. Due values are plain dates (see
// package due); Google ignores any time of day.
package tasks

import (
	"context"
	"fmt"
	"strings"

	gtasks "google.golang.org/api/tasks/v1"
)

// DefaultList addresses the account's default task list ("My Tasks").
const DefaultList = "@default"

// pageSize is the largest page the Tasks API serves.
const pageSize = 100

// Client performs Tasks operations for one Google account.
type Client struct {
	svc     *gtasks.Service
	account string
}

// NewClient wraps an authenticated Tasks service.
func NewClient(svc *gtasks.Service, account string) *Client {
	return &Client{svc: svc, account: account}
}

// Account is the email this client acts as.
func (c *Client) Account() string { return c.account }

// ListID resolves an empty list argument to the default list.
func ListID(id string) string {
	if strings.TrimSpace(id) == "" {
		return DefaultList
	}
	return strings.TrimSpace(id)
}

// TaskLists returns every task list on the account.
func (c *Client) TaskLists(ctx context.Context) ([]TaskList, error) {
	var out []TaskList
	err := c.svc.Tasklists.List().MaxResults(pageSize).Pages(ctx, func(page *gtasks.TaskLists) error {
		for _, l := range page.Items {
			out = append(out, TaskList{ID: l.Id, Title: l.Title, Updated: l.Updated})
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list task lists: %w", Explain(err))
	}
	return out, nil
}

// Visibility selects which finished tasks a listing includes. Open tasks
// are always returned.
type Visibility struct {
	// Completed includes completed tasks that have not been cleared.
	Completed bool
	// Hidden also includes completed tasks hidden by "clear completed".
	Hidden bool
}

// Tasks returns the tasks of one list in Google's display order.
func (c *Client) Tasks(ctx context.Context, listID string, v Visibility) ([]Task, error) {
	var out []Task
	call := c.svc.Tasks.List(ListID(listID)).MaxResults(pageSize).
		ShowCompleted(v.Completed || v.Hidden).ShowHidden(v.Hidden).ShowDeleted(false)
	err := call.Pages(ctx, func(page *gtasks.Tasks) error {
		for _, t := range page.Items {
			out = append(out, fromAPI(t))
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", Explain(err))
	}
	return out, nil
}

// Get fetches one task.
func (c *Client) Get(ctx context.Context, listID, taskID string) (Task, error) {
	t, err := c.svc.Tasks.Get(ListID(listID), taskID).Context(ctx).Do()
	if err != nil {
		return Task{}, fmt.Errorf("get task: %w", Explain(err))
	}
	return fromAPI(t), nil
}
