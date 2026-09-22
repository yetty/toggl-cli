# Forgejo Activity in Toggl Summaries Design

Status: Approved

## Goal

Include the user's activity on the Senseloom Forgejo server (`https://infra.senseloom.com/`) in Toggl work summaries, so `toggl stop` produces useful descriptions even when few commits are made in local clones.

## Current problem

`toggl stop` (and `fill-empty-descriptions` / `repair-summaries`) build the OpenAI prompt solely from `git log --all --author=<git.user>` in locally configured repository paths. When work happens on Forgejo — pushed from other machines, done through pull requests, tracked as issues, or authored by automation — the local clone has no matching commits and the generated summary is empty or vague. The user now produces few local commits, so the summary is frequently missing real work.

Verified server surface: Forgejo `14.0.2~gitea-1.22.0`.

- `GET /repos/{owner}/{repo}/commits` exposes **no** date or author filter in that version's API description; results must be paged and filtered client-side.
- `GET /repos/issues/search` searches across repositories the token can access and supports `created`, `assigned`, `review_requested`, `reviewed`, `mentioned`, and `since` / `before` (which filter on **updated** time).
- Authentication uses `Authorization: token <api_key>`.

## Design

### Configuration

Add a `forgejo` section to `~/.toggl.yaml`:

```yaml
forgejo:
  url: https://infra.senseloom.com
  api_key: <token>            # FORGEJO_API_KEY env overrides
  repositories:               # optional, owner/repo, for repos without a local clone
    - voicesense/voicesense-backend
```

- Integration is enabled when `url` and `api_key` are both non-empty, mirroring `CalendarConfig.Enabled()`.
- `FORGEJO_API_KEY` overrides `api_key`, mirroring the `OPENAI_API_KEY` handling in `loadConfig`.
- No additional author configuration: the existing `git.user` value (split on `\|`) is reused for identity matching so `Juda Kaleta`, the email, and `Forgejo Actions` keep working.

### Repository resolution

Resolve the set of Forgejo repositories to scan from two sources, deduplicated by `owner/repo`:

1. Explicit `forgejo.repositories` entries (`owner/repo`) are always included.
2. For every local path in `repositories` and `projects.<name>.repositories`, run `git -C <path> remote get-url origin`. If the remote host matches the host of `forgejo.url`, derive `owner/repo` and include it. A repository discovered under `projects.<name>` inherits that project's `ProjectID` / name mapping; a repository discovered only under the legacy `repositories` list has no project mapping and participates in summary text only.

Each resolved repository therefore carries an optional `{ProjectName, ProjectID}` mapping. Explicit `forgejo.repositories` entries that match a local repository's Forgejo remote inherit that mapping; entries with no match have no mapping.

Identity matching uses the `git.user` tokens only; no additional identity lookup is performed.

### Activity collection

**Commits.** For each resolved repository:

- `GET /repos/{owner}/{repo}/commits?limit=50&page=N&stat=false&verification=false&files=false`.
- Page newest→oldest; stop once the oldest commit on a page is earlier than the entry start.
- Keep commits whose **committer** time (`commit.committer.date`, matching local `%cI` behavior) falls within `[entry.Start, entry.Stop]` and whose author matches `git.user` tokens case-insensitively against the author name or email, or whose `author.login` equals a token.
- Map each kept commit to the existing `ProjectCommit` shape (`RepoPath` empty, `RepoName` = Forgejo repository name, `ProjectName` / `ProjectID` from resolution; zero values when unmapped).

**Pull requests and issues.** One cross-repository search per run:

- `GET /repos/issues/search?state=all&since=<start>&before=<stop>&created=true&assigned=true&review_requested=true&reviewed=true&limit=50&page=N`. `mentioned` is deliberately omitted to avoid mention-only noise.
- Page until exhausted; filter results client-side to the resolved repository set so the allowlist is honored.
- Split results into pull requests (non-null `pull_request`) and issues.
- `since` / `before` filtering is on updated time, so an item merely touched in the window counts as activity. This is accepted.

### Collector shape

A single function produces both views needed by the callers:

