## ADDED Requirements

### Requirement: Expose track command
The CLI SHALL register a `track` subcommand under the `toggl` root command for on-demand workload status checks.

#### Scenario: Root command includes track
- **WHEN** the CLI starts successfully
- **THEN** it registers `track` as a supported subcommand alongside the existing time-tracking and metadata commands

#### Scenario: Track command has descriptive help
- **WHEN** the user lists command help
- **THEN** the `track` command description indicates that it reports calendar workload or budget tracking status
