## ADDED Requirements

### Requirement: Support Forgejo configuration fields and environment override
The CLI SHALL support an optional `forgejo` configuration section containing `url`, `api_key`, and an optional `repositories` list, and SHALL let the `FORGEJO_API_KEY` environment variable override the configured Forgejo API key when present.

#### Scenario: Environment Forgejo key is present
- **WHEN** `FORGEJO_API_KEY` is set before the CLI loads configuration
- **THEN** the CLI uses the environment value as the Forgejo API key instead of `forgejo.api_key` from `~/.toggl.yaml`

#### Scenario: Environment Forgejo key is absent
- **WHEN** `FORGEJO_API_KEY` is not set before the CLI loads configuration
- **THEN** the CLI uses `forgejo.api_key` from `~/.toggl.yaml`

## MODIFIED Requirements

### Requirement: Load user configuration file
The CLI SHALL load configuration from a file named `.toggl.yaml` in the current user's home directory before executing commands.

#### Scenario: Valid configuration is available
- **WHEN** the user runs the CLI and `~/.toggl.yaml` exists with valid YAML
- **THEN** the CLI loads Toggl, OpenAI, git, repository, project, optional calendar workload, and optional forgejo configuration fields from that file

#### Scenario: Configuration file cannot be read
- **WHEN** the user runs the CLI and `~/.toggl.yaml` cannot be read
- **THEN** the CLI prints `Error loading config:` followed by the error and exits with a non-zero status

#### Scenario: Configuration YAML is invalid
- **WHEN** the user runs the CLI and `~/.toggl.yaml` contains invalid YAML
- **THEN** the CLI prints `Error loading config:` followed by the parse error and exits with a non-zero status
