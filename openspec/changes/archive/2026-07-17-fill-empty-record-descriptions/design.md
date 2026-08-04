## Context

The CLI currently fills a just-stopped entry in `toggl stop` by collecting commits for that entry's start/stop window, building summary prompt text, optionally calling OpenAI, falling back to manual entry, and saving the resulting description. A separate `repair-summaries` command exists for entries with known bad placeholder descriptions, but it requires explicit range flags and only targets exact placeholder text.

This change adds a historical review workflow for entries that have no description at all. The implementation should reuse the same description-generation primitives used by `toggl stop` while avoiding changes to existing stop, split, worklog cleanup, or placeholder repair behavior.

## Goals / Non-Goals

**Goals:**
- Add a user-invoked command that scans a date range, defaulting to the last 7 days.
- Detect stopped entries with blank descriptions and skip entries that already have text.
- Generate one proposed description per blank entry from commits in that entry's time window using the current stop-style prompt/summary/manual fallback path.
- Require explicit confirmation before updating Toggl.
- Keep partial success predictable: one failed update should return an error after preserving already-applied updates.

**Non-Goals:**
- Do not split historical entries into project-specific entries.
- Do not consume or delete the current temporary worklog file while backfilling historical entries.
- Do not introduce new configuration fields or dependencies.
- Do not change `repair-summaries` required flags or known-placeholder matching.

## Decisions

1. **Add a separate `fill-empty-descriptions` command rather than extending `repair-summaries`.**
   - Rationale: blank-description backfill has different defaults, confirmation semantics, and matching criteria from placeholder repair. A separate command avoids changing existing repair behavior.
   - Alternative considered: add a `--empty` mode to `repair-summaries`; rejected because it would mix two workflows and complicate help/output expectations.

2. **Use `--start` and `--end` as optional RFC3339 range flags with a 7-day default.**
   - Rationale: this matches existing Toggl listing helper expectations while satisfying the default lookback requirement. When omitted, the command can compute `[now - 7 days, now]` using `nowFunc()` for testability.
   - Alternative considered: a `--days` flag only; rejected because explicit ranges are still needed for precise historical repair.

3. **Generate proposals from legacy repository commits only, not worklog or project splitting.**
   - Rationale: historical entries do not have a reliable per-entry temporary worklog, and backfill should not mutate entry time/project structure. Reusing `collectCommits`, `buildPromptText`, `getDescription`, and `updateEntryDescription` mirrors the non-split `toggl stop` summary path without side effects.
   - Alternative considered: use project commit collection and split logic; rejected because the requested workflow is to fill descriptions, not rewrite historical records.

4. **Confirm per entry after showing the proposed description.**
   - Rationale: each historical entry may have different quality evidence, and per-entry confirmation prevents a poor proposal from bulk-editing many Toggl records. Simple yes/no input keeps the command interactive without introducing a dry-run state machine.
   - Alternative considered: one final bulk confirmation; rejected because the user would need to remember and evaluate multiple proposals at once.

## Risks / Trade-offs

- **Historical commits may not fully explain an entry** → The command shows the proposal and requires confirmation before writing.
- **OpenAI/manual fallback prompts can become repetitive across many blank entries** → The default 7-day range limits scope, and users can narrow with explicit date flags.
- **Some entries have zero stop times or invalid time windows** → The command should skip entries that cannot define a closed time window and count them as skipped.
- **Update failure after earlier confirmations creates partial success** → Return the error immediately; already-confirmed updates remain in Toggl and the final output before the error should identify progress where practical.
