# Forgejo activity design

## Context

Toggl summaries are built only from local `git log` output, so work pushed from other
machines or done through pull requests and issues is invisible when the local clone has no
matching commits.

## Decisions

- Reuse the existing local repository configuration: Forgejo repositories are derived from
  `git remote get-url origin` for paths in `repositories` and `projects.<name>.repositories`,
  plus an optional explicit `forgejo.repositories` list for repositories without a clone.
- Filter commits client-side by committer date and `git.user` author tokens, because the
  Forgejo commits endpoint exposes no date or author filter in the target server version.
- Use `GET /repos/issues/search` with `created`, `assigned`, `review_requested`, and
  `reviewed`; `since`/`before` filter by updated time, which is accepted.
- Keep two disjoint activity views: project-mapped activity feeds time splitting, while
  formatted sections feed non-split prompts. Within a single prompt path an item appears
  once; the non-split prompt may still repeat an identical local and Forgejo commit line.
- Merge local and Forgejo commits for splitting, de-duplicated by project, time, and subject.
- Treat all Forgejo failures as warnings so summary generation always continues.

## Compatibility

Absent `forgejo:` configuration leaves existing behavior unchanged. The `FORGEJO_API_KEY`
environment variable overrides `forgejo.api_key`, mirroring the existing `OPENAI_API_KEY`
handling.
