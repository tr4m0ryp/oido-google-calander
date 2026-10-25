package due

import (
	"testing"
	"time"
)

func amsterdam(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Amsterdam")
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

func utc(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestTodayAmsterdam(t *testing.T) {
	ams := amsterdam(t)
	// EU clocks go back on 2026-10-25 at 01:00 UTC and forward on 2026-03-29 at 01:00 UTC.
	tests := []struct {
		name string
		now  string
		want string
	}{
		{"summer evening is still the UTC date", "2026-07-01T20:00:00Z", "2026-07-01"},
		{"after local midnight in CEST, UTC still on the previous day", "2026-10-24T22:30:00Z", "2026-10-25"},
		{"one second before local midnight in CEST", "2026-10-24T21:59:59Z", "2026-10-24"},
		{"inside the repeated 02:00-03:00 hour, first pass", "2026-10-25T00:30:00Z", "2026-10-25"},
		{"inside the repeated 02:00-03:00 hour, second pass", "2026-10-25T01:30:00Z", "2026-10-25"},
		{"last second of the 25-hour day", "2026-10-25T22:59:59Z", "2026-10-25"},
		{"local midnight after the switch is 23:00 UTC", "2026-10-25T23:00:00Z", "2026-10-26"},
		{"local midnight before spring forward is 23:00 UTC", "2026-03-28T23:00:00Z", "2026-03-29"},
		{"right after spring forward", "2026-03-29T01:00:00Z", "2026-03-29"},
		{"last second of the 23-hour day", "2026-03-29T21:59:59Z", "2026-03-29"},
		{"local midnight after spring forward is 22:00 UTC", "2026-03-29T22:00:00Z", "2026-03-30"},
		{"new year in Amsterdam before UTC", "2026-12-31T23:30:00Z", "2027-01-01"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Today(utc(tc.now), ams).String(); got != tc.want {
				t.Errorf("Today(%s) = %s, want %s", tc.now, got, tc.want)
			}
		})
	}
}

func TestParse(t *testing.T) {
	ams := amsterdam(t)
	now := utc("2026-10-24T22:30:00Z") // 00:30 on the 25th in Amsterdam
	tests := []struct {
		in, want string
		loc      *time.Location
		wantErr  bool
	}{
		{in: "today", want: "2026-10-25", loc: ams},
		{in: " Tomorrow ", want: "2026-10-26", loc: ams},
		{in: "yesterday", want: "2026-10-24", loc: ams},
		{in: "today", want: "2026-10-24", loc: time.UTC},
		{in: "2026-11-02", want: "2026-11-02", loc: ams},
		{in: "2026-10-25T23:30:00Z", want: "2026-10-26", loc: ams},
		{in: "2026-10-25T23:30:00+01:00", want: "2026-10-25", loc: ams},
		{in: "2026-10-25T23:30:00Z", want: "2026-10-25", loc: time.UTC},
		{in: "2026-10-25T09:00", want: "2026-10-25", loc: ams},
		{in: "2026-02-30", loc: ams, wantErr: true},
		{in: "next week", loc: ams, wantErr: true},
		{in: "", loc: ams, wantErr: true},
	}
	for _, tc := range tests {
		got, err := Parse(tc.in, tc.loc, now)
		if (err != nil) != tc.wantErr {
			t.Errorf("Parse(%q) error = %v, wantErr %v", tc.in, err, tc.wantErr)
			continue
		}
		if got.String() != tc.want {
			t.Errorf("Parse(%q) in %s = %s, want %s", tc.in, tc.loc, got, tc.want)
		}
	}
}

func TestAPIRoundTrip(t *testing.T) {
	tests := []struct{ api, want string }{
		{"2026-10-25T00:00:00.000Z", "2026-10-25"},
		{"2026-03-29T00:00:00Z", "2026-03-29"},
		{"", ""},
	}
	for _, tc := range tests {
		d, err := FromAPI(tc.api)
		if err != nil || d.String() != tc.want {
			t.Errorf("FromAPI(%q) = %s, %v; want %s", tc.api, d, err, tc.want)
		}
		if tc.want != "" && d.API() != tc.want+"T00:00:00.000Z" {
			t.Errorf("API() = %s", d.API())
		}
	}
	if _, err := FromAPI("tomorrow"); err == nil {
		t.Error("FromAPI accepted garbage")
	}
}

func TestBeforeAndAddDays(t *testing.T) {
	a, b := Date{2026, time.October, 31}, Date{2026, time.November, 1}
	if !a.Before(b) || b.Before(a) || a.Before(a) {
		t.Error("Before ordering wrong")
	}
	if a.AddDays(1) != b || b.AddDays(-1) != a {
		t.Error("AddDays across a month boundary wrong")
	}
	if !(Date{}).IsZero() || a.IsZero() {
		t.Error("IsZero wrong")
	}
}
