## 1. Command Implementation

- [x] 1.1 Add a `trackCmd()` Cobra command that errors when calendar workload tracking is not configured.
- [x] 1.2 In `trackCmd()`, load workload status using existing calendar/Toggl read helpers and print the styled calendar overview.
- [x] 1.3 Register `trackCmd()` on the root command without changing existing subcommand behavior.

## 2. Documentation

- [x] 2.1 Update README feature and command lists to include `toggl track`.
- [x] 2.2 Update the calendar configuration description to mention on-demand workload status checks.

## 3. Verification

- [x] 3.1 Add or update targeted tests for `toggl track` configured, unconfigured, error, and read-only behavior where the current test structure supports command testing.
- [x] 3.2 Run `make fmt` and `make test` to verify formatting and behavior.
