## ADDED Requirements

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
