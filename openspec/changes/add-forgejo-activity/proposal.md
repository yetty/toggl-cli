## Why

Toggl summaries are built only from local `git log` output. As more work happens on the
Senseloom Forgejo server (pushed from other machines, done through pull requests, or
tracked as issues), local clones frequently contain no matching commits and the generated
description is empty or vague.

## What Changes

- Add an optional `forgejo` configuration section (server URL, API token, optional explicit repository list).
- Collect the user's Forgejo commits, pull requests, and issues for the stopped session window.
- Discover Forgejo repositories from the Forgejo remotes of already-configured local repository paths.
- Include Forgejo activity in the prompts used by `stop`, `fill-empty-descriptions`, and `repair-summaries`.
- Feed project-mapped Forgejo activity into the existing project time-splitting workflow.

## Capabilities

### New Capabilities

- `forgejo-activity`: collect the user's Forgejo commits, pull requests, and issues for a time window.

### Modified Capabilities

- `git-commit-collection`: additionally source activity from Forgejo when configured.
- `ai-summary-generation`: include Forgejo activity sections in the summary prompt.
- `cli-configuration`: add the optional `forgejo` section and `FORGEJO_API_KEY` override.

## Non-goals

- Do not remove or replace local `git log` collection.
- Do not add new commands or interactive prompts.
- Do not fail any command when Forgejo is unreachable or misconfigured.

## Impact

- Affects `stop`, `fill-empty-descriptions`, and `repair-summaries` prompt construction.
- Adds outbound requests to the configured Forgejo server when enabled.
- Preserves behavior exactly when `forgejo` is not configured.
