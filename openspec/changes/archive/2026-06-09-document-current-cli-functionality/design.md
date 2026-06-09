## Context

The repository contains a single-file Go/Cobra CLI in `src/main.go` with tests in `src/main_test.go`. Runtime behavior is currently concentrated in one `main` package:

```
toggl command
    │
    ├─ loadConfig() ── ~/.toggl.yaml + OPENAI_API_KEY override
    │
    ├─ Toggl API v9 ── start/stop/update/list/me/projects
    │
    ├─ local /tmp/toggl-worklog.txt
    │
    ├─ git log --all over configured repositories
    │
    └─ OpenAI chat completions for final descriptions
```

This change does not redesign that architecture. It creates a baseline set of OpenSpec capabilities that describe the current contract so later behavior changes can be proposed against a known reference.

## Goals / Non-Goals

**Goals:**

- Describe all current CLI commands and helper workflows as testable requirements.
- Preserve distinctions between legacy repository summarization and project-configured splitting.
- Capture current fallbacks, cleanup behavior, warning behavior, and user-visible output.
- Make the specs suitable as an archiveable baseline for future changes.

**Non-Goals:**

- No Go code changes.
- No API compatibility improvements or error-handling changes.
- No refactor of the single-file architecture.
- No new commands, flags, configuration keys, storage locations, or external dependencies.

## Decisions

- **Use multiple focused capabilities rather than one monolithic CLI spec.**
  - Rationale: the current CLI spans configuration, Toggl API usage, git, OpenAI, and local file state. Splitting these into capability specs makes future deltas smaller and easier to review.
  - Alternative considered: one `toggl-cli` spec. This would be simpler initially but would make future changes noisy because unrelated command and integration requirements would live together.

- **Specify existing behavior, including rough edges.**
  - Rationale: this is a baseline, not a cleanup. Specs call out the current temp worklog path, manual fallback behavior, project splitting approximation, and repair of two known placeholder descriptions.
  - Alternative considered: specify idealized behavior. That would blur the line between documentation and product change.

- **Describe behavior from observable boundaries.**
  - Rationale: requirements focus on CLI inputs/outputs, config, API requests, local files, and git/OpenAI interactions rather than private function names.
  - Alternative considered: mirror implementation helpers one-for-one. That would overfit the spec to refactoring details.

- **Keep tasks documentation-oriented.**
  - Rationale: implementation is effectively complete; tasks should verify and archive the baseline, not alter runtime behavior.

## Risks / Trade-offs

- **Risk: Baseline specs accidentally imply desired future behavior rather than current behavior.** → Mitigation: requirements were grounded in `README.md`, `src/main.go`, and existing tests, and avoid proposing fixes.
- **Risk: Eight capabilities may feel granular for a small CLI.** → Mitigation: each capability maps to a separable integration or command family, supporting focused future deltas.
- **Risk: Some current behavior is not fully tested or has implicit edge cases.** → Mitigation: specs describe the behavior visible in code/tests and leave no code implementation in this change.
- **Risk: Project splitting is approximate by design.** → Mitigation: the spec documents the midpoint allocation and contiguous-entry application model explicitly.
