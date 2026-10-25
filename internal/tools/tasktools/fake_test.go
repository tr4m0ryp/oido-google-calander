package tasktools

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"

	gtasks "google.golang.org/api/tasks/v1"
)

// fakeTasksAPI is a small stateful stand-in for the Google Tasks API with
// one list, "@default".
type fakeTasksAPI struct {
	mu     sync.Mutex
	items  []*gtasks.Task
	nextID int
	writes []string
}

func (f *fakeTasksAPI) seed(id, title, status, due string) {
	f.items = append(f.items, &gtasks.Task{Id: id, Title: title, Status: status, Due: due})
}

func (f *fakeTasksAPI) find(id string) *gtasks.Task {
	for _, t := range f.items {
		if t.Id == id {
			return t
		}
	}
	return nil
}

func (f *fakeTasksAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	path := strings.TrimPrefix(r.URL.Path, "/tasks/v1/")
	if r.Method != http.MethodGet {
		f.writes = append(f.writes, r.Method+" "+path+"?"+r.URL.RawQuery)
	}
	parts := strings.Split(path, "/")
	switch {
	case path == "users/@me/lists":
		write(w, map[string]any{"items": []map[string]string{{"id": "@default", "title": "My Tasks"}}})
	case len(parts) == 3 && parts[2] == "clear":
		for _, t := range f.items {
			if t.Status == "completed" {
				t.Hidden = true
			}
		}
		w.WriteHeader(http.StatusNoContent)
	case len(parts) == 3 && r.Method == http.MethodGet:
		f.list(w, r)
	case len(parts) == 3 && r.Method == http.MethodPost:
		var t gtasks.Task
		_ = json.NewDecoder(r.Body).Decode(&t)
		f.nextID++
		t.Id, t.Parent = "new"+string(rune('0'+f.nextID)), r.URL.Query().Get("parent")
		f.items = append([]*gtasks.Task{&t}, f.items...)
		write(w, t)
	case len(parts) == 4:
		t := f.find(parts[3])
		if t == nil {
			w.WriteHeader(http.StatusNotFound)
			write(w, map[string]any{"error": map[string]any{"code": 404, "message": "Task not found"}})
			return
		}
		f.item(w, r, t)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (f *fakeTasksAPI) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var out []*gtasks.Task
	for _, t := range f.items {
		if (t.Status == "completed" && q.Get("showCompleted") != "true") || (t.Hidden && q.Get("showHidden") != "true") {
			continue
		}
		out = append(out, t)
	}
	write(w, map[string]any{"items": out})
}

func (f *fakeTasksAPI) item(w http.ResponseWriter, r *http.Request, t *gtasks.Task) {
	switch r.Method {
	case http.MethodGet:
		write(w, t)
	case http.MethodDelete:
		for i, it := range f.items {
			if it == t {
				f.items = append(f.items[:i], f.items[i+1:]...)
				break
			}
		}
		w.WriteHeader(http.StatusNoContent)
	case http.MethodPatch:
		var patch map[string]json.RawMessage
		_ = json.NewDecoder(r.Body).Decode(&patch)
		str := func(k string) string { var s string; _ = json.Unmarshal(patch[k], &s); return s }
		for k := range patch {
			switch k {
			case "title":
				t.Title = str(k)
			case "notes":
				t.Notes = str(k)
			case "due":
				t.Due = str(k) // null decodes to ""
			case "status":
				t.Status = str(k)
				if t.Status == "completed" {
					done := "2026-10-25T08:00:00.000Z"
					t.Completed = &done
				}
			case "completed":
				t.Completed = nil
			}
		}
		write(w, t)
	}
}

func write(w http.ResponseWriter, v any) { _ = json.NewEncoder(w).Encode(v) }
