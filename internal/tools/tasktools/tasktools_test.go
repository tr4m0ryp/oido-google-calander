package tasktools

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"google.golang.org/api/option"
	gtasks "google.golang.org/api/tasks/v1"

	"github.com/tr4m0ryp/oido-google-calander/internal/calendar"
	"github.com/tr4m0ryp/oido-google-calander/internal/tasks"
)

// fakeProvider serves one account; legacy simulates a grant without Tasks.
type fakeProvider struct {
	svc    *gtasks.Service
	legacy bool
}

func (p fakeProvider) Emails() []string { return []string{"me@example.com"} }

func (p fakeProvider) Client(context.Context, string) (*calendar.Client, error) {
	return nil, errors.New("calendar not used here")
}

func (p fakeProvider) Tasks(_ context.Context, email string) (*tasks.Client, error) {
	if p.legacy {
		return nil, tasks.MissingScope(email, "https://cal.example.test/setup")
	}
	return tasks.NewClient(p.svc, email), nil
}

// 00:30 in Amsterdam on 2026-10-25 (clocks go back that night) is still
// the 24th in UTC.
var testNow = time.Date(2026, 10, 24, 22, 30, 0, 0, time.UTC)

func connect(t *testing.T, legacy bool) (*mcp.ClientSession, *fakeTasksAPI) {
	t.Helper()
	api := &fakeTasksAPI{}
	api.seed("overdue", "Reply to Sam", "needsAction", "2026-10-24T00:00:00.000Z")
	api.seed("today", "Gym", "needsAction", "2026-10-25T00:00:00.000Z")
	api.seed("undated", "Read paper", "needsAction", "")
	api.seed("done", "Pay rent", "completed", "2026-10-23T00:00:00.000Z")
	srv := httptest.NewServer(api)
	t.Cleanup(srv.Close)
	svc, err := gtasks.NewService(context.Background(), option.WithEndpoint(srv.URL+"/"),
		option.WithoutAuthentication(), option.WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatal(err)
	}
	s := mcp.NewServer(&mcp.Implementation{Name: "test"}, nil)
	register(s, fakeProvider{svc: svc, legacy: legacy}, func() time.Time { return testNow })
	ct, st := mcp.NewInMemoryTransports()
	if _, err := s.Connect(context.Background(), st, nil); err != nil {
		t.Fatal(err)
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "c"}, nil).Connect(context.Background(), ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs, api
}

func call(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any) (map[string]any, string) {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	text := res.Content[0].(*mcp.TextContent).Text
	if res.IsError {
		return nil, text
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("%s returned non-JSON %q", name, text)
	}
	return out, ""
}

func TestToolsRegistered(t *testing.T) {
	cs, _ := connect(t, false)
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Tools) != Count {
		t.Errorf("registered %d tools, Count says %d", len(res.Tools), Count)
	}
}

func TestRolloverToolIsIdempotent(t *testing.T) {
	cs, api := connect(t, false)
	args := map[string]any{"include_undated": true, "clear_completed": true, "dry_run": true}
	out, errText := call(t, cs, "g_tasks_rollover_tasks", args)
	if errText != "" || len(api.writes) != 0 {
		t.Fatalf("dry run: %s, writes %v", errText, api.writes)
	}
	r := out["rollover"].(map[string]any)
	if r["today"] != "2026-10-25" || r["moved_overdue"] != 1.0 || r["moved_undated"] != 1.0 || r["cleared_completed"] != 1.0 {
		t.Fatalf("dry run result = %v", r)
	}
	delete(args, "dry_run")
	if _, errText = call(t, cs, "g_tasks_rollover_tasks", args); errText != "" {
		t.Fatal(errText)
	}
	if api.find("overdue").Due != "2026-10-25T00:00:00.000Z" || api.find("undated").Due == "" || !api.find("done").Hidden {
		t.Fatalf("state after rollover: %+v %+v %+v", api.find("overdue"), api.find("undated"), api.find("done"))
	}
	writes := len(api.writes)
	out, _ = call(t, cs, "g_tasks_rollover_tasks", args)
	r = out["rollover"].(map[string]any)
	if len(api.writes) != writes || r["moved_overdue"] != 0.0 || r["moved_undated"] != 0.0 || len(r["today_tasks"].([]any)) != 3 {
		t.Errorf("second run not a no-op: %v, writes %v", r, api.writes[writes:])
	}
}

func TestCreateAndUpdateDates(t *testing.T) {
	cs, api := connect(t, false)
	out, errText := call(t, cs, "g_tasks_create_task", map[string]any{
		"title": "Answer invoice mail", "due": "tomorrow", "parent": "today", "notes": "from mail"})
	if errText != "" {
		t.Fatal(errText)
	}
	created := out["created"].(map[string]any)
	if created["due"] != "2026-10-26" || created["parent"] != "today" {
		t.Errorf("created = %v", created)
	}
	id := created["id"].(string)
	if _, errText = call(t, cs, "g_tasks_update_task", map[string]any{"task_id": id, "due": "", "notes": ""}); errText != "" {
		t.Fatal(errText)
	}
	if got := api.find(id); got.Due != "" || got.Notes != "" {
		t.Errorf("due/notes not cleared: %+v", got)
	}
	out, _ = call(t, cs, "g_tasks_complete_task", map[string]any{"task_id": id})
	if out["completed"].(map[string]any)["status"] != "completed" {
		t.Errorf("complete = %v", out)
	}
	out, _ = call(t, cs, "g_tasks_list_tasks", map[string]any{"due_on": "today"})
	if out["count"] != 1.0 {
		t.Errorf("due_on today = %v", out)
	}
}

func TestDestructiveToolsNeedConfirm(t *testing.T) {
	cs, api := connect(t, false)
	out, _ := call(t, cs, "g_tasks_delete_task", map[string]any{"task_id": "today"})
	if out["deleted"] != false || api.find("today") == nil {
		t.Fatalf("delete without confirm = %v", out)
	}
	out, _ = call(t, cs, "g_tasks_clear_completed", map[string]any{})
	if out["cleared"] != false || out["completed_tasks"] != 1.0 || api.find("done").Hidden {
		t.Fatalf("clear without confirm = %v", out)
	}
	call(t, cs, "g_tasks_delete_task", map[string]any{"task_id": "today", "confirm": true})
	call(t, cs, "g_tasks_clear_completed", map[string]any{"confirm": true})
	if api.find("today") != nil || !api.find("done").Hidden {
		t.Error("confirmed delete/clear did not apply")
	}
}

func TestMissingScopeTellsUserToReconnect(t *testing.T) {
	cs, _ := connect(t, true)
	for _, name := range []string{"g_tasks_list_tasklists", "g_tasks_rollover_tasks"} {
		_, errText := call(t, cs, name, map[string]any{})
		if !strings.Contains(errText, "Reconnect to enable Tasks") || !strings.Contains(errText, "/setup") {
			t.Errorf("%s error = %q", name, errText)
		}
	}
}

func TestBadInputRejected(t *testing.T) {
	cs, _ := connect(t, false)
	for name, args := range map[string]map[string]any{
		"g_tasks_create_task":    {"title": "x", "due": "someday"},
		"g_tasks_list_tasks":     {"status": "pending"},
		"g_tasks_rollover_tasks": {"timezone": "Mars/Olympus"},
		"g_tasks_update_task":    {"task_id": "today", "status": "all"},
	} {
		if _, errText := call(t, cs, name, args); errText == "" {
			t.Errorf("%s accepted %v", name, args)
		}
	}
}
