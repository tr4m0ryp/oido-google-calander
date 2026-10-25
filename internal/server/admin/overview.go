package admin

import (
	"context"
	"errors"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/tr4m0ryp/oido-google-calander/internal/calendar"
	"github.com/tr4m0ryp/oido-google-calander/internal/server/admin/views"
)

const (
	chipLimit   = 6
	agendaLimit = 4
)

var hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// overview snapshots every account in parallel: its calendars and next few
// events, as a sanity check that each connection really works.
func (a *Admin) overview(ctx context.Context, now time.Time) []views.Account {
	accounts := a.deps.Store.List()
	out := make([]views.Account, len(accounts))
	var wg sync.WaitGroup
	for i, acct := range accounts {
		out[i] = views.Account{
			Email:     acct.Email,
			Initial:   strings.ToUpper(acct.Email[:1]),
			AddedISO:  acct.AddedAt.Format(time.RFC3339),
			AddedText: acct.AddedAt.Format("Jan 2, 2006"),
		}
		wg.Add(1)
		go func(v *views.Account) {
			defer wg.Done()
			a.snapshot(ctx, v, now)
		}(&out[i])
	}
	wg.Wait()
	return out
}

func (a *Admin) snapshot(ctx context.Context, v *views.Account, now time.Time) {
	// A failed scope lookup leaves the Tasks state unknown; the calendar
	// calls below surface the underlying problem.
	if ok, err := a.deps.Accounts.HasTasks(ctx, v.Email); err == nil {
		v.TasksEnabled, v.TasksNeedsReconnect = ok, !ok
	}
	client, err := a.deps.Accounts.Client(ctx, v.Email)
	if err != nil {
		v.Error = friendly(err)
		return
	}
	cals, err := client.ListCalendars(ctx)
	if err != nil {
		v.Error = friendly(err)
		return
	}
	loc := now.Location()
	for _, c := range cals {
		if c.Primary && c.TimeZone != "" {
			v.TimeZone = c.TimeZone
			if l, err := time.LoadLocation(c.TimeZone); err == nil {
				loc = l
			}
		}
	}
	v.Calendars, v.MoreCount = chips(cals, v.Email)
	v.CalendarCount = len(cals)
	events, err := client.ListEvents(ctx, calendar.ListQuery{MaxResults: agendaLimit})
	if err != nil {
		v.Error = friendly(err)
		return
	}
	v.Agenda = agenda(events, now.In(loc))
}

// chips orders primary first, then owned, then the rest by name.
func chips(cals []calendar.Calendar, email string) ([]views.Chip, int) {
	rank := func(c calendar.Calendar) int {
		switch {
		case c.Primary:
			return 0
		case c.AccessRole == "owner":
			return 1
		}
		return 2
	}
	sort.SliceStable(cals, func(i, j int) bool {
		if ri, rj := rank(cals[i]), rank(cals[j]); ri != rj {
			return ri < rj
		}
		return strings.ToLower(cals[i].Summary) < strings.ToLower(cals[j].Summary)
	})
	var out []views.Chip
	for i, c := range cals {
		if i == chipLimit {
			return out, len(cals) - chipLimit
		}
		chip := views.Chip{Name: c.Summary, Tag: roleTag(c.AccessRole)}
		if c.Primary {
			chip.Tag = "Primary"
			// The primary calendar is named after the account, already shown above.
			if strings.EqualFold(c.Summary, email) {
				chip.Name, chip.Tag = "Primary calendar", ""
			}
		}
		if hexColor.MatchString(c.Color) {
			chip.Color = c.Color
		}
		out = append(out, chip)
	}
	return out, 0
}

func roleTag(role string) string {
	switch role {
	case "reader", "freeBusyReader":
		return "Read only"
	case "writer":
		return "Shared"
	}
	return ""
}

func agenda(events []calendar.Event, now time.Time) []views.AgendaItem {
	today := now.Format("2006-01-02")
	tomorrow := now.AddDate(0, 0, 1).Format("2006-01-02")
	var out []views.AgendaItem
	for _, e := range events {
		item := views.AgendaItem{Title: e.Summary, Time: "All day"}
		if item.Title == "" {
			item.Title = "(no title)"
		}
		var day time.Time
		if e.AllDay {
			day, _ = time.ParseInLocation("2006-01-02", e.Start, now.Location())
		} else if t, err := time.Parse(time.RFC3339, e.Start); err == nil {
			day = t.In(now.Location())
			item.Time = day.Format("15:04")
		}
		switch key := day.Format("2006-01-02"); key {
		case today:
			item.Day, item.IsToday = "Today", true
		case tomorrow:
			item.Day = "Tomorrow"
		default:
			item.Day = day.Format("Mon 2 Jan")
		}
		out = append(out, item)
	}
	return out
}

// friendly turns an API failure into one short line for the dashboard.
func friendly(err error) string {
	switch {
	case errors.Is(err, calendar.ErrNeedsReauth):
		return "Google no longer accepts this account's sign-in. Reconnect it to restore access."
	case errors.Is(err, context.DeadlineExceeded):
		return "Google Calendar did not answer in time. Reload to try again."
	}
	msg := err.Error()
	if strings.Contains(msg, "not enabled") || strings.Contains(msg, "has not been used") {
		return "The Google Calendar API is not enabled for this OAuth project."
	}
	return "Could not reach Google Calendar for this account."
}
