# Oido Google Calendar MCP

A Google Calendar and Google Tasks [MCP](https://modelcontextprotocol.io) server in Go for
assistant agents: one endpoint, several Google accounts, and the scheduling tools an email +
calendar agent actually needs (agenda across accounts, free/busy, free-slot finding, RSVPs, Meet
links), plus a daily to-do list with an idempotent morning rollover.

> **Fork of [Djancyp/oido-google-calander](https://github.com/Djancyp/oido-google-calander).**
> Upstream is a single-account stdio server for the Oido plugin system, authenticated with a
> service account. This fork keeps that mode working and adds a multi-account HTTP service with
> an admin dashboard, Google OAuth, encrypted token storage, eight new calendar tools and
> eight Google Tasks tools.
>
> **License:** upstream publishes no license, so this fork does not add one either and does not
> relicense anything. Without a license, default copyright applies to the upstream code; ask the
> upstream author before reusing it outside GitHub's fork-and-view terms.

## Features

- **Two transports**: Streamable HTTP (`POST /mcp`, static bearer token) for remote agents, and
  stdio for local clients and the Oido plugin (upstream behaviour, unchanged).
- **Multi-account**: connect any number of Google accounts through the `/setup` dashboard. Every
  tool takes `account`; read-only tools also accept `all` and fan out in parallel, reporting
  per-account errors inline instead of failing the whole call.
- **Scheduling**: merged free/busy across accounts and a free-slot finder that respects working
  hours, working days, buffers, slot alignment and DST.
- **Events**: list, search, get, create, update, delete (two-step, needs `confirm:true`),
  natural-language quick add, and RSVP. Create supports guests, optional guests, reminders,
  RFC 5545 recurrence and Google Meet links.
- **Timezones done properly**: RFC3339 input keeps its offset; naive input (`2026-05-15T09:00`) is
  read in the `timezone` argument, else the calendar's own zone, else `DEFAULT_TIMEZONE`. A bare
  date means an all-day event, or the whole day in a range. Zone data is compiled in.
- **Google Tasks** (HTTP service): task lists, tasks with notes, due dates and subtasks, and a
  daily rollover that moves unfinished tasks to today and clears completed ones.
- **Admin dashboard**: password login with throttling, connected accounts with a live snapshot of
  each account's calendars and next events, one-click connect and remove, light and dark themes,
  phone layout. No external requests, strict CSP.

## Tools

All tools take an optional `account` (email). It may be omitted when exactly one account is
connected. Results are JSON.

| Tool | Purpose | Key arguments |
|---|---|---|
| `g_calendar_list_accounts` | Connected accounts | |
| `g_calendar_list_calendars` | Calendars per account, with access role and zone | `account` (or `all`) |
| `g_calendar_list_events` | Events in a range, recurring events expanded | `time_min`, `time_max`, `query`, `calendar_id`, `timezone`, `max_results` |
| `g_calendar_search_events` | Text search over title, description, location, guests | `query`, range as above |
| `g_calendar_get_event` | One event in full (guests, responses, Meet link, recurrence) | `event_id` |
| `g_calendar_create_event` | Create an event | `summary`, `start_time`, `end_time`, `attendees`, `optional_attendees`, `recurrence`, `reminders`, `add_meet`, `send_updates` |
| `g_calendar_update_event` | Patch an event; omitted fields stay | `event_id`, any create field, `add_attendees`, `remove_attendees` |
| `g_calendar_delete_event` | Preview, then delete with `confirm:true` | `event_id`, `confirm`, `send_updates` |
| `g_calendar_quick_add` | Create from a sentence ("Lunch with Sam Friday 1pm") | `text` |
| `g_calendar_respond_to_event` | Accept, decline or tentatively accept an invite | `event_id`, `response`, `comment` |
| `g_calendar_freebusy` | Busy blocks per account plus the merged union | `account` (default `all`), `time_min`, `time_max` |
| `g_calendar_find_free_slots` | Free ranges open on every selected account | `duration_minutes`, `working_hours_start`/`_end`, `working_days`, `buffer_minutes`, `granularity_minutes`, `timezone` |

## Tasks

The HTTP service also exposes Google Tasks, for a daily to-do workflow: the agent adds tasks from
mail and from "today I want to..." messages, and every morning `g_tasks_rollover_tasks` moves
unfinished tasks to today and clears the completed ones. Task tools act on one account; `list`
defaults to `@default` (the account's "My Tasks").

| Tool | Purpose | Key arguments |
|---|---|---|
| `g_tasks_list_tasklists` | Task lists of an account | `account` |
| `g_tasks_list_tasks` | Tasks of a list, filtered | `list`, `status` (`open`, `completed`, `all`), `due_before`, `due_after`, `due_on`, `include_completed`, `include_hidden`, `timezone` |
| `g_tasks_create_task` | Create a task or subtask | `title`, `notes`, `due`, `parent`, `list` |
| `g_tasks_update_task` | Change title, notes, due date or status | `task_id`, `title`, `notes`, `due` (empty removes it), `status` |
| `g_tasks_complete_task` | Mark a task completed | `task_id`, `list` |
| `g_tasks_delete_task` | Preview, then delete with `confirm:true` | `task_id`, `confirm` |
| `g_tasks_clear_completed` | Hide all completed tasks of a list (Google's clear); count only without `confirm:true` | `list`, `confirm` |
| `g_tasks_rollover_tasks` | Open tasks due before today get due today; optionally undated ones too; optionally clear completed. Returns counts, the moves and today's list | `list`, `timezone`, `include_undated`, `clear_completed`, `dry_run` |

- **Due is a date only.** The Google Tasks API stores a due date and discards any time of day,
  so `due` takes `YYYY-MM-DD`, `today` or `tomorrow`; a timestamp is reduced to its date in
  `timezone` and its time is ignored.
- **"Today" has a zone.** It is computed in `timezone` (default `Europe/Amsterdam`), so a run at
  00:30 local time already sees the new day even though UTC does not, including on DST nights.
- **Rollover is idempotent.** After a run no open task is due before today, so running it again
  the same day changes nothing. `dry_run:true` reports the same result without writing.
- **Extra OAuth scope.** Tasks need `https://www.googleapis.com/auth/tasks`, which
  `/oauth/start` now requests. Accounts connected before Tasks support lack it: the dashboard
  shows **Tasks off** with a **Reconnect to enable Tasks** button, and task tools return an
  error asking for a reconnect. Reconnecting the same Google account updates it in place (same
  entry, new token and scopes); calendar access keeps working until then. Granted scopes are
  recorded at connect time; for older accounts they are read once from Google and cached.
- Stdio mode (service account) has no Tasks tools: Tasks belong to a user account.

## Architecture

```
cmd/oido-google-calendar/     entry point: stdio (default), `serve`, `healthcheck`
internal/
  config/                     env parsing, fail-fast validation, upstream --config/.env support
  auth/crypt/                 AES-256-GCM sealing of refresh tokens (scrypt-derived key)
  auth/googleauth/            OAuth web flow: consent URL, code exchange, verified email
  store/                      encrypted account store (atomic 0600 JSON file)
  account/                    account name -> calendar/tasks client, granted-scope checks
  calendar/                   Google Calendar API wrapper and compact event views
    timeinput/                time and timezone parsing
    availability/             interval merging and the free-slot search (pure, no Google types)
  tasks/                      Google Tasks API wrapper, task views and filters
    due/                      date-only due values, "today" in a zone
    rollover/                 the daily rollover: pure plan plus an idempotent run
  tools/                      MCP tool schemas, validation and multi-account fan-out
    tasktools/                the g_tasks_* tools
  server/                     HTTP mux: /mcp (bearer), /health
    clientip/                 real client IP behind trusted proxies
    admin/                    /setup dashboard and /oauth/start, /oauth/callback
      access/                 login throttle, sessions, constant-time password check
      views/                  embedded templates and CSS
```

The HTTP service is stateless towards MCP clients (each request gets a fresh session) and keeps
one cached, auto-refreshing token source per connected account.

## Quick start (Docker)

```bash
git clone https://github.com/tr4m0ryp/oido-google-calander && cd oido-google-calander
cp .env.example .env            # fill in the values, see below
chmod 600 .env
docker compose up -d --build    # listens on 127.0.0.1:3110
curl -s http://127.0.0.1:3110/health
```

Put a TLS reverse proxy in front. With Caddy:

```
calendar.example.com {
	reverse_proxy 127.0.0.1:3110
}
```

Then open `https://calendar.example.com/setup`, sign in with `ADMIN_PASSWORD`, and click
**Connect Google account** for each account.

## Google OAuth setup

1. In the [Google Cloud console](https://console.cloud.google.com/), pick or create a project and
   enable the **Google Calendar API** and the **Google Tasks API**
   (`gcloud services enable calendar-json.googleapis.com tasks.googleapis.com`).
2. Under **Google Auth Platform > Branding / Audience**, configure the consent screen. While the
   app is in *Testing*, add every Google account you will connect as a test user.
3. Under **Data access**, add the scopes `https://www.googleapis.com/auth/calendar` and
   `https://www.googleapis.com/auth/tasks` (plus `openid` and `email`). Unverified apps still work for test users after a warning screen.
4. Under **Clients**, create an OAuth client of type **Web application** and add the authorized
   redirect URI `https://calendar.example.com/oauth/callback` (that is, `SERVER_URL` +
   `/oauth/callback`).
5. Copy the client ID and secret into `GOOGLE_CLIENT_ID` and `GOOGLE_CLIENT_SECRET`.

One OAuth client can serve several apps; add one redirect URI per deployment.

## Configuration

| Variable | Required | Default | Meaning |
|---|---|---|---|
| `SERVER_URL` | yes | `http://localhost:$PORT` | Public base URL; builds the MCP URL and the OAuth redirect URI |
| `GOOGLE_CLIENT_ID` | yes | | OAuth client ID |
| `GOOGLE_CLIENT_SECRET` | yes | | OAuth client secret |
| `ADMIN_PASSWORD` | yes | | Dashboard password, 12+ characters |
| `ENCRYPTION_KEY` | yes | | Secret for sealing refresh tokens (`openssl rand -hex 32`); changing it orphans stored tokens |
| `MCP_API_KEY` | yes | | Bearer token MCP clients must send (`openssl rand -hex 32`) |
| `MCP_ALLOW_UNAUTHENTICATED` | no | `false` | Allow an empty `MCP_API_KEY`; private networks only |
| `DATA_DIR` | no | `./data` (`/data` in Docker) | Where `accounts.json` lives |
| `PORT` / `BIND` | no | `3000` / `0.0.0.0` | Listen address |
| `DEFAULT_TIMEZONE` | no | `UTC` | Last-resort zone for naive times |
| `ADMIN_LOGIN_MAX_FAILURES` | no | `5` | Failed logins per IP before lockout |
| `ADMIN_LOGIN_WINDOW_MINUTES` | no | `15` | Sliding window for counting failures |
| `ADMIN_LOGIN_LOCKOUT_MINUTES` | no | `15` | Lockout length (answers 429 with `Retry-After`) |
| `ADMIN_LOGIN_GLOBAL_MAX_FAILURES` | no | `50` | Failures across all IPs that pause every login (0 = off) |
| `ADMIN_SESSION_HOURS` | no | `12` | Admin session lifetime |
| `TRUSTED_PROXIES` | no | loopback and private ranges | CIDRs allowed to set `X-Forwarded-For` |

Stdio mode reads the upstream variables instead: `CALENDAR_SERVICE_ACCOUNT_JSON` or
`CALENDAR_SERVICE_ACCOUNT_B64`, `CALENDAR_ID` and `CALENDAR_TIMEZONE`, from the environment, a
`.env` next to the binary, or `--config '<json>'`.

## Connecting an MCP client

Every request to `/mcp` needs `Authorization: Bearer <MCP_API_KEY>`.

Claude Code:

```bash
claude mcp add --transport http google-calendar https://calendar.example.com/mcp \
  --header "Authorization: Bearer $MCP_API_KEY"
```

Generic JSON client configuration:

```json
{
  "mcpServers": {
    "google-calendar": {
      "type": "http",
      "url": "https://calendar.example.com/mcp",
      "headers": { "Authorization": "Bearer <MCP_API_KEY>" }
    }
  }
}
```

Smoke test:

```bash
curl -s https://calendar.example.com/mcp \
  -H "Authorization: Bearer $MCP_API_KEY" -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}'
```

### Stdio and the Oido plugin

Without arguments the binary speaks MCP over stdio with a service account, exactly like
upstream, so `plugin.json`, `oido-extension.json` and `make dist` keep working. Share the target
calendar with the service account's email and set `CALENDAR_ID` to it.

```bash
make build
CALENDAR_SERVICE_ACCOUNT_B64=$(base64 < key.json) CALENDAR_ID=you@example.com ./oido-google-calendar-mcp
```

## Security notes

- Refresh tokens are sealed with AES-256-GCM (random nonce, account email as associated data, key
  derived with scrypt) and written atomically with mode 0600. Access tokens live only in memory.
- `/mcp` compares the bearer token in constant time. The server refuses to start without
  `MCP_API_KEY` unless `MCP_ALLOW_UNAUTHENTICATED=true`.
- The dashboard uses a server-side session in an `HttpOnly`, `SameSite=Lax` cookie (`Secure` on
  HTTPS); the password never appears in a URL. Form posts carry a CSRF token. Scripts may send
  the password in an `X-Admin-Key` header instead.
- Failed logins are throttled per client IP (default 5 per 15 minutes, then a 15-minute lockout
  with `429` and `Retry-After`) and globally. The client IP comes from `X-Forwarded-For` only when
  the direct peer is a trusted proxy, so bind the service to localhost behind the proxy.
- The OAuth `state` is a single-use nonce bound to the admin session that started the flow and
  expires after 10 minutes.
- Admin pages send a strict nonce-based CSP, `no-store`, `no-referrer` and `DENY` framing, and
  never render secrets. Request logs record method, path and status only.
- Deleting an event or a task, and clearing completed tasks, is two-step: without
  `confirm:true` the tool returns a preview.

## Development

```bash
make test        # go vet + go test ./...
make build       # ./oido-google-calendar-mcp
VIEWS_PREVIEW_DIR=/tmp/preview go test ./internal/server/admin/views -run Preview
                 # renders fixture dashboard pages for visual review
```
