# time-tracking-commands Specification

## Purpose
TBD - created by archiving change document-current-cli-functionality. Update Purpose after archive.
## Requirements
### Requirement: Start a running Toggl entry
The CLI SHALL provide a `start` command that creates a running Toggl time entry in the configured workspace and default project.

#### Scenario: Start command succeeds
- **WHEN** the user runs `toggl start`
- **THEN** the CLI sends a `POST` request to `workspaces/{workspace_id}/time_entries` with `workspace_id`, `project_id`, `created_with: toggl-cli`, the current UTC start time, and `duration: -1`
- **THEN** the CLI prints `Started tracking`

#### Scenario: Start command transport fails
- **WHEN** the Toggl request for `toggl start` returns a transport error
- **THEN** the command returns an error to the root command

### Requirement: Stop the current Toggl entry
The CLI SHALL provide a `stop` command that stops the currently running Toggl entry before building a description.

#### Scenario: Current entry exists
- **WHEN** the user runs `toggl stop` and Toggl returns a current time entry with a non-zero ID
- **THEN** the CLI sets the entry stop time to the current time and sends a `PUT` request to `workspaces/{workspace_id}/time_entries/{entry_id}`

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

