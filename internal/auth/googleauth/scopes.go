package googleauth

import (
	"strings"

	gcal "google.golang.org/api/calendar/v3"
	gtasks "google.golang.org/api/tasks/v1"
)

// TasksScope grants read and write access to Google Tasks. Accounts
// connected before Tasks support lack it until they reconnect.
const TasksScope = gtasks.TasksScope

// Scopes requested when connecting an account: full calendar and tasks
// access plus the minimum OpenID scopes needed to learn which account was
// connected.
var Scopes = []string{gcal.CalendarScope, TasksScope, "openid", "email"}

// ParseScopes splits Google's space-separated scope string.
func ParseScopes(s string) []string {
	return strings.Fields(s)
}

// HasScope reports whether want is among granted. The broader full-access
// scope satisfies its ".readonly" variant, never the reverse.
func HasScope(granted []string, want string) bool {
	for _, g := range granted {
		if g == want || g+".readonly" == want {
			return true
		}
	}
	return false
}
