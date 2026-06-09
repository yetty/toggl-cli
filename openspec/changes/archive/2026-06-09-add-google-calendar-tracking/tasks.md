## 1. Configuration and API Setup

- [x] 1.1 Investigate the simplest non-OAuth read-only Google Calendar access option for a shareable calendar, comparing at least shareable calendar/feed access and service-account access.
- [x] 1.2 Add calendar workload fields to the config model and preserve behavior when the `calendar` section is absent.
- [x] 1.3 Add Google Calendar read-only client setup using configured calendar ID, exact event names, fixed monthly budget, and the selected access settings.
- [x] 1.4 Add documentation for the new `calendar` YAML section using placeholder values only.

## 2. Calendar and Workload Calculations

- [x] 2.1 Implement current-month and current-day boundary helpers using the system local timezone.
- [x] 2.2 Fetch matching timed Google Calendar events for the current month and exclude non-matching or all-day events.
- [x] 2.3 Fetch current-month stopped Toggl entries for listed project IDs and sum actual worked duration.
- [x] 2.4 Implement workload status calculations for worked hours, planned hours, remaining required hours, future planned hours after today, and today's recommended work excluding today's events and the current timer.

## 3. CLI Integration

- [x] 3.1 Extend `toggl start` to print the existing start confirmation and then a concise recommended-work-today hint when calendar tracking is configured.
- [x] 3.2 Extend `toggl stop` to preserve existing stop, summary, worklog, and project-splitting behavior while adding a styled CLI block calendar workload overview when configured.
- [x] 3.3 Ensure configured calendar tracking failures are reported visibly without changing behavior for users who do not configure calendar tracking.

## 4. Verification

- [x] 4.1 Add focused unit tests for timezone/month boundaries, event filtering, and workload recommendation calculations.
- [x] 4.2 Add command-level tests or injectable-client tests covering `start` and `stop` output with and without calendar tracking configured.
- [x] 4.3 Run `make fmt` and `make test`.
