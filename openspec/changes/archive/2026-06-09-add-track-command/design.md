## Context

Calendar workload tracking already has reusable helpers for loading current-month Google Calendar events, loading Toggl worked time for configured project IDs, calculating workload status, and printing either a brief start hint or styled stop overview. Today those helpers are only reached from `start` and `stop`, so checking budget status requires mutating Toggl state.

The implementation should remain small in the single-file Cobra CLI and preserve existing configuration semantics.

## Goals / Non-Goals

**Goals:**
- Add `toggl track` as a read-only command for on-demand calendar workload reporting.
- Reuse existing calendar workload configuration, API clients, calculations, and overview formatting.
- Make missing calendar workload configuration visible for `track`, because the command has no useful non-calendar behavior.
- Avoid changes to `start`, `stop`, worklog cleanup, OpenAI summaries, and project split behavior.

**Non-Goals:**
- No new configuration keys, calendar authentication flows, or Toggl write operations.
- No change to how budget, worked time, future planned work, or recommended work for today are calculated.
- No attempt to include currently running timer duration unless the existing workload calculation is changed by a separate proposal.

## Decisions

1. **Implement `track` as a dedicated Cobra command that only reads workload status.**
   - Rationale: Keeps the user intent clear and avoids overloading `start` or `stop` with flags.
   - Alternative considered: Add a `--status` flag to existing commands. This would be less discoverable and risks mixing read-only and mutating workflows.

2. **Reuse `loadWorkloadStatus` and `printCalendarOverview`.**
   - Rationale: The user asked for output similar to current calendar-related `start`/`stop` output; reusing the stop overview gives the most complete existing view with minimal risk.
   - Alternative considered: Print only the `start` command's one-line hint. That is less useful for standalone status because it omits worked, planned, and remaining details.

3. **Return an error when calendar workload tracking is not configured.**
   - Rationale: `start` and `stop` have useful primary behavior without calendar settings, but `track` does not. A visible configuration error helps users fix setup instead of seeing no output.
   - Alternative considered: Silently do nothing like `start`/`stop` do when the optional calendar feature is disabled. That would make the new command confusing.

4. **Do not write to Toggl, OpenAI, git, or the worklog file.**
   - Rationale: The command is intended as a safe status check. It should only perform the reads already required by calendar workload calculations.
   - Alternative considered: Updating a Toggl description or logging a status note was excluded because it would make `track` a mutating workflow.

## Risks / Trade-offs

- **External API failures still affect the command** → Surface the same calendar workload errors currently used by start/stop instead of retrying or hiding them.
- **Read-only status excludes currently running timer time** → Keep parity with existing calculation semantics and document that a formula change is out of scope.
- **A full overview may be more verbose than the start hint** → Prefer completeness for a standalone command; users can still use `start` for the brief hint.
