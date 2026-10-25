package timeinput

import (
	"testing"
	"time"
)

func mustZone(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

func TestParse(t *testing.T) {
	ams := mustZone(t, "Europe/Amsterdam")
	tests := []struct {
		name string
		in   string
		loc  *time.Location
		want string
	}{
		{"rfc3339 keeps offset", "2026-05-15T09:00:00-04:00", ams, "2026-05-15T13:00:00Z"},
		{"zulu", "2026-05-15T09:00:00Z", ams, "2026-05-15T09:00:00Z"},
		{"naive in summer time", "2026-05-15T09:00", ams, "2026-05-15T07:00:00Z"},
		{"naive in winter time", "2026-01-15 09:00", ams, "2026-01-15T08:00:00Z"},
		{"naive with seconds", "2026-01-15T09:00:30", ams, "2026-01-15T08:00:30Z"},
		{"bare date is local midnight", "2026-05-15", ams, "2026-05-14T22:00:00Z"},
		{"nil zone means UTC", "2026-05-15T09:00", nil, "2026-05-15T09:00:00Z"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Parse(tc.in, tc.loc)
			if err != nil {
				t.Fatalf("Parse(%q): %v", tc.in, err)
			}
			if s := got.UTC().Format(time.RFC3339); s != tc.want {
				t.Errorf("Parse(%q) = %s, want %s", tc.in, s, tc.want)
			}
		})
	}
}

func TestParseRejects(t *testing.T) {
	for _, in := range []string{"", "tomorrow", "15/05/2026", "2026-13-01"} {
		if _, err := Parse(in, time.UTC); err == nil {
			t.Errorf("Parse(%q) succeeded, want error", in)
		}
	}
}

func TestParseEndBareDateIsInclusive(t *testing.T) {
	got, err := ParseEnd("2026-05-15", time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if want := time.Date(2026, 5, 16, 0, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Errorf("ParseEnd = %v, want %v", got, want)
	}
}

func TestEventSpan(t *testing.T) {
	ny := mustZone(t, "America/New_York")
	tests := []struct {
		name               string
		start, end         string
		wantStart, wantEnd string
		wantErr            bool
	}{
		{"timed naive", "2026-03-10T09:00", "2026-03-10T10:30",
			"2026-03-10T09:00:00-04:00", "2026-03-10T10:30:00-04:00", false},
		{"timed default duration", "2026-01-10T09:00", "",
			"2026-01-10T09:00:00-05:00", "2026-01-10T10:00:00-05:00", false},
		{"rfc3339 re-expressed in zone", "2026-01-10T14:00:00Z", "2026-01-10T15:00:00Z",
			"2026-01-10T09:00:00-05:00", "2026-01-10T10:00:00-05:00", false},
		{"all-day default one day", "2026-05-15", "", "2026-05-15", "2026-05-16", false},
		{"all-day multi", "2026-05-15", "2026-05-18", "2026-05-15", "2026-05-18", false},
		{"end before start", "2026-01-10T09:00", "2026-01-10T08:00", "", "", true},
		{"all-day timed end", "2026-05-15", "2026-05-15T10:00", "", "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s, e, err := EventSpan(tc.start, tc.end, ny, time.Hour)
			if tc.wantErr {
				if err == nil {
					t.Fatal("want error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			gotS, gotE := s.DateTime+s.Date, e.DateTime+e.Date
			if gotS != tc.wantStart || gotE != tc.wantEnd {
				t.Errorf("span = %s..%s, want %s..%s", gotS, gotE, tc.wantStart, tc.wantEnd)
			}
			if s.DateTime != "" && s.TimeZone != "America/New_York" {
				t.Errorf("timed event zone = %q", s.TimeZone)
			}
		})
	}
}

func TestLoadZone(t *testing.T) {
	if _, err := LoadZone("Mars/Olympus", nil); err == nil {
		t.Error("want error for unknown zone")
	}
	loc, err := LoadZone("", time.UTC)
	if err != nil || loc != time.UTC {
		t.Errorf("empty name should return default, got %v %v", loc, err)
	}
}
