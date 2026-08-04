## Context

Calendar workload tracking already calculates current-month worked and planned hours for `toggl track` using the configured calendar ID, event names, monthly hour budget, and Toggl project IDs. The requested change is limited to the read-only `track` command output: it needs to add a next-month planning signal without changing current-month budget math or any Toggl mutation workflow.

## Goals / Non-Goals

**Goals:**

- Add one forward-looking line to `toggl track` showing total matching planned work for the next local calendar month.
- Indicate on that line whether the next-month planned total meets or falls short of the configured monthly hour budget.
- Keep existing configuration semantics and current-month workload output intact.

**Non-Goals:**

- Do not change `start` or `stop` calendar hints.
- Do not count next-month Toggl worked time or create a second complete workload status for next month.
- Do not introduce new configuration for month-specific budgets or event names.
- Do not mutate Toggl entries, Google Calendar events, worklogs, or summaries.

## Decisions

- Fetch next-month calendar events only for `toggl track`.
  - Rationale: the new signal is an on-demand planning overview, while `start` and `stop` should remain focused on today's current-month recommendation.
  - Alternative considered: adding next-month data to the shared workload status loaded by all commands. That would risk extra Google Calendar calls and output changes in unrelated workflows.

- Reuse the current `monthly_hour_budget` as the next-month expected threshold.
  - Rationale: the existing configuration exposes one fixed monthly expectation, and the request asks whether planned hours meet the expected number.
  - Alternative considered: adding a separate next-month expected-hours key. That would expand configuration without evidence that separate budgets are needed.

- Derive next-month planned hours from the same matching calendar-event filter used for the current month.
  - Rationale: users should see a directly comparable planned-work total, and non-matching or all-day events should continue to be excluded.
  - Alternative considered: count all calendar events. That would make next-month output inconsistent with existing planned-work reporting.

## Risks / Trade-offs

- Extra Google Calendar request during `toggl track` → Keep it scoped to `track` and surface failures through the existing calendar workload error path.
- Boundary/time-zone mistakes around month transitions → Reuse local-month boundary helper patterns and add tests for next-month bounds.
- Ambiguous wording for threshold result → Use explicit output states such as `meets expected 120.0h` or `below expected 120.0h`.
