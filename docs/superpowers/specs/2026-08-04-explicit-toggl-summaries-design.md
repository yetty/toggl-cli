# Explicit Toggl Summaries Design

## Goal

Make generated Toggl descriptions identify the day's most important concrete outcomes rather than abstract process work.

## Current problem

The OpenAI system prompt requires one sentence of at most 140 characters and forbids commit hashes. This encourages vague summaries such as "Implemented various OpenSpec changes" even when the commits identify specific work items.

For example, the August 3 session included commits for:

- `#3917` operational dashboard GraphQL data
- `#3916` trial-source-agent seeding
- `#3898` compliance-stat ordering
- `#3871` managed Keycloak user sync

The saved summary instead described "various OpenSpec changes" and the Keycloak update generally.

## Design

Keep the existing one-call summary flow. Change only the prompt contract used by `openAISummarize`.

The model must:

1. Infer the one to three highest-impact outcomes from commits and work-log entries.
2. State each selected outcome using a concrete action and affected capability, feature, or defect.
3. Retain an issue identifier only when it identifies a selected high-impact outcome. It must not list every issue or pull request.
4. Prefer substantive implementation work over process noise. Ignore OpenSpec proposals and archives, merge commits, dependency updates, localisation churn, and configuration-only changes unless no substantive work is present.
5. Return one readable line of at most 280 characters, with concise clauses separated by semicolons where useful.
6. Avoid vague phrases such as "OpenSpec changes," "various updates," "implemented improvements," and "merged PRs" unless unavoidable.

## Example

For the August 3 session, an acceptable description is:

> `#3917 add operational dashboard GraphQL data; #3916 seed trial-source agents; enable managed Keycloak user sync (#3871).`

## Error handling

No new error paths are introduced. Existing API-failure and manual-description fallback behavior remains unchanged.

## Testing

Add a test that captures the OpenAI request body and verifies the prompt includes the concrete-selection, selective-ticket, and process-noise rules. Existing end-to-end command tests continue to verify that the returned description is saved unchanged.
