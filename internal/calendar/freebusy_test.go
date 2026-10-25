package calendar

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	gcal "google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"

	"github.com/tr4m0ryp/oido-google-calander/internal/calendar/availability"
)

func fakeService(t *testing.T, body string) *gcal.Service {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	svc, err := gcal.NewService(context.Background(), option.WithEndpoint(srv.URL+"/"),
		option.WithoutAuthentication(), option.WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

func TestFreeBusyKeys(t *testing.T) {
	window := availability.Interval{Start: time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC),
		End: time.Date(2026, 3, 3, 0, 0, 0, 0, time.UTC)}
	busy := `{"busy":[{"start":"2026-03-02T09:00:00Z","end":"2026-03-02T10:00:00Z"}]}`
	tests := []struct {
		name    string
		body    string
		ids     []string
		wantErr bool
	}{
		{"keyed as requested", `{"calendars":{"primary":` + busy + `}}`, nil, false},
		{"primary answered under email", `{"calendars":{"Me@Example.com":` + busy + `}}`, nil, false},
		{"single unknown key", `{"calendars":{"other-id":` + busy + `}}`, []string{"team"}, false},
		{"missing among several", `{"calendars":{"a":` + busy + `,"b":` + busy + `}}`, []string{"c", "a"}, true},
		{"google error", `{"calendars":{"primary":{"errors":[{"reason":"notFound"}]}}}`, nil, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := NewClient(fakeService(t, tc.body), "me@example.com", Options{})
			got, err := c.FreeBusy(context.Background(), tc.ids, window)
			if err != nil {
				t.Fatal(err)
			}
			first := got[0]
			if tc.wantErr != (first.Error != "") {
				t.Fatalf("entry = %+v, wantErr %v", first, tc.wantErr)
			}
			if !tc.wantErr && len(first.Busy) != 1 {
				t.Errorf("busy = %v", first.Busy)
			}
		})
	}
}
