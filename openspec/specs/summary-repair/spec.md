# summary-repair Specification

## Purpose
TBD - created by archiving change document-current-cli-functionality. Update Purpose after archive.
## Requirements
### Requirement: Require repair date range flags
The CLI SHALL provide a `repair-summaries` command that requires `--start` and `--end` flags.

#### Scenario: Start or end flag is missing
- **WHEN** the user runs `toggl repair-summaries` without either required date range flag
- **THEN** the command returns an error stating `--start and --end are required`

### Requirement: Fetch entries in the repair date range
The repair workflow SHALL list Toggl time entries between the provided start and end date strings.

#### Scenario: Repair range is provided
- **WHEN** the user runs `toggl repair-summaries --start <start> --end <end>`
- **THEN** the CLI sends a `GET` request to `me/time_entries` with `start_date` and `end_date` query parameters containing the provided values

### Requirement: Identify known bad placeholder summaries
The repair workflow SHALL consider entries repairable only when their descriptions match known bad OpenAI placeholder responses.

#### Scenario: Entry has primary known bad summary
- **WHEN** a listed entry description is `Sure! Please provide the details of your commits so I can generate a concise summary for you.`
- **THEN** the repair workflow attempts to repair that entry

#### Scenario: Entry has alternate known bad summary
- **WHEN** a listed entry description is `Sure! Please provide the details of your commits so I can generate a summary for your Toggl time entry.`
- **THEN** the repair workflow attempts to repair that entry

#### Scenario: Entry does not have a known bad summary
- **WHEN** a listed entry description does not match a known bad placeholder response
- **THEN** the repair workflow leaves that entry unchanged

### Requirement: Repair bad summaries using collected commits or manual fallback
For each repairable entry, the CLI SHALL collect legacy repository commits in that entry's time window and save a new non-empty description when one is available.

#### Scenario: Repair summary is generated
- **WHEN** a repairable entry has collected commits and OpenAI returns a non-empty summary
- **THEN** the CLI updates that entry description with the generated summary

#### Scenario: Manual repair summary is provided
- **WHEN** a repairable entry has no prompt text or OpenAI summarization fails and the user provides manual text
- **THEN** the CLI updates that entry description with the manual text

#### Scenario: Repair description is skipped
- **WHEN** a repairable entry cannot obtain a non-empty generated or manual description
- **THEN** the CLI prints `Skipped entry <id>.` and leaves that entry unchanged

### Requirement: Report repair count
The repair workflow SHALL print the number of entries successfully repaired.

#### Scenario: Repair command completes
- **WHEN** the repair workflow finishes scanning all returned entries
- **THEN** the CLI prints `Repaired <count> entries.`

