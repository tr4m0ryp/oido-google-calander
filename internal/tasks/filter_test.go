package tasks

import (
	"testing"
	"time"

	"github.com/tr4m0ryp/oido-google-calander/internal/tasks/due"
)

func date(y int, m time.Month, d int) due.Date { return due.Date{Year: y, Month: m, Day: d} }

func TestFilter(t *testing.T) {
	open := Task{ID: "o", Status: StatusOpen}.WithDue(date(2026, 10, 25))
	done := Task{ID: "c", Status: StatusCompleted}.WithDue(date(2026, 10, 20))
	undated := Task{ID: "u", Status: StatusOpen}
	tests := []struct {
		name string
		f    Filter
		want string
	}{
		{"open only", Filter{Status: StatusOpen}, "ou"},
		{"completed only", Filter{Status: StatusCompleted}, "c"},
		{"all", Filter{Status: StatusAll}, "ocu"},
		{"before is exclusive", Filter{Status: StatusAll, DueBefore: date(2026, 10, 25)}, "c"},
		{"after is exclusive", Filter{Status: StatusAll, DueAfter: date(2026, 10, 20)}, "o"},
		{"on", Filter{Status: StatusOpen, DueOn: date(2026, 10, 25)}, "o"},
		{"window", Filter{Status: StatusAll, DueAfter: date(2026, 10, 19), DueBefore: date(2026, 10, 26)}, "oc"},
	}
	for _, tc := range tests {
		got := ""
		for _, task := range tc.f.Apply([]Task{open, done, undated}) {
			got += task.ID
		}
		if got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestNormaliseStatus(t *testing.T) {
	for in, want := range map[string]string{"": StatusOpen, "needsAction": StatusOpen, "Completed": StatusCompleted, "all": StatusAll} {
		if got, err := NormaliseStatus(in); err != nil || got != want {
			t.Errorf("NormaliseStatus(%q) = %q, %v", in, got, err)
		}
	}
	if _, err := NormaliseStatus("pending"); err == nil {
		t.Error("unknown status accepted")
	}
}
