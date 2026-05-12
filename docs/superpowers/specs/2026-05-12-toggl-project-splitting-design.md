# Toggl project splitting design

## Goal

Update `toggl stop` so a single running Toggl timer can be split into separate Toggl entries for Senseloom projects based on git activity.

The resulting entries must cover exactly the original `toggl start` to `toggl stop` range: no trimming, extension, gaps, or overlaps.

Historical cleanup for the current month will be handled outside this CLI change. No new history-splitting command is needed.

## Configuration

Add named project groups to `~/.toggl.yaml`:

```yaml
toggl:
  api_key: "your_toggl_api_token"
  workspace_id: 8320206
  project_id: 204198137

openai:
  api_key: "your_openai_api_key"
  model: "gpt-4o-mini"

git:
  user: "Your Name"

projects:
  cortex:
    project_id: 111111111
    repositories:
      - "~/Projects/lkq/cortex"

  voicesense:
    project_id: 222222222
    repositories:
      - "~/Projects/lkq/other-repo"
      - "~/Projects/lkq/another-voicesense-repo"
```

Existing flat `repositories:` config remains supported as a legacy fallback. If `projects:` is absent, `toggl stop` keeps the current single-entry behavior.

Repository paths should support `~` expansion.

## Commit collection

Keep the existing text commit collection for legacy behavior, and add structured project-aware collection for configured project groups.

Structured commits should include:

- project name
- Toggl project ID
- repository path/name
- commit subject
- commit timestamp

Use git log with ISO timestamps and subjects:

```bash
git -C <repo> log --since=<start> --until=<stop> --author=<user> --pretty=format:%cI%x09%s
```

If a repository path is missing or not a git repository, print a warning and skip it, matching the current behavior.

## Split algorithm

Use timeline midpoint allocation.

1. Sort structured commits by timestamp.
2. For each commit, compute its ownership window:
   - first commit starts at the original entry start,
   - otherwise starts at the midpoint between previous commit time and this commit time,
   - last commit stops at the original entry stop,
   - otherwise stops at the midpoint between this commit time and next commit time.
3. Add each ownership window duration to that commit's project.
4. Convert project durations into contiguous Toggl entry ranges that exactly cover the original entry range.

Example:

```text
Original timer: 09:00 -> 13:00

09:30 Cortex
11:30 VoiceSense
12:30 VoiceSense
```

Ownership:

- Cortex: 09:00 -> 10:30
- VoiceSense marker 1: 10:30 -> 12:00
- VoiceSense marker 2: 12:00 -> 13:00

Final ranges:

- Cortex: 09:00 -> 10:30
- VoiceSense: 10:30 -> 13:00

The final ranges are contiguous and based on project totals, not necessarily on exact marker windows.

## `toggl stop` behavior

1. Get the current entry.
2. Stop the entry, preserving the original start and computed stop time.
3. If project groups are configured, collect structured project commits.
4. Build the summary prompt from commits and worklog.
5. Generate one description for the whole session.
6. Apply project splitting when possible.

Cases:

- No `projects:` config: keep current behavior.
- No project commits: keep current behavior.
- Commits for one project: update the original entry to that project and preserve the original full range.
- Commits for multiple projects: update the original entry to the first split range, then create additional entries for the remaining ranges.

For additional entries, call `POST /workspaces/{workspace_id}/time_entries` with workspace ID, project ID, description, start, stop, duration, and `created_with: toggl-cli`.

## Failure handling

Stop the timer first, as today.

If splitting fails before any update/create call, leave the stopped original entry unchanged apart from being stopped.

If updating the original split succeeds but creating another split entry fails, return an error and preserve `/tmp/toggl-worklog.txt`. Only remove the worklog after all Toggl updates and creates succeed.

On success, print a concise split summary such as:

```text
Stopped tracking. Split into:
- cortex: 1h30m
- voicesense: 2h30m
Summary saved.
```

## Tests

Add or update tests for:

- parsing the new `projects:` config,
- preserving legacy flat `repositories:` config,
- structured commit collection with fixed git commit timestamps,
- midpoint allocation and exact contiguous ranges,
- single-project commit sessions,
- no-commit sessions,
- full `stop` split integration with mocked Toggl requests,
- preserving the worklog if additional entry creation fails.

Verification commands:

```bash
make fmt
make test
make build
```
