## ADDED Requirements

### Requirement: Enable splitting from project configuration and project commits
The stop workflow SHALL consider project time-entry splitting only when project configuration exists and project commits are collected for the stopped session.

#### Scenario: Project configuration has no matching commits
- **WHEN** `projects` is configured but no project commits are collected in the stopped session window
- **THEN** the CLI keeps the original entry project and uses the non-split summary flow

#### Scenario: Project commits are present
- **WHEN** `projects` is configured and at least one project commit is collected
- **THEN** the CLI calculates project splits and applies project-specific time entries instead of using legacy repository splitting behavior

### Requirement: Allocate split durations using commit midpoints
The CLI SHALL approximate time allocation by assigning intervals around each project commit using midpoints between neighboring commit timestamps.

#### Scenario: Commits belong to two projects
- **WHEN** the stopped entry spans from `09:00` to `13:00` and commits occur at `09:30` for project A and `11:30` plus `12:30` for project B
- **THEN** project A receives `09:00` to `10:30`
- **THEN** project B receives `10:30` to `13:00`

#### Scenario: Project commits alternate
- **WHEN** sorted commit ownership alternates between projects
- **THEN** the CLI aggregates all midpoint-derived durations per project before creating final contiguous project entries

### Requirement: Produce contiguous project split entries
The CLI SHALL convert allocated project durations into contiguous time ranges that cover the original stopped entry window.

#### Scenario: Multiple splits are calculated
- **WHEN** project splits are calculated
- **THEN** the first split starts at the original entry start time
- **THEN** each split starts when the previous split stops
- **THEN** the last split stops at the original entry stop time

### Requirement: Summarize each project split independently
The CLI SHALL generate a description for each project split using only commits belonging to that split's project identity.

#### Scenario: Multiple project splits have different commits
- **WHEN** a stopped session has commits for project A and project B
- **THEN** the OpenAI prompt for project A includes project A commits and excludes project B commits
- **THEN** the OpenAI prompt for project B includes project B commits and excludes project A commits

#### Scenario: Split description cannot be produced
- **WHEN** a project split receives an empty description
- **THEN** the CLI removes the worklog and prints `Stopped tracking. No summary saved.` without applying split entries

### Requirement: Apply project splits to Toggl entries
The CLI SHALL apply the first project split by updating the original stopped entry and SHALL apply later splits by creating stopped Toggl entries.

#### Scenario: Single project split exists
- **WHEN** project splitting produces exactly one split
- **THEN** the CLI updates the original stopped entry with the split project ID, split start and stop times, and split description
- **THEN** the CLI does not create additional Toggl entries

#### Scenario: Multiple project splits exist
- **WHEN** project splitting produces more than one split
- **THEN** the CLI updates the original entry with the first split
- **THEN** the CLI creates one stopped Toggl time entry for each remaining split with `created_with: toggl-cli`, project ID, description, start, stop, and positive duration in seconds

#### Scenario: Split application succeeds
- **WHEN** all split updates and creations succeed
- **THEN** the CLI removes the worklog and prints `Stopped tracking. Split into:` followed by each split entry ID, project name, project ID, time range, duration, and `Summary saved.`

#### Scenario: Original split update fails
- **WHEN** the original entry update for the first split fails
- **THEN** the command returns an error and does not create later split entries
- **THEN** the worklog remains available

#### Scenario: Later split creation fails
- **WHEN** creation of a later split entry fails
- **THEN** the command returns an error and the worklog remains available
