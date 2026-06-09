# cli-configuration Specification

## Purpose
TBD - created by archiving change document-current-cli-functionality. Update Purpose after archive.
## Requirements
### Requirement: Load user configuration file
The CLI SHALL load configuration from a file named `.toggl.yaml` in the current user's home directory before executing commands.

#### Scenario: Valid configuration is available
- **WHEN** the user runs the CLI and `~/.toggl.yaml` exists with valid YAML
- **THEN** the CLI loads Toggl, OpenAI, git, repository, and project configuration fields from that file

#### Scenario: Configuration file cannot be read
- **WHEN** the user runs the CLI and `~/.toggl.yaml` cannot be read
- **THEN** the CLI prints `Error loading config:` followed by the error and exits with a non-zero status

#### Scenario: Configuration YAML is invalid
- **WHEN** the user runs the CLI and `~/.toggl.yaml` contains invalid YAML
- **THEN** the CLI prints `Error loading config:` followed by the parse error and exits with a non-zero status

### Requirement: Support Toggl configuration fields
The CLI SHALL support Toggl configuration containing `api_key`, `workspace_id`, and default `project_id` fields.

#### Scenario: Default Toggl project is configured
- **WHEN** the user runs `toggl start`
- **THEN** the CLI uses `toggl.workspace_id` and `toggl.project_id` from configuration to create the running time entry

### Requirement: Support OpenAI configuration fields and environment override
The CLI SHALL support OpenAI configuration containing `api_key` and `model`, and SHALL let the `OPENAI_API_KEY` environment variable override the configured OpenAI API key when present.

#### Scenario: Environment OpenAI key is present
- **WHEN** `OPENAI_API_KEY` is set before the CLI loads configuration
- **THEN** the CLI uses the environment value as the OpenAI API key instead of `openai.api_key` from `~/.toggl.yaml`

#### Scenario: Environment OpenAI key is absent
- **WHEN** `OPENAI_API_KEY` is not set before the CLI loads configuration
- **THEN** the CLI uses `openai.api_key` from `~/.toggl.yaml`

### Requirement: Support git and repository configuration
The CLI SHALL support `git.user`, a legacy top-level `repositories` list, and a `projects` map whose entries contain `project_id` and `repositories`.

#### Scenario: Legacy repositories are configured
- **WHEN** `repositories` contains repository paths
- **THEN** the stop and repair workflows can collect matching commits from those repositories for summary-only behavior

#### Scenario: Project repositories are configured
- **WHEN** `projects` contains one or more named projects with repository paths
- **THEN** the stop workflow can collect commits with project metadata and use them for project-based splitting

### Requirement: Expand home-relative repository paths
The CLI SHALL expand repository paths equal to `~` or beginning with `~/` to the current user's home directory before invoking git.

#### Scenario: Repository path begins with tilde slash
- **WHEN** a configured repository path is `~/Projects/example`
- **THEN** the CLI resolves it to `<home>/Projects/example` before invoking git

#### Scenario: Repository path does not use home shorthand
- **WHEN** a configured repository path does not equal `~` and does not begin with `~/`
- **THEN** the CLI uses the path unchanged

