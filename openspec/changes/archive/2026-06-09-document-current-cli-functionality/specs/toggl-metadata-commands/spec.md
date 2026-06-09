## ADDED Requirements

### Requirement: Show current Toggl user information
The CLI SHALL provide a `whoami` command that retrieves and prints the current Toggl user's ID, full name, and email address.

#### Scenario: Toggl user response is valid
- **WHEN** the user runs `toggl whoami` and Toggl returns a valid `me` response with `data.id`, `data.fullname`, and `data.email`
- **THEN** the CLI prints `User ID:`, `Full Name:`, and `Email:` lines containing those values

#### Scenario: Toggl user response is malformed
- **WHEN** the user runs `toggl whoami` and the response cannot be decoded as the expected JSON shape
- **THEN** the command returns a JSON decoding error

### Requirement: List workspace projects
The CLI SHALL provide a `projects` command that lists projects for the configured Toggl workspace.

#### Scenario: Workspace has projects
- **WHEN** the user runs `toggl projects` and Toggl returns one or more projects for `toggl.workspace_id`
- **THEN** the CLI prints `Projects:` followed by each project's ID and name

#### Scenario: Workspace has no projects
- **WHEN** the user runs `toggl projects` and Toggl returns an empty project list
- **THEN** the CLI prints `No projects found in this workspace.`

#### Scenario: Project list response is malformed
- **WHEN** the user runs `toggl projects` and the response cannot be decoded as a project list
- **THEN** the command returns a JSON decoding error
