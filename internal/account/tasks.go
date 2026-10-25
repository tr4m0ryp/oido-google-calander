package account

import (
	"context"
	"fmt"

	"google.golang.org/api/option"
	gtasks "google.golang.org/api/tasks/v1"

	"github.com/tr4m0ryp/oido-google-calander/internal/tasks"
)

// TasksProvider is implemented by providers that can reach Google Tasks.
// The service-account provider does not: Tasks belong to a user, and a
// service account only has its own empty lists.
type TasksProvider interface {
	Provider
	// Tasks returns a ready Tasks client, or an error wrapping
	// tasks.ErrScopeMissing when the account must reconnect first.
	Tasks(ctx context.Context, email string) (*tasks.Client, error)
}

// OneTasks resolves a single-account argument to its Tasks client.
func OneTasks(ctx context.Context, p TasksProvider, arg string) (*tasks.Client, error) {
	emails, err := Resolve(p, arg, false)
	if err != nil {
		return nil, err
	}
	return p.Tasks(ctx, emails[0])
}

// Tasks returns a Tasks client for email after checking the grant.
func (o *OAuth) Tasks(ctx context.Context, email string) (*tasks.Client, error) {
	ok, err := o.HasTasks(ctx, email)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, tasks.MissingScope(email, o.SetupURL)
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	c, err := o.entry(email)
	if err != nil {
		return nil, err
	}
	if c.tasks == nil {
		svc, err := gtasks.NewService(context.Background(), option.WithTokenSource(c.ts))
		if err != nil {
			return nil, fmt.Errorf("tasks service for %s: %w", email, err)
		}
		c.tasks = svc
	}
	return tasks.NewClient(c.tasks, email), nil
}
