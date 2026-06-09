## Context

The CLI currently loads a single YAML config, starts/stops Toggl time entries, and enriches stopped entries with git/worklog summaries. Users also maintain planned work blocks in Google Calendar, but the CLI does not read those blocks or compare them with Toggl time against a monthly budget. This change adds an optional read-only planning integration that augments `toggl start` and `toggl stop` output without changing the existing Toggl entry lifecycle.

## Goals / Non-Goals

**Goals:**
- Read exact-name matching events from one configured Google Calendar for the current local month.
- Compare worked Toggl time, planned calendar time, and a configured monthly hourly budget.
- Print concise status information on `start` and a styled CLI block progress overview on `stop`.
- Preserve current behavior when calendar tracking is absent or disabled.

**Non-Goals:**
- Mutating Google Calendar data.
- Changing how Toggl entries are created, stopped, split, or summarized.
- Introducing multi-calendar scheduling, variable monthly budgets, or per-project budget allocation.
- Using Google Calendar as the source of actual worked time.
- Counting today's calendar events or the currently running Toggl timer in the `toggl start` recommendation.

## Decisions

1. **Use optional nested calendar configuration.**
   - Add a `calendar` config section with `id`, `event_names`, `monthly_hour_budget`, and credential/access fields needed for read-only calendar access.
   - Rationale: keeps the integration opt-in and avoids affecting existing config files.
   - Alternative considered: infer event names or budget from Toggl projects. Rejected because the user explicitly wants calendar ID and event names in YAML and a separate fixed monthly budget.

2. **Calculate tracking status from current-month local boundaries.**
   - Use the system local timezone, determine month start/end and today start/end, and classify calendar blocks as elapsed, today, or future.
   - Rationale: budget and planning questions are framed as “today”, “next days”, and “in the month”, which are local-calendar concepts rather than UTC concepts.
   - Alternative considered: make timezone configurable. Deferred until there is a real need; system local timezone keeps configuration smaller.

3. **Use Toggl time entries as actual worked time.**
   - Query Toggl entries for the configured workspace/month and sum stopped durations only for project IDs listed in the YAML config.
   - Rationale: Toggl remains the system of record for actual work, while the configured project list defines the scope of this budget.
   - Alternative considered: count all workspace entries. Rejected because unrelated Toggl work would distort the configured monthly budget.

4. **Keep `start` recommendation forward-looking after today.**
   - `toggl start` should not include today's calendar events and should not include the currently running timer. It should use completed worked time plus future matching calendar events after today.
   - Rationale: the user wants to know what they need to work today so future days still land exactly on the monthly budget.
   - Alternative considered: include today's remaining calendar events. Rejected because the user explicitly wants only future days after today.

5. **Make calendar status best-effort by default.**
   - If calendar tracking is disabled, print exactly the existing outputs. If it is enabled but Google Calendar or Toggl history lookup fails, return an actionable error before/after the main Toggl operation only when the command needs that data for configured tracking.
   - Rationale: disabled users should see no regression; enabled users should learn when their on-track report cannot be trusted.
   - Alternative considered: silently skip failed calendar status. Rejected because stale or missing planning output could mislead the user.

6. **Keep reporting separate from summary generation and project splitting.**
   - Calendar status is printed in addition to current `start` and `stop` messages and does not feed OpenAI prompts or split logic.
   - Rationale: the integration is informational and should not alter Toggl descriptions or project allocation.
   - Alternative considered: include calendar context in AI summaries. Rejected as out of scope and not required for on-track feedback.

7. **Investigate the simplest non-OAuth calendar access path before implementation.**
   - Use Google Calendar API key access to a calendar that is public/shareable enough for read-only API-key reads.
   - Rationale: the user can share the calendar and wants the simplest setup without OAuth. API-key access uses the Calendar `events` endpoint directly and can return expanded timed event instances for a month with no OAuth token exchange.
   - Service-account access was considered and remains viable for private calendars, but it requires Google Cloud service-account creation, sharing the calendar with the service-account email, signed JWT/OAuth token exchange, and more credential handling in the CLI.
   - Alternative considered: implement OAuth user login first. Rejected as unnecessarily complex until the non-OAuth options are ruled out.

## Risks / Trade-offs

- **Google access complexity** → Start with an investigation task for shareable calendar/feed or service-account access; avoid OAuth unless simpler options cannot satisfy private calendar access.
- **Timezone and daylight-saving edge cases** → Use Go `time.Location` and event start/end timestamps rather than fixed 24-hour assumptions.
- **Calendar all-day or recurring event handling** → Rely on Google Calendar expanded instances for the query window and ignore all-day events unless they have timed start/end values.
- **Output becoming noisy** → Keep `start` to a brief hint and render `stop` as a compact styled CLI block with detailed worked/planned/remaining numbers.
- **API failures after stopping the Toggl entry** → Do not undo a successful Toggl stop; report the calendar-status failure separately while preserving existing worklog/summary behavior.

## Migration Plan

1. Existing users without `calendar:` configuration continue with unchanged behavior.
2. Users add `calendar:` settings to enable reports.
3. Documentation shows a token-free placeholder example and explains the selected non-OAuth read-only calendar setup.
4. Rollback is removing or disabling the `calendar:` section; no persistent data migration is required.
