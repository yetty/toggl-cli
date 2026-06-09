# worklog-notes Specification

## Purpose
TBD - created by archiving change document-current-cli-functionality. Update Purpose after archive.
## Requirements
### Requirement: Append worklog notes
The CLI SHALL provide a `log [MESSAGE]` command that appends timestamped work notes to a temporary worklog file.

#### Scenario: User logs one message
- **WHEN** the user runs `toggl log "test message"`
- **THEN** the CLI appends a line containing an RFC3339 timestamp in square brackets and the message to `${TMPDIR}/toggl-worklog.txt`
- **THEN** the CLI prints `Logged: test message`

#### Scenario: User logs multiple messages
- **WHEN** the user runs `toggl log` more than once
- **THEN** each message is appended as a separate line without removing earlier entries

#### Scenario: User omits message argument
- **WHEN** the user runs `toggl log` without a message
- **THEN** the command returns an argument validation error

### Requirement: Include worklog notes in stop summaries
The stop workflow SHALL include non-empty worklog contents in the prompt text used to derive the stopped-entry description for non-split behavior.

#### Scenario: Worklog exists and has content
- **WHEN** `toggl stop` reads `${TMPDIR}/toggl-worklog.txt` and the file contains non-whitespace text
- **THEN** the prompt text includes a `Work log:` section with the trimmed worklog content

#### Scenario: Worklog does not exist
- **WHEN** `toggl stop` attempts to read the temporary worklog and the file is absent
- **THEN** the CLI treats the worklog as empty

### Requirement: Clean worklog after completed stop flow
The CLI SHALL remove the temporary worklog after a stop flow completes without a final Toggl persistence error.

#### Scenario: Summary is saved
- **WHEN** `toggl stop` successfully saves a description or successfully applies project splits
- **THEN** the CLI removes `${TMPDIR}/toggl-worklog.txt`

#### Scenario: User skips description
- **WHEN** `toggl stop` stops the timer but receives an empty manual description
- **THEN** the CLI removes `${TMPDIR}/toggl-worklog.txt`

#### Scenario: Project split summarization is skipped
- **WHEN** project split summarization cannot produce a description and the split flow stops without saving summaries
- **THEN** the CLI removes `${TMPDIR}/toggl-worklog.txt`

