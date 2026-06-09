# calendar-workload-tracking Specification

## Purpose
Describe calendar-backed workload planning and on-track reporting for Toggl start and stop commands.
## Requirements
### Requirement: Read planned work blocks from Google Calendar
The CLI SHALL read timed Google Calendar events from the configured calendar for the current system-local month and include only events whose names exactly match configured work-block names.

#### Scenario: Matching timed events are available
- **WHEN** calendar workload tracking is configured with a calendar ID and event names
- **THEN** the CLI queries that calendar for events between the configured local month start and month end
- **THEN** it includes timed events whose summary exactly equals one of the configured event names

#### Scenario: Non-matching events are present
- **WHEN** the configured calendar contains events whose summaries are not listed in the configured event names
- **THEN** those events are excluded from planned-work calculations

#### Scenario: All-day events are present
- **WHEN** the configured calendar contains all-day events without timed start and end timestamps
- **THEN** those events are excluded from planned-work calculations

### Requirement: Calculate monthly workload status
The CLI SHALL calculate calendar workload status for the current system-local month using the configured fixed monthly hourly budget, worked Toggl duration for listed project IDs, elapsed planned duration, remaining budget duration, today's planned duration, and future planned duration.

#### Scenario: Worked and planned durations are calculated
- **WHEN** calendar workload tracking is enabled and the CLI has current-month Toggl entries and matching calendar events
- **THEN** it calculates actual worked hours from stopped Toggl entries in the current month whose project IDs are listed in the YAML configuration
- **THEN** it calculates planned hours from matching calendar event durations in the current month
- **THEN** it calculates remaining required hours as monthly budget minus actual worked hours

#### Scenario: User is ahead of budget
- **WHEN** actual worked hours are greater than or equal to the monthly hourly budget
- **THEN** remaining required hours are reported as zero

### Requirement: Report recommended work for today
The CLI SHALL derive a brief recommendation for how long the user should work today so completed actual work plus matching calendar events on future days after today can exactly reach the monthly hourly budget.

#### Scenario: Future calendar capacity is enough
- **WHEN** the monthly budget is greater than actual worked hours and matching future events after today have planned duration
- **THEN** the recommendation for today is the positive difference between remaining required hours and future planned hours after today, capped at zero when future planned hours already cover the remaining budget

#### Scenario: Today's events and current timer are excluded
- **WHEN** the user runs `toggl start` during a running timer and matching calendar events exist later today
- **THEN** the recommendation excludes the currently running timer duration
- **THEN** the recommendation excludes all calendar events occurring today

#### Scenario: No remaining work is required
- **WHEN** actual worked hours are greater than or equal to the monthly hourly budget
- **THEN** the recommendation says no additional work is needed today for the monthly budget

### Requirement: Handle calendar tracking failures visibly
The CLI SHALL report calendar workload tracking failures when tracking is configured and required data cannot be loaded.

#### Scenario: Google Calendar request fails
- **WHEN** calendar workload tracking is configured and the Google Calendar request fails
- **THEN** the command reports an error containing the calendar failure reason

#### Scenario: Calendar tracking is not configured
- **WHEN** the user runs `toggl start` or `toggl stop` without calendar workload tracking configuration
- **THEN** the CLI does not query Google Calendar
- **THEN** no calendar workload status is printed

### Requirement: Report calendar workload on demand
The CLI SHALL provide a read-only `track` command that reports current calendar workload status using the existing calendar workload configuration, Toggl worked-time reads, Google Calendar planned-time reads, and monthly budget calculation.

#### Scenario: Track command succeeds with calendar workload tracking
- **WHEN** the user runs `toggl track` and calendar workload tracking is configured
- **THEN** the CLI queries the configured calendar for matching current-month planned work events
- **THEN** the CLI queries Toggl for current-month stopped time entries in configured calendar project IDs
- **THEN** it prints a styled calendar workload overview containing worked hours, planned calendar hours, remaining required work, future planned work, and recommended work for today

#### Scenario: Track command is read-only
- **WHEN** the user runs `toggl track`
- **THEN** the CLI does not create, stop, split, or update any Toggl time entry
- **THEN** the CLI does not read or remove the temporary worklog file
- **THEN** the CLI does not request an OpenAI summary or inspect local git repositories

#### Scenario: Calendar tracking is not configured for track
- **WHEN** the user runs `toggl track` without calendar workload tracking configuration
- **THEN** the command returns an error indicating that calendar workload tracking is not configured
- **THEN** the CLI does not query Google Calendar

#### Scenario: Track command calendar request fails
- **WHEN** the user runs `toggl track` and the Google Calendar request fails
- **THEN** the command returns an error containing the calendar failure reason

