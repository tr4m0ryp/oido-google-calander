package availability

import (
	"testing"
	"time"
)

var base = time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC) // a Monday

func at(h, m int) time.Time {
	return base.Add(time.Duration(h)*time.Hour + time.Duration(m)*time.Minute)
}

func iv(h1, m1, h2, m2 int) Interval { return Interval{Start: at(h1, m1), End: at(h2, m2)} }

func TestMerge(t *testing.T) {
	tests := []struct {
		name string
		in   []Interval
		want []Interval
	}{
		{"empty", nil, nil},
		{"disjoint unsorted", []Interval{iv(11, 0, 12, 0), iv(9, 0, 10, 0)},
			[]Interval{iv(9, 0, 10, 0), iv(11, 0, 12, 0)}},
		{"overlap", []Interval{iv(9, 0, 10, 30), iv(10, 0, 11, 0)}, []Interval{iv(9, 0, 11, 0)}},
		{"touching", []Interval{iv(9, 0, 10, 0), iv(10, 0, 11, 0)}, []Interval{iv(9, 0, 11, 0)}},
		{"contained", []Interval{iv(9, 0, 12, 0), iv(10, 0, 11, 0)}, []Interval{iv(9, 0, 12, 0)}},
		{"drops empty", []Interval{iv(9, 0, 9, 0), iv(10, 0, 9, 0)}, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Merge(tc.in)
			if len(got) != len(tc.want) {
				t.Fatalf("Merge = %v, want %v", got, tc.want)
			}
			for i := range got {
				if !got[i].Start.Equal(tc.want[i].Start) || !got[i].End.Equal(tc.want[i].End) {
					t.Errorf("Merge[%d] = %v, want %v", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestSubtract(t *testing.T) {
	free := Subtract(iv(9, 0, 17, 0), Merge([]Interval{iv(8, 0, 9, 30), iv(12, 0, 13, 0), iv(16, 30, 18, 0)}))
	want := []Interval{iv(9, 30, 12, 0), iv(13, 0, 16, 30)}
	if len(free) != len(want) {
		t.Fatalf("Subtract = %v, want %v", free, want)
	}
	for i := range want {
		if !free[i].Start.Equal(want[i].Start) || !free[i].End.Equal(want[i].End) {
			t.Errorf("free[%d] = %v, want %v", i, free[i], want[i])
		}
	}
}

func TestFindSlots(t *testing.T) {
	workweek := []time.Weekday{time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday}
	tests := []struct {
		name  string
		busy  []Interval
		q     SlotQuery
		wantN int
		first Interval
	}{
		{
			name: "gap between meetings, aligned to 15 min",
			busy: []Interval{iv(9, 0, 10, 10), iv(11, 30, 17, 0)},
			q: SlotQuery{Window: iv(0, 0, 24, 0), Duration: time.Hour, WorkStart: 9 * time.Hour,
				WorkEnd: 17 * time.Hour, Granularity: 15 * time.Minute},
			wantN: 1, first: iv(10, 15, 11, 30),
		},
		{
			name: "too-short gap skipped",
			busy: []Interval{iv(9, 0, 10, 0), iv(10, 30, 17, 0)},
			q: SlotQuery{Window: iv(0, 0, 24, 0), Duration: time.Hour, WorkStart: 9 * time.Hour,
				WorkEnd: 17 * time.Hour},
			wantN: 0,
		},
		{
			name: "buffer shrinks gap",
			busy: []Interval{iv(9, 0, 10, 0), iv(11, 0, 17, 0)},
			q: SlotQuery{Window: iv(0, 0, 24, 0), Duration: 45 * time.Minute, WorkStart: 9 * time.Hour,
				WorkEnd: 17 * time.Hour, Buffer: 10 * time.Minute},
			wantN: 0,
		},
		{
			name: "weekend skipped, max slots honoured",
			busy: nil,
			q: SlotQuery{Window: Interval{Start: base.AddDate(0, 0, -2), End: base.AddDate(0, 0, 3)},
				Duration: time.Hour, WorkStart: 9 * time.Hour, WorkEnd: 17 * time.Hour,
				WorkDays: workweek, MaxSlots: 2},
			wantN: 2, first: iv(9, 0, 17, 0),
		},
		{
			name: "window starts mid-day",
			busy: nil,
			q: SlotQuery{Window: iv(13, 7, 24, 0), Duration: 30 * time.Minute, WorkStart: 9 * time.Hour,
				WorkEnd: 17 * time.Hour, Granularity: 30 * time.Minute},
			wantN: 1, first: iv(13, 30, 17, 0),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := FindSlots(tc.busy, tc.q)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != tc.wantN {
				t.Fatalf("got %d slots %v, want %d", len(got), got, tc.wantN)
			}
			if tc.wantN > 0 && (!got[0].Start.Equal(tc.first.Start) || !got[0].End.Equal(tc.first.End)) {
				t.Errorf("first slot = %v, want %v", got[0], tc.first)
			}
		})
	}
}

func TestFindSlotsWorkingHoursAcrossDST(t *testing.T) {
	ams, err := time.LoadLocation("Europe/Amsterdam")
	if err != nil {
		t.Fatal(err)
	}
	// Clocks move forward on Sunday 2026-03-29; Monday 09:00 local is 07:00 UTC.
	q := SlotQuery{
		Window:   Interval{Start: time.Date(2026, 3, 27, 0, 0, 0, 0, ams), End: time.Date(2026, 3, 31, 0, 0, 0, 0, ams)},
		Duration: time.Hour, Location: ams, WorkStart: 9 * time.Hour, WorkEnd: 17 * time.Hour,
		WorkDays: []time.Weekday{time.Friday, time.Monday},
	}
	got, err := FindSlots(nil, q)
	if err != nil || len(got) != 2 {
		t.Fatalf("got %v, %v", got, err)
	}
	if h := got[0].Start.UTC().Hour(); h != 8 {
		t.Errorf("Friday 09:00 CET = %d UTC, want 8", h)
	}
	if h := got[1].Start.UTC().Hour(); h != 7 {
		t.Errorf("Monday 09:00 CEST = %d UTC, want 7", h)
	}
}

func TestFindSlotsValidation(t *testing.T) {
	bad := []SlotQuery{
		{Window: iv(9, 0, 17, 0)},
		{Window: iv(17, 0, 9, 0), Duration: time.Hour},
		{Window: iv(0, 0, 24, 0), Duration: time.Hour, WorkStart: 17 * time.Hour, WorkEnd: 9 * time.Hour},
	}
	for i, q := range bad {
		if _, err := FindSlots(nil, q); err == nil {
			t.Errorf("case %d: want validation error", i)
		}
	}
}

func TestParseClockAndWeekdays(t *testing.T) {
	if d, err := ParseClock("09:30"); err != nil || d != 9*time.Hour+30*time.Minute {
		t.Errorf("ParseClock(09:30) = %v, %v", d, err)
	}
	for _, s := range []string{"25:00", "9", "24:30", "ab:cd"} {
		if _, err := ParseClock(s); err == nil {
			t.Errorf("ParseClock(%q) want error", s)
		}
	}
	days, err := ParseWeekdays([]string{"Mon", "friday", "SAT"})
	if err != nil || len(days) != 3 || days[0] != time.Monday || days[2] != time.Saturday {
		t.Errorf("ParseWeekdays = %v, %v", days, err)
	}
	if _, err := ParseWeekdays([]string{"mo"}); err == nil {
		t.Error("two-letter weekday should be rejected")
	}
}
