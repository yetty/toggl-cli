# time-tracking-commands Specification

## Purpose
TBD - created by archiving change document-current-cli-functionality. Update Purpose after archive.
## Requirements
### Requirement: Start a running Toggl entry
The CLI SHALL provide a `start` command that creates a running Toggl time entry in the configured workspace and default project, and SHALL print calendar workload status when calendar workload tracking is configured.

#### Scenario: Start command succeeds
- **WHEN** the user runs `toggl start`
- **THEN** the CLI sends a `POST` request to `workspaces/{workspace_id}/time_entries` with `workspace_id`, `project_id`, `created_with: toggl-cli`, the current UTC start time, and `duration: -1`
- **THEN** the CLI prints `Started tracking`

#### Scenario: Start command succeeds with calendar workload tracking
- **WHEN** the user runs `toggl start` and calendar workload tracking is configured
- **THEN** the CLI creates the running Toggl time entry
- **THEN** it prints a brief calendar workload hint containing how long the user should work today to stay on track for the configured monthly hourly budget based on completed worked time and matching calendar events on future days after today

#### Scenario: Start command transport fails
- **WHEN** the Toggl request for `toggl start` returns a transport error
- **THEN** the command returns an error to the root command

### Requirement: Stop the current Toggl entry
The CLI SHALL provide a `stop` command that stops the currently running Toggl entry before building a description, and SHALL print calendar workload status when calendar workload tracking is configured.

#### Scenario: Current entry exists
- **WHEN** the user runs `toggl stop` and Toggl returns a current time entry with a non-zero ID
- **THEN** the CLI sets the entry stop time to the current time and sends a `PUT` request to `workspaces/{workspace_id}/time_entries/{entry_id}`

#### Scenario: Current entry exists with calendar workload tracking
- **WHEN** the user runs `toggl stop`, Toggl returns a current time entry with a non-zero ID, and calendar workload tracking is configured
- **THEN** the CLI stops the current entry
- **THEN** it prints a styled CLI calendar workload overview containing worked hours, planned calendar hours, remaining required hours for upcoming days, and remaining planned calendar hours

#### Scenario: No current entry exists
- **WHEN** the user runs `toggl stop` and Toggl returns a current entry with ID `0`
- **THEN** the command returns an error containing `no running timer`

### Requirement: Save a stopped entry description
After stopping a non-split entry, the CLI SHALL update the stopped Toggl entry description when a non-empty description is available.

#### Scenario: Description is available for non-split entry
- **WHEN** `toggl stop` obtains a non-empty summary or manual description and no project split is applied
- **THEN** the CLI sends a `PUT` request updating the original entry description
- **THEN** the CLI prints `Stopped tracking. Entry:`, the entry details, and `Summary saved.`

#### Scenario: Description is skipped
- **WHEN** `toggl stop` cannot obtain a non-empty description because the user skips manual entry
- **THEN** the CLI removes the worklog file if present
- **THEN** the CLI prints `Stopped tracking. No summary saved.`

### Requirement: Preserve worklog on failed final update
The CLI SHALL preserve the temporary worklog if stopping succeeds but the final Toggl description update fails.

#### Scenario: Final update fails
- **WHEN** `toggl stop` has a description but the Toggl `PUT` that saves the description returns an error
- **THEN** the command returns the error
- **THEN** the worklog file remains available for retry or recovery

### Requirement: Expose CLI version through Cobra root command
The CLI SHALL configure the root command with use name `toggl` and the build-time `version` value.

#### Scenario: Root command is constructed
- **WHEN** the CLI starts successfully
- **THEN** it registers the supported subcommands under a root command named `toggl`

### Requirement: Expose track command
The CLI SHALL register a `track` subcommand under the `toggl` root command for on-demand workload status checks.

#### Scenario: Root command includes track
- **WHEN** the CLI starts successfully
- **THEN** it registers `track` as a supported subcommand alongside the existing time-tracking and metadata commands

#### Scenario: Track command has descriptive help
- **WHEN** the user lists command help
- **THEN** the `track` command description indicates that it reports calendar workload or budget tracking status

