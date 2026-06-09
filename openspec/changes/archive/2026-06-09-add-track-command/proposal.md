## Why

Calendar workload status is currently only visible as a side effect of starting or stopping a timer. Users need a read-only way to check whether they are on track for monthly budget hours without creating, stopping, or modifying Toggl entries.

## What Changes

- Add a new `toggl track` command that reports the current calendar workload status on demand.
- Reuse the existing calendar workload calculations and styled overview used by `toggl stop` so users see worked hours, planned calendar time, remaining required work, future planned work, and the recommendation for today.
- When calendar workload tracking is not configured, the command should fail visibly instead of silently doing nothing, because `track` exists specifically to show that configured status.
- No breaking changes to `start`, `stop`, existing configuration, or Toggl entry mutation behavior.

## Non-goals

- Changing the monthly workload calculation formula.
- Adding new calendar providers, authentication modes, or calendar configuration fields.
- Creating, stopping, splitting, or updating Toggl entries from `toggl track`.
- Adding interactive prompts or AI-generated summaries to the track workflow.

## Capabilities

### New Capabilities
- None.

### Modified Capabilities
- `calendar-workload-tracking`: Add on-demand calendar workload reporting through a read-only `toggl track` command.
- `time-tracking-commands`: Register and document the new `track` CLI command without changing start/stop behavior.

## Impact

- Affected code: `src/main.go` command registration and command helper functions.
- Affected docs: `README.md` command list and calendar tracking description.
- Affected specs: calendar workload reporting and root command registration expectations.
- External systems: reads Google Calendar and Toggl time entries when configured; does not write to Toggl or OpenAI.
