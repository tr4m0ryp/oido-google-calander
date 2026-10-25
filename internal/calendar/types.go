// Package calendar wraps the Google Calendar v3 API for one account and
// returns compact, JSON-friendly views tuned for an assistant agent.
package calendar

// Calendar is one entry of an account's calendar list.
type Calendar struct {
	ID          string `json:"id"`
	Summary     string `json:"summary"`
	Description string `json:"description,omitempty"`
	TimeZone    string `json:"time_zone,omitempty"`
	AccessRole  string `json:"access_role"`
	Primary     bool   `json:"primary,omitempty"`
	Selected    bool   `json:"selected,omitempty"`
	Color       string `json:"color,omitempty"`
}

// Attendee is a guest on an event.
type Attendee struct {
	Email     string `json:"email"`
	Name      string `json:"name,omitempty"`
	Response  string `json:"response,omitempty"`
	Optional  bool   `json:"optional,omitempty"`
	Organizer bool   `json:"organizer,omitempty"`
	Self      bool   `json:"self,omitempty"`
}

// Reminder is one reminder override ("popup" or "email", minutes before start).
type Reminder struct {
	Method  string `json:"method,omitempty" jsonschema:"popup (default) or email"`
	Minutes int64  `json:"minutes" jsonschema:"Minutes before the start"`
}

// Reminders mirrors the event's reminder settings.
type Reminders struct {
	UseDefault bool       `json:"use_default"`
	Overrides  []Reminder `json:"overrides,omitempty"`
}

// Event is an event as returned to tools. Start and End are RFC3339 for
// timed events and YYYY-MM-DD (end exclusive) for all-day events.
type Event struct {
	ID               string     `json:"id"`
	CalendarID       string     `json:"calendar_id,omitempty"`
	Summary          string     `json:"summary"`
	Start            string     `json:"start"`
	End              string     `json:"end"`
	AllDay           bool       `json:"all_day,omitempty"`
	TimeZone         string     `json:"time_zone,omitempty"`
	Status           string     `json:"status,omitempty"`
	Location         string     `json:"location,omitempty"`
	Description      string     `json:"description,omitempty"`
	MeetLink         string     `json:"meet_link,omitempty"`
	HTMLLink         string     `json:"html_link,omitempty"`
	Organizer        string     `json:"organizer,omitempty"`
	MyResponse       string     `json:"my_response,omitempty"`
	Attendees        []Attendee `json:"attendees,omitempty"`
	Recurrence       []string   `json:"recurrence,omitempty"`
	RecurringEventID string     `json:"recurring_event_id,omitempty"`
	Reminders        *Reminders `json:"reminders,omitempty"`
	Transparency     string     `json:"transparency,omitempty"`
	Visibility       string     `json:"visibility,omitempty"`
}
