package tools

import (
	"context"
	"errors"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tr4m0ryp/oido-google-calander/internal/calendar/availability"
)

// slotQuery converts tool arguments into an availability query, applying defaults.
func (a freeSlotsArgs) slotQuery(q busyQuery) (availability.SlotQuery, error) {
	if a.DurationMinutes <= 0 {
		return availability.SlotQuery{}, errors.New("duration_minutes must be positive")
	}
	startText, endText := a.WorkingStart, a.WorkingEnd
	if startText == "" {
		startText = defaultWorkingStart
	}
	if endText == "" {
		endText = defaultWorkingEnd
	}
	workStart, err := availability.ParseClock(startText)
	if err != nil {
		return availability.SlotQuery{}, err
	}
	workEnd, err := availability.ParseClock(endText)
	if err != nil {
		return availability.SlotQuery{}, err
	}
	dayNames := a.WorkingDays
	if len(dayNames) == 0 {
		dayNames = defaultWorkDays
	}
	days, err := availability.ParseWeekdays(dayNames)
	if err != nil {
		return availability.SlotQuery{}, err
	}
	step := a.GranularityMin
	if step <= 0 {
		step = defaultSlotStep
	}
	max := a.MaxSlots
	if max <= 0 {
		max = defaultMaxSlots
	}
	return availability.SlotQuery{
		Window: q.window, Duration: time.Duration(a.DurationMinutes) * time.Minute,
		Location: q.loc, WorkStart: workStart, WorkEnd: workEnd, WorkDays: days,
		Granularity: time.Duration(step) * time.Minute,
		Buffer:      time.Duration(max0(a.BufferMinutes)) * time.Minute, MaxSlots: max,
	}, nil
}

func max0(n int) int {
	if n < 0 {
		return 0
	}
	return n
}

// slotView presents a free range with the earliest bookable meeting in it.
type slotView struct {
	Start          time.Time `json:"start"`
	End            time.Time `json:"end"`
	FreeMinutes    int       `json:"free_minutes"`
	SuggestedStart time.Time `json:"suggested_start"`
	SuggestedEnd   time.Time `json:"suggested_end"`
}

func (h *handlers) findFreeSlots(ctx context.Context, _ *mcp.CallToolRequest, a freeSlotsArgs) (
	*mcp.CallToolResult, any, error) {
	q, err := h.resolveWindow(ctx, a.Account, a.TimeZone, a.TimeMin, a.TimeMax)
	if err != nil {
		return nil, nil, err
	}
	sq, err := a.slotQuery(q)
	if err != nil {
		return nil, nil, err
	}
	res, merged, warnings, err := h.collectBusy(ctx, a.Account, a.CalendarIDs, q)
	if err != nil {
		return nil, nil, err
	}
	slots, err := availability.FindSlots(merged, sq)
	if err != nil {
		return nil, nil, err
	}
	views := make([]slotView, len(slots))
	for i, s := range slots {
		views[i] = slotView{Start: s.Start, End: s.End, FreeMinutes: int(s.Duration().Minutes()),
			SuggestedStart: s.Start, SuggestedEnd: s.Start.Add(sq.Duration)}
	}
	accounts := make([]string, len(res))
	for i, r := range res {
		accounts[i] = r.Account
	}
	if len(warnings) > 0 {
		warnings = append(warnings, "some calendars could not be read; slots may overlap their events")
	}
	return jsonResult(map[string]any{
		"timezone": q.loc.String(), "window": inZone(q.window, q.loc), "accounts": accounts,
		"duration_minutes": a.DurationMinutes, "slots": views, "warnings": warnings,
	})
}
