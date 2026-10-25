package rollover

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/tr4m0ryp/oido-google-calander/internal/tasks"
	"github.com/tr4m0ryp/oido-google-calander/internal/tasks/due"
)

func d(s string) due.Date {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return due.Of(t)
}

func task(id, status, dueOn string) tasks.Task {
	t := tasks.Task{ID: id, Title: "task " + id, Status: status}
	if dueOn != "" {
		t = t.WithDue(d(dueOn))
	}
	return t
}

func TestPlan(t *testing.T) {
	today := d("2026-10-25")
	hidden := task("h", tasks.StatusOpen, "2026-10-01")
	hidden.Hidden = true
	tests := []struct {
		name     string
		in       []tasks.Task
		undated  bool
		wantIDs  []string
		wantFrom []string
	}{
		{"nothing to do", []tasks.Task{task("a", "open", "2026-10-25"), task("b", "open", "2026-10-30")}, false, nil, nil},
		{"overdue moves", []tasks.Task{task("a", "open", "2026-10-24"), task("b", "open", "2026-10-25")}, false,
			[]string{"a"}, []string{"2026-10-24"}},
		{"long overdue across months", []tasks.Task{task("a", "open", "2026-09-30")}, false,
			[]string{"a"}, []string{"2026-09-30"}},
		{"completed overdue stays", []tasks.Task{task("a", "completed", "2026-10-20")}, false, nil, nil},
		{"hidden stays", []tasks.Task{hidden}, true, nil, nil},
		{"undated ignored by default", []tasks.Task{task("a", "open", "")}, false, nil, nil},
		{"undated moves when asked", []tasks.Task{task("a", "open", ""), task("b", "completed", "")}, true,
			[]string{"a"}, []string{""}},
		{"list order kept", []tasks.Task{task("c", "open", "2026-10-01"), task("a", "open", ""),
			task("b", "open", "2026-10-24")}, true, []string{"c", "a", "b"}, []string{"2026-10-01", "", "2026-10-24"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var ids, from []string
			for _, m := range Plan(tc.in, today, tc.undated) {
				ids, from = append(ids, m.ID), append(from, m.From)
			}
			if !reflect.DeepEqual(ids, tc.wantIDs) || !reflect.DeepEqual(from, tc.wantFrom) {
				t.Errorf("Plan = %v from %v, want %v from %v", ids, from, tc.wantIDs, tc.wantFrom)
			}
		})
	}
}

// fakeList is an in-memory task list implementing Applier.
type fakeList struct {
	items   []tasks.Task
	writes  int
	cleared int
}

func (f *fakeList) Tasks(_ context.Context, _ string, v tasks.Visibility) ([]tasks.Task, error) {
	var out []tasks.Task
	for _, t := range f.items {
		if (t.Status == tasks.StatusCompleted && !v.Completed) || (t.Hidden && !v.Hidden) {
			continue
		}
		out = append(out, t)
	}
	return out, nil
}

func (f *fakeList) SetDue(_ context.Context, _, id string, dd due.Date) (tasks.Task, error) {
	f.writes++
	for i := range f.items {
		if f.items[i].ID == id {
			f.items[i] = f.items[i].WithDue(dd)
			return f.items[i], nil
		}
	}
	return tasks.Task{}, context.Canceled
}

func (f *fakeList) ClearCompleted(context.Context, string) error {
	f.cleared++
	for i := range f.items {
		if f.items[i].Status == tasks.StatusCompleted {
			f.items[i].Hidden = true
		}
	}
	return nil
}

func newFake() *fakeList {
	return &fakeList{items: []tasks.Task{
		task("overdue", "open", "2026-10-24"), task("today", "open", "2026-10-25"),
		task("future", "open", "2026-11-01"), task("undated", "open", ""),
		task("done", "completed", "2026-10-24"),
	}}
}

// The run happens at 00:30 local on the day the clocks go back; in UTC it
// is still the 24th, so a UTC "today" would wrongly leave "overdue" alone.
func amsterdamToday(t *testing.T) due.Date {
	loc, err := time.LoadLocation("Europe/Amsterdam")
	if err != nil {
		t.Fatal(err)
	}
	return due.Today(time.Date(2026, 10, 24, 22, 30, 0, 0, time.UTC), loc)
}

func TestRunIsIdempotentAcrossDST(t *testing.T) {
	f := newFake()
	opts := Options{Today: amsterdamToday(t), IncludeUndated: true, ClearCompleted: true}
	res, err := Run(context.Background(), f, opts)
	if err != nil {
		t.Fatal(err)
	}
	if res.Today != "2026-10-25" || res.MovedOverdue != 1 || res.MovedUndated != 1 || res.ClearedCompleted != 1 {
		t.Fatalf("first run = %+v", res)
	}
	if got := ids(res.TodayTasks); !reflect.DeepEqual(got, []string{"overdue", "today", "undated"}) {
		t.Errorf("today list = %v", got)
	}
	writes := f.writes
	again, err := Run(context.Background(), f, opts)
	if err != nil {
		t.Fatal(err)
	}
	if again.MovedOverdue+again.MovedUndated+again.ClearedCompleted != 0 || f.writes != writes || f.cleared != 1 {
		t.Errorf("second run changed something: %+v (writes %d->%d, clears %d)", again, writes, f.writes, f.cleared)
	}
	if !reflect.DeepEqual(ids(again.TodayTasks), ids(res.TodayTasks)) {
		t.Errorf("today list drifted: %v vs %v", ids(again.TodayTasks), ids(res.TodayTasks))
	}
}

func TestDryRunWritesNothing(t *testing.T) {
	f := newFake()
	res, err := Run(context.Background(), f, Options{Today: d("2026-10-25"), ClearCompleted: true, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if f.writes != 0 || f.cleared != 0 {
		t.Fatalf("dry run wrote: %d writes, %d clears", f.writes, f.cleared)
	}
	if !res.DryRun || res.MovedOverdue != 1 || res.MovedUndated != 0 || res.ClearedCompleted != 1 {
		t.Errorf("dry run = %+v", res)
	}
	if got := ids(res.TodayTasks); !reflect.DeepEqual(got, []string{"overdue", "today"}) {
		t.Errorf("projected today list = %v", got)
	}
	if f.items[0].Due != "2026-10-24" {
		t.Error("dry run mutated the source list")
	}
}

func TestRunNeedsToday(t *testing.T) {
	if _, err := Run(context.Background(), newFake(), Options{}); err == nil {
		t.Error("zero today accepted")
	}
}

func ids(ts []tasks.Task) []string {
	out := []string{}
	for _, t := range ts {
		out = append(out, t.ID)
	}
	return out
}