```go
type ForgejoActivity struct {
    SplitCommits   []ProjectCommit // commits plus PR/issue pseudo-commits, mapped repositories only
    PromptSections []string        // "Forgejo commits", "Forgejo pull requests", "Forgejo issues"
}

func collectForgejoActivity(entry TogglTimeEntry) ForgejoActivity
```

- `SplitCommits` contains Forgejo commits **and** PR/issue pseudo-commits (`[PR #12] title` / `[Issue #45] title`, time = `updated_at`) for repositories that have a project mapping. It is consumed only by the split path.
- `PromptSections` contains formatted commit sections (all resolved repositories, mapped or not) plus separate pull-request and issue sections. It is consumed only by non-split callers.
- The two views are disjoint by design, so a PR/issue never appears twice in one prompt.
- A disabled or unconfigured integration returns an empty `ForgejoActivity` without making requests.

### Integration into `stop`

- Split path: append `activity.SplitCommits` to the locally collected project commits and pass the combined slice to `calculateProjectSplits` / `describeProjectSplits`. Per-project prompts then naturally include both sources. `stop` must filter the combined slice to commits with a non-zero `ProjectID` before splitting, so unmapped Forgejo repositories cannot create phantom `ProjectID 0` splits or trigger the empty-commit manual prompt in `describeProjectSplits`.
- Identical commits present both locally and on Forgejo are de-duplicated by `(project, time, subject)` before splitting. The non-split prompt may still repeat an identical commit line, which is accepted.
- Non-split path: build the prompt from local commits plus `activity.PromptSections` plus the worklog.
- `fill-empty-descriptions` and `repair-summaries`: build the prompt from local commits plus `activity.PromptSections`, so all three commands include Forgejo activity through the same helper.

### Failure handling

- Unconfigured Forgejo leaves behavior identical to today.
- Any Forgejo API, network, or authentication error produces a single stderr warning and processing continues with local data; `stop` never fails because of Forgejo.
- A local repository path that cannot be inspected as a git repository (missing path, no `origin`, not a git repo) is skipped silently during Forgejo repository resolution. Local commit collection already warns about the same path, so a second warning would be noise.
- A page cap (10 pages per repository and per search) bounds pathological repositories and warns when reached.
- A repository without a project mapping is excluded from splits by the explicit `ProjectID` filter and still contributes summary text through `PromptSections`.

## Example

With local clones stale but Forgejo carrying the day's work, `toggl stop` collects, for example:

```text
Forgejo commits:
Repository: voicesense-backend
Add workspace quota checks to billing endpoints

Forgejo pull requests:
[PR #812] Persist quota decisions across retries
```

and produces a concrete one-line Toggl description from them.

## Testing

Following the existing `httptest` + base-URL-override pattern:

- Remote URL parsing: SSH and HTTPS Forgejo remotes resolve to `owner/repo`; non-Forgejo hosts are ignored.
- Commit collection: author and committer-date filtering, paging stop condition, and mapping to `ProjectCommit`.
- Issue/PR search: relationship flags, allowlist filtering, PR/issue separation, pseudo-commit conversion.
- Integration: `SplitCommits` feed split calculation; unmapped repositories never produce a `ProjectID 0` split; `PromptSections` appear in the non-split prompt without duplicating pseudo-commits.
- The non-zero `ProjectID` split filter applies to the combined slice, so it also hardens existing local collection: a `projects.<name>` entry without a `project_id` no longer produces a `ProjectID 0` split.
- Disabled config: no Forgejo calls and unchanged output.
- Failure: API error yields a warning and local-only behavior.

## Documentation

- Update `README.md` with the `forgejo` configuration and its effect on summaries.
- Add an OpenSpec change (`proposal`, `design`, `spec`, `tasks`) describing the new capability and modified `git-commit-collection` / `ai-summary-generation` behavior.

## Non-goals

- Do not replace or remove local `git log` collection.
- Do not introduce an activity-source interface or refactor beyond what the integration requires.
- Do not add commands, prompts, or interactive flows.
- Do not store tokens anywhere other than `~/.toggl.yaml` / environment.
