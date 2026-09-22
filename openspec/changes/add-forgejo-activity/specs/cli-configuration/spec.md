## ADDED Requirements

### Requirement: Support Forgejo configuration fields and environment override
The CLI SHALL support an optional `forgejo` configuration section containing `url`, `api_key`, and an optional `repositories` list, and SHALL let the `FORGEJO_API_KEY` environment variable override the configured Forgejo API key when present.

#### Scenario: Forgejo integration is configured
- **WHEN** `forgejo.url` and `forgejo.api_key` are present in `~/.toggl.yaml`
- **THEN** `stop`, `fill-empty-descriptions`, and `repair-summaries` can include Forgejo activity in summaries

#### Scenario: Forgejo integration is omitted
- **WHEN** the `forgejo` configuration section is absent or missing `url` or `api_key`
- **THEN** the CLI makes no Forgejo requests and summary behavior is unchanged

#### Scenario: Environment Forgejo key is present
- **WHEN** `FORGEJO_API_KEY` is set before the CLI loads configuration
- **THEN** the CLI uses the environment value as the Forgejo API key instead of `forgejo.api_key` from `~/.toggl.yaml`
