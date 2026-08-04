## Why

`toggl track` currently reports current-month workload status, but it does not show whether the next month already has enough planned work scheduled. Users need a quick forward-looking check before the month begins so they can adjust their calendar early.

## What Changes

- Extend the read-only `toggl track` output with one additional line showing the total planned work hours for the next calendar month.
- The new line indicates whether that next-month planned total meets the configured monthly hour budget.
- Reuse the existing calendar workload configuration for matching work event names and the configured monthly hour budget.
- Preserve existing current-month `toggl track`, `start`, and `stop` behavior.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `calendar-workload-tracking`: Add forward-looking next-month planned-hours reporting to the `toggl track` command.

## Non-goals

- Do not change how actual worked Toggl time is calculated for the current month.
- Do not add a new command, configuration key, or interactive prompt.
- Do not change `toggl start` or `toggl stop` calendar hints.
- Do not modify Toggl entries, calendar events, worklogs, git inspection, or OpenAI summarization.

## Impact

- Affects the `track` command's printed workload overview and calendar-fetching logic.
- Requires reading matching Google Calendar events for next month's local-month bounds in addition to the current month when `toggl track` is run.
- Requires tests/spec coverage for next-month planned-hours output and threshold indication.
