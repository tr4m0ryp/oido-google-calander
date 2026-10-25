// Package rollover implements the daily to-do reset: open tasks that are
// overdue (and optionally undated) move to today, and completed tasks are
// optionally cleared. Planning is pure; Run applies a plan through a client.
//
// The operation is idempotent: after a run no open task is due before today
// and, with IncludeUndated, none is undated, so a second run on the same day
// changes nothing.
package rollover

import (
	"context"
	"fmt"

	"github.com/tr4m0ryp/oido-google-calander/internal/tasks"
	"github.com/tr4m0ryp/oido-google-calander/internal/tasks/due"
)

// Reason says why a task moves.
const (
	ReasonOverdue = "overdue"
	ReasonUndated = "undated"
)

// Move is one planned (or applied) due-date change.
type Move struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	From   string `json:"from,omitempty"` // previous due date, empty when undated
	Reason string `json:"reason"`
}

// Plan returns the moves that bring ts up to today, in list order.
// Completed and hidden tasks never move.
func Plan(ts []tasks.Task, today due.Date, includeUndated bool) []Move {
	var out []Move
	for _, t := range ts {
		if !t.Open() || t.Hidden {
			continue
		}
		d := t.DueDate()
		switch {
		case d.IsZero() && includeUndated:
			out = append(out, Move{ID: t.ID, Title: t.Title, Reason: ReasonUndated})
		case !d.IsZero() && d.Before(today):
			out = append(out, Move{ID: t.ID, Title: t.Title, From: d.String(), Reason: ReasonOverdue})
		}
	}
	return out
}

// Options configure one run.
type Options struct {
	List           string
	Today          due.Date
	IncludeUndated bool
	ClearCompleted bool
	DryRun         bool
}

// Result reports what a run did, or would do with DryRun.
type Result struct {
	List             string       `json:"list"`
	Today            string       `json:"today"`
	DryRun           bool         `json:"dry_run"`
	MovedOverdue     int          `json:"moved_overdue"`
	MovedUndated     int          `json:"moved_undated"`
	ClearedCompleted int          `json:"cleared_completed"`
	Moves            []Move       `json:"moves"`
	TodayTasks       []tasks.Task `json:"today_tasks"`
}

// Applier is the slice of the Tasks client a run needs.
type Applier interface {
	Tasks(ctx context.Context, listID string, v tasks.Visibility) ([]tasks.Task, error)
	SetDue(ctx context.Context, listID, taskID string, d due.Date) (tasks.Task, error)
	ClearCompleted(ctx context.Context, listID string) error
}

// Run plans against the live list and applies the plan unless DryRun.
// On a partial failure it returns the counts applied so far with the
// error; re-running is safe and finishes the job.
func Run(ctx context.Context, c Applier, o Options) (Result, error) {
	if o.Today.IsZero() {
		return Result{}, fmt.Errorf("rollover needs today's date")
	}
	res := Result{List: tasks.ListID(o.List), Today: o.Today.String(), DryRun: o.DryRun, Moves: []Move{}}
	current, err := c.Tasks(ctx, o.List, tasks.Visibility{Completed: true})
	if err != nil {
		return res, err
	}
	index := make(map[string]int, len(current))
	for i, t := range current {
		index[t.ID] = i
	}
	for _, m := range Plan(current, o.Today, o.IncludeUndated) {
		moved := current[index[m.ID]].WithDue(o.Today)
		if !o.DryRun {
			if moved, err = c.SetDue(ctx, o.List, m.ID, o.Today); err != nil {
				return res, fmt.Errorf("moved %d tasks, then failed on %q: %w", len(res.Moves), m.Title, err)
			}
		}
		current[index[m.ID]] = moved
		res.Moves = append(res.Moves, m)
		if m.Reason == ReasonOverdue {
			res.MovedOverdue++
		} else {
			res.MovedUndated++
		}
	}
	if o.ClearCompleted {
		res.ClearedCompleted = tasks.CountVisibleCompleted(current)
		if res.ClearedCompleted > 0 && !o.DryRun {
			if err := c.ClearCompleted(ctx, o.List); err != nil {
				res.ClearedCompleted = 0
				return res, err
			}
		}
	}
	res.TodayTasks = tasks.Filter{Status: tasks.StatusOpen, DueOn: o.Today}.Apply(current)
	return res, nil
}
