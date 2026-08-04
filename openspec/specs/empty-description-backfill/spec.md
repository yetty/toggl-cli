# empty-description-backfill Specification

## Purpose
Describe the behavior of the `fill-empty-descriptions` workflow for proposing and optionally saving descriptions for stopped Toggl entries with blank descriptions.

## Requirements
### Requirement: Provide empty-description backfill command
The CLI SHALL provide a `fill-empty-descriptions` command that scans Toggl entries for empty descriptions in a date range.

#### Scenario: Root command includes fill-empty-descriptions
- **WHEN** the CLI starts successfully
- **THEN** it registers `fill-empty-descriptions` as a supported subcommand under the `toggl` root command

#### Scenario: Command help describes blank description backfill
- **WHEN** the user lists command help
- **THEN** the `fill-empty-descriptions` command description indicates that it finds empty Toggl descriptions and proposes replacements

### Requirement: Default scan range to the last seven days
The backfill workflow SHALL scan entries from the last 7 days through the current time when no date range flags are provided.

#### Scenario: No range flags are provided
- **WHEN** the user runs `toggl fill-empty-descriptions` without `--start` or `--end`
- **THEN** the CLI sends a `GET` request to `me/time_entries` with `start_date` equal to the current time minus 7 days and `end_date` equal to the current time

#### Scenario: Explicit range flags are provided
- **WHEN** the user runs `toggl fill-empty-descriptions --start <start> --end <end>`
- **THEN** the CLI sends a `GET` request to `me/time_entries` with `start_date` and `end_date` query parameters containing the provided values

#### Scenario: Only one explicit range flag is provided
- **WHEN** the user runs `toggl fill-empty-descriptions` with only one of `--start` or `--end`
- **THEN** the command returns an error stating that `--start and --end must be provided together`

### Requirement: Identify only stopped entries with blank descriptions
The backfill workflow SHALL consider an entry fillable only when it has a closed time window and its description is empty or whitespace-only.

#### Scenario: Entry description is empty
- **WHEN** a listed entry has an empty description and a stop time after its start time
- **THEN** the workflow attempts to propose a description for that entry

#### Scenario: Entry description is whitespace-only
- **WHEN** a listed entry description contains only whitespace and the entry has a stop time after its start time
- **THEN** the workflow attempts to propose a description for that entry

#### Scenario: Entry already has a description
- **WHEN** a listed entry description contains non-whitespace text
- **THEN** the workflow leaves that entry unchanged

#### Scenario: Entry has no closed time window
- **WHEN** a listed entry has no stop time or its stop time is not after its start time
- **THEN** the workflow skips that entry without attempting to generate or save a description

### Requirement: Propose descriptions using stop-style summary generation
For each fillable entry, the backfill workflow SHALL collect configured legacy repository commits in that entry's time window and obtain a proposed description using the same summary/manual fallback behavior as the non-split `toggl stop` path, without reading or deleting the temporary worklog file.

#### Scenario: Proposal is generated from commits
- **WHEN** a fillable entry has collected commits and OpenAI returns a non-empty summary
- **THEN** the workflow presents that summary as the proposed description for the entry

#### Scenario: Manual proposal is provided
- **WHEN** a fillable entry has no prompt text or OpenAI summarization fails and the user provides manual text
- **THEN** the workflow presents the manual text as the proposed description for the entry

#### Scenario: Proposal is skipped
- **WHEN** a fillable entry cannot obtain a non-empty generated or manual description
- **THEN** the workflow prints `Skipped entry <id>.` and leaves that entry unchanged

### Requirement: Confirm before updating each entry
The backfill workflow SHALL ask for confirmation before saving each proposed description to Toggl.

#### Scenario: User confirms proposed description
- **WHEN** the workflow has a proposed description for an entry and the user confirms it
- **THEN** the CLI updates that entry description through the Toggl API
- **THEN** the workflow counts the entry as updated

#### Scenario: User rejects proposed description
- **WHEN** the workflow has a proposed description for an entry and the user does not confirm it
- **THEN** the workflow leaves that entry unchanged
- **THEN** the workflow counts the entry as skipped

### Requirement: Report backfill results
The backfill workflow SHALL print a completion summary with scan and outcome counts.

#### Scenario: Backfill command completes
- **WHEN** the workflow finishes scanning all returned entries
- **THEN** the CLI prints counts for scanned entries, entries with blank descriptions, proposed descriptions, updated entries, and skipped entries

#### Scenario: No blank entries are found
- **WHEN** the workflow finds no fillable blank-description entries
- **THEN** the CLI prints a message indicating that no empty descriptions were found
