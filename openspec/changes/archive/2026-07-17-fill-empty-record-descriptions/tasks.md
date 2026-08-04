## 1. Command and Range Handling

- [x] 1.1 Add a `fill-empty-descriptions` Cobra command and register it on the root command.
- [x] 1.2 Add optional `--start` and `--end` flags; default both omitted to `nowFunc().AddDate(0, 0, -7)` through `nowFunc()` in RFC3339 format.
- [x] 1.3 Return an error containing `--start and --end must be provided together` when exactly one range flag is provided.

## 2. Backfill Workflow

- [x] 2.1 Add helper logic to classify fillable entries as stopped entries with empty or whitespace-only descriptions.
- [x] 2.2 Fetch Toggl entries for the computed or explicit range using the existing `listTimeEntries` helper.
- [x] 2.3 For each fillable entry, collect legacy repository commits for that entry window, build prompt text without reading the worklog file, and call existing description-generation behavior.
- [x] 2.4 Print the entry identity/time window and proposed description before asking for confirmation.
- [x] 2.5 Update the Toggl entry only when the user confirms; otherwise leave it unchanged and count it as skipped.
- [x] 2.6 Print `Skipped entry <id>.` when no non-empty proposed description can be obtained.
- [x] 2.7 Print final scanned, blank, proposed, updated, and skipped counts, including a clear message when no blank descriptions were found.

## 3. Tests and Documentation

- [x] 3.1 Add targeted tests for default date range computation, explicit range handling, and one-sided range validation.
- [x] 3.2 Add targeted tests for blank-description filtering, invalid time-window skipping, confirmation updates, and rejection/no-proposal skipping.
- [x] 3.3 Update README command list/features with `fill-empty-descriptions` usage and the 7-day default.
- [x] 3.4 Run `make fmt` and `make test`.
