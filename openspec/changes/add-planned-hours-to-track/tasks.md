## 1. Workload Calculation

- [x] 1.1 Add a helper for next local calendar-month bounds using the existing time-boundary conventions.
- [x] 1.2 Add logic to fetch and sum matching next-month calendar events for `toggl track` only.
- [x] 1.3 Compare next-month planned duration with `calendar.monthly_hour_budget` and produce an explicit meets/below status.

## 2. Track Output

- [x] 2.1 Extend the `toggl track` overview to print one next-month planned-work line with hours and expected-budget status.
- [x] 2.2 Preserve existing current-month output lines and keep `start`/`stop` calendar output unchanged.
- [x] 2.3 Ensure calendar request failures from the added next-month read are reported through the existing `track` error behavior.

## 3. Verification and Documentation

- [x] 3.1 Add or update tests covering next-month planned hours that meet the configured budget.
- [x] 3.2 Add or update tests covering next-month planned hours below the configured budget and month-boundary handling.
- [x] 3.3 Update README command/configuration documentation to mention the next-month planned-work line in `toggl track`.
- [x] 3.4 Run `make fmt` and `make test`.
