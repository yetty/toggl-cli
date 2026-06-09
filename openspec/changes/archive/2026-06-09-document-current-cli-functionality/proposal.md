## Why

The CLI has meaningful behavior across Toggl API calls, local work logs, git commit collection, OpenAI summarization, and project-based time-entry splitting, but OpenSpec currently has no capability specs that describe this existing contract. Capturing the current behavior as specs creates a baseline for future changes and makes regressions easier to reason about before implementation work begins.

## What Changes

- Add baseline OpenSpec capabilities describing the CLI's current externally visible behavior.
- Cover configuration loading, command behavior, Toggl API interactions, git commit collection, work-log handling, OpenAI summary generation, project-based splitting, and repair of known bad summaries.
- Treat this as documentation/specification of existing behavior, not an application-code change.
- No **BREAKING** behavior changes are proposed.

## Capabilities

### New Capabilities
- `cli-configuration`: Loading `~/.toggl.yaml`, supported configuration fields, environment override behavior, and path expansion semantics.
- `time-tracking-commands`: Starting and stopping Toggl time entries, updating descriptions, and user-facing command outcomes.
- `worklog-notes`: Capturing local session notes with `toggl log` and consuming/cleaning the temporary worklog during stop.
- `git-commit-collection`: Collecting git commit subjects from configured repositories and project repositories for a stopped session window.
- `ai-summary-generation`: Generating one-line Toggl descriptions with OpenAI and falling back to manual descriptions when needed.
- `project-time-splitting`: Splitting a stopped time entry into project-specific entries based on configured projects and commit timing.
- `toggl-metadata-commands`: Showing current Toggl user information and listing workspace projects.
- `summary-repair`: Finding entries with known bad AI placeholder summaries and repairing their descriptions.

### Modified Capabilities
- None.

## Impact

- OpenSpec artifacts under `openspec/changes/document-current-cli-functionality/`.
- Source behavior analyzed from `src/main.go`, `src/main_test.go`, and `README.md`.
- No Go source files, runtime APIs, dependencies, or CLI behavior are changed by this proposal.
