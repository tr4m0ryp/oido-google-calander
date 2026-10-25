# Oido Google Calendar Extension

Read and manage Google Calendar through MCP tools. In the Oido plugin the server runs over stdio
with a service account and acts on the calendar in `CALENDAR_ID`; the same binary can also run as
a multi-account HTTP service (see README).

## Conventions

- `account` is optional on every tool when one account is configured; read-only tools accept
  `all` to merge results across accounts.
- Times: RFC3339 with an offset (`2026-05-15T09:00:00+02:00`), or a local time
  (`2026-05-15T09:00`) read in `timezone` (IANA name, default: the calendar's zone). A bare date
  (`2026-05-15`) means an all-day event, or the whole day when used as a range bound.
- `calendar_id` defaults to the configured calendar.
- Results are JSON; event ids from list/search feed get/update/delete/respond.

## Tools

- `g_calendar_list_accounts`: accounts this server can use.
- `g_calendar_list_calendars`: calendars with access role and time zone.
- `g_calendar_list_events`: events between `time_min` (default now) and `time_max`, optional `query`.
- `g_calendar_search_events`: text search (`query` required) over title, description, location, guests.
- `g_calendar_get_event`: one event in full by `event_id`.
- `g_calendar_create_event`: `summary`, `start_time`, optional `end_time` (default +1h),
  `attendees`, `optional_attendees`, `recurrence` (e.g. `RRULE:FREQ=WEEKLY;COUNT=5`),
  `reminders`, `add_meet`, `send_updates`.
- `g_calendar_update_event`: patch by `event_id`; only the fields given change.
- `g_calendar_delete_event`: returns a preview; deletes only with `confirm: true`.
- `g_calendar_quick_add`: create from a sentence, e.g. "Dentist Tuesday 3pm".
- `g_calendar_respond_to_event`: `response` accepted, declined or tentative.
- `g_calendar_freebusy`: busy blocks in a window, merged across accounts.
- `g_calendar_find_free_slots`: free ranges of `duration_minutes` within working hours and days.

## Guidelines

1. Confirm with the user before deleting, and before sending invitations (`send_updates: all`).
2. Use `g_calendar_find_free_slots` before proposing meeting times instead of guessing.
3. State times in the user's zone; pass `timezone` when the user names one.

## Example

```
User: Find an hour for a call with the team next week and send the invite.

Assistant: [g_calendar_find_free_slots duration_minutes: 60, time_min: "2026-05-18", time_max: "2026-05-22"]
Tuesday 10:00-11:00 and Wednesday 14:00-15:00 are free. Which one?

User: Tuesday. Invite sam@example.com with a Meet link.

Assistant: [g_calendar_create_event summary: "Team call", start_time: "2026-05-19T10:00",
            attendees: ["sam@example.com"], add_meet: true, send_updates: "all"]
Created "Team call" on Tuesday 10:00-11:00 with a Meet link; Sam has been invited.
```
