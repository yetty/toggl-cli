## Why

Users plan focused work in Google Calendar but currently only see Toggl time after the fact. Connecting the CLI to calendar work blocks lets `toggl start` and `toggl stop` show whether today's work pace and the remaining monthly plan are aligned with a configured hourly budget.

## What Changes

- Add optional Google Calendar planning configuration to `~/.toggl.yaml`, including calendar ID, exact event-name matches, and a single fixed monthly hourly budget.
- When running `toggl start`, print a brief on-track hint showing how long the user should work today, taking into account time already worked and matching future calendar blocks after today in the current month.
- When running `toggl stop`, print a styled CLI overview comparing worked hours vs planned calendar hours, plus remaining required work vs remaining planned calendar hours.
- Keep Toggl tracking behavior functional when calendar planning is not configured.

## Non-goals

- Creating, editing, or deleting Google Calendar events.
- Synchronizing Toggl entries back into Google Calendar.
- Supporting multiple independent monthly budgets in one command run.
- Counting today's calendar events or the currently running timer in the `toggl start` recommendation.
- Replacing the existing AI summary, git commit collection, worklog, or project split behavior.

## Capabilities

### New Capabilities
- `calendar-workload-tracking`: Calendar-backed workload planning and on-track reporting for `toggl start` and `toggl stop`.

### Modified Capabilities
- `cli-configuration`: Add optional Google Calendar planning configuration fields.
- `time-tracking-commands`: Extend `start` and `stop` output with calendar workload status when configured.

## Impact

- Affected code: `src/main.go`, configuration structs/loading, `start` and `stop` command flows, and tests or testable helpers added around calendar calculations.
- External APIs: Google Calendar API or the simplest viable read-only calendar access path for a shareable calendar; existing Toggl API calls continue as today.
- Dependencies: to be determined by an implementation spike, with preference for a non-OAuth option such as a shareable calendar/feed or service-account access.
- User-visible behavior: additional informational output from `toggl start` and `toggl stop` when calendar tracking is enabled; no output changes are required when it is disabled.
