## Why

Stopped Toggl entries sometimes remain empty because `toggl stop` had no usable commits/worklog, failed during summary generation, or the user skipped the manual prompt. A dedicated review-and-fill workflow would let the user recover those empty descriptions later without manually inspecting Toggl entry by entry.

## What Changes

- Add a CLI command that scans Toggl time entries in a user-specified date range, defaulting to the last 7 days.
- Identify entries whose descriptions are empty or whitespace-only.
- For each empty entry, propose a description using the same commit collection and OpenAI/manual fallback behavior used by `toggl stop` for an entry's time window.
- Ask for user confirmation before saving any proposed description to Toggl.
- Report scanned, proposed, updated, and skipped counts when the workflow completes.

## Non-goals

- Do not modify entries that already have non-empty descriptions.
- Do not automatically update entries without an explicit user confirmation.
- Do not infer descriptions from unrelated Toggl entries beyond the configured repository/git and summary workflow already used for stopped records.
- Do not change the existing `toggl stop` behavior or the existing `repair-summaries` placeholder-repair workflow.

## Capabilities

### New Capabilities
- `empty-description-backfill`: Covers scanning historical Toggl entries, detecting empty descriptions, proposing descriptions, confirming updates, and reporting results.

### Modified Capabilities

## Impact

- Affected CLI surface: new subcommand under the `toggl` root command.
- Affected integrations: Toggl API reads for historical entries, Toggl API updates for confirmed entries, local git commit inspection, and OpenAI summary generation.
- Affected code: command wiring, date-range parsing/defaults, reusable description-generation helper code currently exercised by `toggl stop`, and tests for the new workflow.
