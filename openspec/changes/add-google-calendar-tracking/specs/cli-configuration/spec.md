## MODIFIED Requirements

### Requirement: Load user configuration file
The CLI SHALL load configuration from a file named `.toggl.yaml` in the current user's home directory before executing commands.

#### Scenario: Valid configuration is available
- **WHEN** the user runs the CLI and `~/.toggl.yaml` exists with valid YAML
- **THEN** the CLI loads Toggl, OpenAI, git, repository, project, and optional calendar workload configuration fields from that file

#### Scenario: Configuration file cannot be read
- **WHEN** the user runs the CLI and `~/.toggl.yaml` cannot be read
- **THEN** the CLI prints `Error loading config:` followed by the error and exits with a non-zero status

#### Scenario: Configuration YAML is invalid
- **WHEN** the user runs the CLI and `~/.toggl.yaml` contains invalid YAML
- **THEN** the CLI prints `Error loading config:` followed by the parse error and exits with a non-zero status

## ADDED Requirements

### Requirement: Support calendar workload configuration fields
The CLI SHALL support optional calendar workload configuration containing calendar ID, exact event names, a fixed monthly hourly budget, and the selected read-only Google Calendar access settings.

#### Scenario: Calendar workload tracking is configured
- **WHEN** `calendar.id`, `calendar.event_names`, `calendar.monthly_hour_budget`, and required read-only calendar access settings are present in `~/.toggl.yaml`
- **THEN** `toggl start` and `toggl stop` can read matching Google Calendar events for workload status

#### Scenario: Calendar workload tracking is omitted
- **WHEN** the `calendar` configuration section is absent
- **THEN** `toggl start` and `toggl stop` continue without calendar workload tracking

#### Scenario: Day and month boundaries are needed
- **WHEN** calendar workload tracking calculates today or the current month
- **THEN** the CLI uses the system local timezone for day and month boundaries
