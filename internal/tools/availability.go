package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tr4m0ryp/oido-google-calander/internal/account"
	"github.com/tr4m0ryp/oido-google-calander/internal/calendar"
	"github.com/tr4m0ryp/oido-google-calander/internal/calendar/availability"
	"github.com/tr4m0ryp/oido-google-calander/internal/calendar/timeinput"
)

const (
	defaultSearchSpan   = 7 * 24 * time.Hour
	maxSearchSpan       = 62 * 24 * time.Hour
	defaultSlotStep     = 30
	defaultMaxSlots     = 10
	defaultWorkingStart = "09:00"
	defaultWorkingEnd   = "17:00"
)

var defaultWorkDays = []string{"mon", "tue", "wed", "thu", "fri"}

type freeBusyArgs struct {
	Account     string   `json:"account,omitempty" jsonschema:"Account email, or \"all\" (default: all)"`
	CalendarIDs []string `json:"calendar_ids,omitempty" jsonschema:"Calendars per account to check (default: primary)"`
	TimeMin     string   `json:"time_min,omitempty" jsonschema:"Window start (default: now)"`
	TimeMax     string   `json:"time_max,omitempty" jsonschema:"Window end (default: 7 days after start)"`
	TimeZone    string   `json:"timezone,omitempty" jsonschema:"IANA zone for naive inputs and output"`
}

type freeSlotsArgs struct {
	DurationMinutes int      `json:"duration_minutes" jsonschema:"Length of the meeting to fit"`
	Account         string   `json:"account,omitempty" jsonschema:"Whose calendars must be free: an email or \"all\" (default: all)"`
	CalendarIDs     []string `json:"calendar_ids,omitempty" jsonschema:"Calendars per account that block time (default: primary)"`
	TimeMin         string   `json:"time_min,omitempty" jsonschema:"Search window start (default: now)"`
	TimeMax         string   `json:"time_max,omitempty" jsonschema:"Search window end (default: 7 days after start, max 62)"`
	TimeZone        string   `json:"timezone,omitempty" jsonschema:"IANA zone for working hours and output"`
	WorkingStart    string   `json:"working_hours_start,omitempty" jsonschema:"HH:MM local (default 09:00; 00:00 with end 24:00 for any time)"`
	WorkingEnd      string   `json:"working_hours_end,omitempty" jsonschema:"HH:MM local (default 17:00)"`
	WorkingDays     []string `json:"working_days,omitempty" jsonschema:"Weekdays to consider (default mon..fri)"`
	GranularityMin  int      `json:"granularity_minutes,omitempty" jsonschema:"Align slot starts to this grid (default 30)"`
	BufferMinutes   int      `json:"buffer_minutes,omitempty" jsonschema:"Free gap to keep around existing events"`
	MaxSlots        int      `json:"max_slots,omitempty" jsonschema:"Maximum ranges to return (default 10)"`
}

func (h *handlers) registerAvailability(s *mcp.Server) {
	h.registerRespond(s)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "g_calendar_freebusy",
		Description: "Busy blocks per account in a window, plus the merged busy time across them.",
		Annotations: readOnly,
	}, h.freeBusy)
	mcp.AddTool(s, &mcp.Tool{
		Name: "g_calendar_find_free_slots",
		Description: "Find free time ranges of at least duration_minutes that are open on every selected " +
			"account, within working hours and days, in the given timezone.",
		Annotations: readOnly,
	}, h.findFreeSlots)
}

// busyQuery is the resolved window plus the zone outputs are expressed in.
type busyQuery struct {
	window availability.Interval
	loc    *time.Location
}

// resolveWindow parses the window, reading naive times in the explicit zone
// or else the first account's calendar zone.
func (h *handlers) resolveWindow(ctx context.Context, acct, zone, min, max string) (busyQuery, error) {
	emails, err := account.Resolve(h.accounts, defaultAll(acct), true)
	if err != nil {
		return busyQuery{}, err
	}
	first, err := h.accounts.Client(ctx, emails[0])
	if err != nil {
		return busyQuery{}, err
	}
	loc, err := first.Zone(ctx, "", zone)
	if err != nil {
		return busyQuery{}, err
	}
	start := time.Now().In(loc)
	if min != "" {
		if start, err = timeinput.Parse(min, loc); err != nil {
			return busyQuery{}, err
		}
	}
	end := start.Add(defaultSearchSpan)
	if max != "" {
		if end, err = timeinput.ParseEnd(max, loc); err != nil {
			return busyQuery{}, err
		}
	}
	if !end.After(start) {
		return busyQuery{}, fmt.Errorf("time_max must be after time_min")
	}
	if end.Sub(start) > maxSearchSpan {
		return busyQuery{}, fmt.Errorf("window too long: at most %d days", int(maxSearchSpan.Hours()/24))
	}
	return busyQuery{window: availability.Interval{Start: start, End: end}, loc: loc}, nil
}

func defaultAll(acct string) string {
	if acct == "" {
		return account.All
	}
	return acct
}

// collectBusy queries every selected account and merges their busy time.
func (h *handlers) collectBusy(ctx context.Context, acct string, calIDs []string, q busyQuery) (
	[]perAccount[[]calendar.CalendarBusy], []availability.Interval, []string, error) {
	res, err := fanOut(ctx, h.accounts, defaultAll(acct),
		func(ctx context.Context, c *calendar.Client) ([]calendar.CalendarBusy, error) {
			return c.FreeBusy(ctx, calIDs, q.window)
		})
	if err != nil {
		return nil, nil, nil, err
	}
	var all []availability.Interval
	var warnings []string
	for i := range res {
		if res[i].Error != "" {
			warnings = append(warnings, res[i].Account+": "+res[i].Error)
		}
		for j := range res[i].Items {
			cb := &res[i].Items[j]
			if cb.Error != "" {
				warnings = append(warnings, res[i].Account+"/"+cb.CalendarID+": "+cb.Error)
			}
			for k := range cb.Busy {
				cb.Busy[k] = inZone(cb.Busy[k], q.loc)
			}
			all = append(all, cb.Busy...)
		}
	}
	merged := availability.Merge(all)
	for i := range merged {
		merged[i] = inZone(merged[i], q.loc)
	}
	return res, merged, warnings, nil
}

func inZone(iv availability.Interval, loc *time.Location) availability.Interval {
	return availability.Interval{Start: iv.Start.In(loc), End: iv.End.In(loc)}
}

func (h *handlers) freeBusy(ctx context.Context, _ *mcp.CallToolRequest, a freeBusyArgs) (
	*mcp.CallToolResult, any, error) {
	q, err := h.resolveWindow(ctx, a.Account, a.TimeZone, a.TimeMin, a.TimeMax)
	if err != nil {
		return nil, nil, err
	}
	res, merged, warnings, err := h.collectBusy(ctx, a.Account, a.CalendarIDs, q)
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{
		"timezone": q.loc.String(), "window": inZone(q.window, q.loc),
		"results": res, "merged_busy": merged, "warnings": warnings,
	})
}
