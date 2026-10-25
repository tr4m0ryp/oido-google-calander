package calendar

import (
	"context"
	"fmt"
	"strings"
	"time"

	gcal "google.golang.org/api/calendar/v3"

	"github.com/tr4m0ryp/oido-google-calander/internal/calendar/timeinput"
)

// PrimaryID addresses an account's main calendar.
const PrimaryID = "primary"

// Client performs calendar operations for one Google account.
type Client struct {
	svc         *gcal.Service
	account     string
	defaultCal  string
	defaultZone *time.Location
}

// Options tune a Client; the zero value is valid.
type Options struct {
	// DefaultCalendar replaces "primary" when a call names no calendar
	// (service accounts usually act on a shared calendar, not their own).
	DefaultCalendar string
	// DefaultZone is the last-resort zone for naive times when neither the
	// caller nor the calendar names one.
	DefaultZone *time.Location
}

// NewClient wraps an authenticated service.
func NewClient(svc *gcal.Service, account string, opts Options) *Client {
	c := &Client{svc: svc, account: account, defaultCal: opts.DefaultCalendar, defaultZone: opts.DefaultZone}
	if c.defaultCal == "" {
		c.defaultCal = PrimaryID
	}
	if c.defaultZone == nil {
		c.defaultZone = time.UTC
	}
	return c
}

// Account is the email (or identifier) this client acts as.
func (c *Client) Account() string { return c.account }

func (c *Client) calendarID(id string) string {
	if strings.TrimSpace(id) == "" {
		return c.defaultCal
	}
	return id
}

// Zone resolves the zone used to read naive times: the explicit name if
// given, else the calendar's own zone, else the server default.
func (c *Client) Zone(ctx context.Context, calID, name string) (*time.Location, error) {
	if strings.TrimSpace(name) != "" {
		return timeinput.LoadZone(name, nil)
	}
	cal, err := c.svc.Calendars.Get(c.calendarID(calID)).Context(ctx).Fields("timeZone").Do()
	if err != nil || cal.TimeZone == "" {
		return c.defaultZone, nil
	}
	if loc, err := time.LoadLocation(cal.TimeZone); err == nil {
		return loc, nil
	}
	return c.defaultZone, nil
}

// zoneFor resolves a zone only when some input actually needs one, saving a
// Calendars.Get round trip when every time carries its own offset.
func (c *Client) zoneFor(ctx context.Context, calID, name string, inputs ...string) (*time.Location, error) {
	needed := strings.TrimSpace(name) != ""
	for _, in := range inputs {
		if strings.TrimSpace(in) != "" && !timeinput.HasOffset(in) {
			needed = true
		}
	}
	if !needed {
		return c.defaultZone, nil
	}
	return c.Zone(ctx, calID, name)
}

// ListCalendars returns every calendar on the account's calendar list.
func (c *Client) ListCalendars(ctx context.Context) ([]Calendar, error) {
	var out []Calendar
	call := c.svc.CalendarList.List().MaxResults(250)
	err := call.Pages(ctx, func(page *gcal.CalendarList) error {
		for _, item := range page.Items {
			out = append(out, calendarFromAPI(item))
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list calendars: %w", Explain(err))
	}
	return out, nil
}
