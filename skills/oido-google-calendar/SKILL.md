---
name: oido-google-calendar
description: Read and manage Google Calendar events, free/busy, free slots and invitations across one or more Google accounts
---

# Oido Google Calendar Extension

## Overview

Tools over the Google Calendar API v3. Use them when the user asks about their schedule, wants to
book or move meetings, find a free time, or answer an invitation. Results are JSON.

## Conventions

- **account**: optional when one account is configured. Read-only tools accept `all` and return
  one entry per account, each with its own `error` if that account failed.
- **Times**: RFC3339 with an offset, or local `2026-05-15T09:00` read in `timezone` (IANA name,
  default: the calendar's zone). A bare date is an all-day event, or the whole day in a range.
- **calendar_id**: defaults to the configured calendar; get others from `g_calendar_list_calendars`.

## Tools

| Tool | Use it when | Notes |
|---|---|---|
| `g_calendar_list_accounts` | You need the account emails | |
| `g_calendar_list_calendars` | The user mentions a non-default calendar | access role and zone included |
| `g_calendar_list_events` | "What's on my calendar this week?" | `time_min` default now; `time_max`, `query`, `max_results` (25) |
| `g_calendar_search_events` | "When is my dentist appointment?" | `query` required; searches from now unless `time_min` is set |
| `g_calendar_get_event` | Details, guests, Meet link of one event | `event_id` from list/search |
| `g_calendar_create_event` | Booking a meeting | `summary`, `start_time`, `end_time` (default +1h), `attendees`, `recurrence`, `reminders`, `add_meet`, `send_updates` |
| `g_calendar_update_event` | Moving or editing an event | only given fields change; `add_attendees` / `remove_attendees` |
| `g_calendar_delete_event` | Cancelling an event | first call previews; pass `confirm: true` after the user agrees |
| `g_calendar_quick_add` | The user dictates a sentence | Google parses "Lunch with Sam Friday 1pm" |
| `g_calendar_respond_to_event` | Accept, decline, maybe | `response`: accepted, declined, tentative; optional `comment` |
| `g_calendar_freebusy` | Raw busy blocks in a window | merged across accounts in `merged_busy` |
| `g_calendar_find_free_slots` | Proposing meeting times | `duration_minutes`; working hours default 09:00-17:00 Mon-Fri |

## Best practices

1. **Search before acting**: get the `event_id` from list or search; never invent ids.
2. **Confirm side effects**: ask before deleting, and before `send_updates: "all"` emails guests.
3. **Find, then propose**: use `g_calendar_find_free_slots` rather than guessing free times.
4. **Speak the user's zone**: pass `timezone` when the user names one and echo times in it.
5. **Report partial failures**: when `all` returns an `error` for one account, say so.

## Example

```
User: Move tomorrow's standup to 10:30 and tell everyone.

Assistant: [g_calendar_search_events query: "standup", time_min: "2026-05-15", time_max: "2026-05-15"]
[g_calendar_update_event event_id: "abc123", start_time: "2026-05-15T10:30",
 end_time: "2026-05-15T10:45", send_updates: "all"]
Standup moved to 10:30-10:45 tomorrow; attendees were notified.
```
