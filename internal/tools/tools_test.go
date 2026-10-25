package tools

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gcal "google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"

	"github.com/tr4m0ryp/oido-google-calander/internal/calendar"
)

// fakeGoogle records requests and serves canned Calendar API responses.
type fakeGoogle struct {
	mu       sync.Mutex
	requests []string
	bodies   map[string]string
}

func (f *fakeGoogle) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	key := r.Method + " " + r.URL.Path
	f.mu.Lock()
	f.requests = append(f.requests, key+"?"+r.URL.RawQuery)
	f.bodies[key] = string(body)
	f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	switch {
	case strings.HasSuffix(r.URL.Path, "/calendars/primary") && r.Method == http.MethodGet:
		_, _ = io.WriteString(w, `{"timeZone":"Europe/Amsterdam"}`)
	case r.URL.Path == "/calendars/primary/events" && r.Method == http.MethodPost:
		_, _ = w.Write(body) // echo the inserted event
	case strings.HasPrefix(r.URL.Path, "/calendars/primary/events/") && r.Method == http.MethodGet:
		_, _ = io.WriteString(w, `{"id":"ev1","summary":"Standup","start":{"dateTime":"2026-03-02T09:00:00+01:00"},`+
			`"end":{"dateTime":"2026-03-02T09:15:00+01:00"},"attendees":[{"email":"boss@example.com","organizer":true},`+
			`{"email":"me@example.com","self":true,"responseStatus":"needsAction"}]}`)
	case strings.HasPrefix(r.URL.Path, "/calendars/primary/events/") && r.Method == http.MethodPatch:
		_, _ = io.WriteString(w, `{"id":"ev1","summary":"Standup","start":{"dateTime":"2026-03-02T09:00:00+01:00"},`+
			`"end":{"dateTime":"2026-03-02T09:15:00+01:00"},"attendees":[{"email":"me@example.com","self":true,"responseStatus":"accepted"}]}`)
	case r.URL.Path == "/freeBusy":
		_, _ = io.WriteString(w, `{"calendars":{"primary":{"busy":[{"start":"2026-03-02T08:00:00Z","end":"2026-03-02T10:00:00Z"}]}}}`)
	case r.Method == http.MethodDelete:
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"error":{"code":404,"message":"unexpected `+key+`"}}`)
	}
}

func (f *fakeGoogle) saw(prefix string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.requests {
		if strings.HasPrefix(r, prefix) {
			return true
		}
	}
	return false
}

type fakeProvider struct{ svc *gcal.Service }

func (p fakeProvider) Emails() []string { return []string{"me@example.com"} }

func (p fakeProvider) Client(context.Context, string) (*calendar.Client, error) {
	return calendar.NewClient(p.svc, "me@example.com", calendar.Options{}), nil
}

func connect(t *testing.T) (*mcp.ClientSession, *fakeGoogle) {
	t.Helper()
	fake := &fakeGoogle{bodies: map[string]string{}}
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	svc, err := gcal.NewService(context.Background(), option.WithEndpoint(srv.URL+"/"),
		option.WithoutAuthentication(), option.WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(fakeProvider{svc: svc}, "test")
	ct, st := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs, fake
}

func call(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any) (map[string]any, bool) {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	text := res.Content[0].(*mcp.TextContent).Text
	if res.IsError {
		t.Logf("%s error: %s", name, text)
	}
	var out map[string]any
	if !res.IsError {
		if err := json.Unmarshal([]byte(text), &out); err != nil {
			t.Fatalf("%s returned non-JSON %q", name, text)
		}
	}
	return out, res.IsError
}

func TestToolList(t *testing.T) {
	cs, _ := connect(t)
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Tools) != Count {
		t.Errorf("registered %d tools, Count says %d", len(res.Tools), Count)
	}
	names := map[string]bool{}
	for _, tool := range res.Tools {
		names[tool.Name] = true
	}
	for _, want := range []string{"list_accounts", "list_calendars", "list_events", "search_events",
		"get_event", "create_event", "update_event", "delete_event", "quick_add",
		"respond_to_event", "freebusy", "find_free_slots"} {
		if !names["g_calendar_"+want] {
			t.Errorf("missing tool g_calendar_%s", want)
		}
	}
}

func TestDeleteNeedsConfirm(t *testing.T) {
	cs, fake := connect(t)
	out, isErr := call(t, cs, "g_calendar_delete_event", map[string]any{"event_id": "ev1"})
	if isErr || out["deleted"] != false {
		t.Fatalf("preview = %v (error %v)", out, isErr)
	}
	if fake.saw("DELETE") {
		t.Fatal("deleted without confirm")
	}
	out, _ = call(t, cs, "g_calendar_delete_event", map[string]any{"event_id": "ev1", "confirm": true})
	if out["deleted"] != true || !fake.saw("DELETE /calendars/primary/events/ev1") {
		t.Fatalf("confirmed delete = %v", out)
	}
}

func TestCreateWithMeetUsesCalendarZone(t *testing.T) {
	cs, fake := connect(t)
	_, isErr := call(t, cs, "g_calendar_create_event", map[string]any{
		"summary": "Review", "start_time": "2026-07-01T10:00", "attendees": []string{"a@example.com"},
		"add_meet": true, "reminders": map[string]any{"overrides": []map[string]any{{"minutes": 10}}},
	})
	if isErr {
		t.Fatal("create failed")
	}
	if !fake.saw("POST /calendars/primary/events?") || !strings.Contains(strings.Join(fake.requests, " "), "conferenceDataVersion=1") {
		t.Errorf("requests = %v", fake.requests)
	}
	body := fake.bodies["POST /calendars/primary/events"]
	for _, want := range []string{`"dateTime":"2026-07-01T10:00:00+02:00"`, `"timeZone":"Europe/Amsterdam"`,
		`"hangoutsMeet"`, `"a@example.com"`, `"useDefault":false`, `"method":"popup"`} {
		if !strings.Contains(body, want) {
			t.Errorf("insert body lacks %s: %s", want, body)
		}
	}
}

func TestRespondPatchesSelf(t *testing.T) {
	cs, fake := connect(t)
	out, isErr := call(t, cs, "g_calendar_respond_to_event", map[string]any{"event_id": "ev1", "response": "accept"})
	if isErr || out["my_response"] != "accepted" {
		t.Fatalf("respond = %v", out)
	}
	if !strings.Contains(fake.bodies["PATCH /calendars/primary/events/ev1"], `"responseStatus":"accepted"`) {
		t.Errorf("patch body = %s", fake.bodies["PATCH /calendars/primary/events/ev1"])
	}
	if _, isErr := call(t, cs, "g_calendar_respond_to_event", map[string]any{"event_id": "ev1", "response": "perhaps"}); !isErr {
		t.Error("invalid response accepted")
	}
}

func TestFindFreeSlots(t *testing.T) {
	cs, _ := connect(t)
	out, isErr := call(t, cs, "g_calendar_find_free_slots", map[string]any{
		"duration_minutes": 60, "time_min": "2026-03-02", "time_max": "2026-03-02", "timezone": "UTC",
	})
	if isErr {
		t.Fatal("find_free_slots failed")
	}
	slots := out["slots"].([]any)
	if len(slots) != 1 || slots[0].(map[string]any)["start"] != "2026-03-02T10:00:00Z" {
		t.Errorf("slots = %v", slots)
	}
}
