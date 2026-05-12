# Toggl Project Splitting Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Split `toggl stop` sessions into Senseloom project-specific Toggl entries based on project-aware git commit timestamps.

**Architecture:** Keep the existing single-file Go CLI structure. Add project-group config, structured commit collection, midpoint split calculation, and Toggl entry creation helpers, then wire them into `stopCmd` while preserving legacy behavior when `projects:` is absent or no project commits are found.

**Tech Stack:** Go 1.25, Cobra, yaml.v3, net/http test servers, local git CLI in tests.

---

## Files

- Modify `src/main.go`: config types, path expansion, project-aware commit collection, split calculation, Toggl entry update/create helpers, `stopCmd` integration.
- Modify `src/main_test.go`: TDD tests for config parsing, path expansion, project commit collection, midpoint split ranges, single-project stop, multi-project stop, create failure preserving worklog.
- Modify `README.md`: document `projects:` config and stop splitting behavior.
- Existing uncommitted work in `src/main.go` and `src/main_test.go` must be preserved.

---

### Task 1: Config and path expansion

**Files:**
- Modify: `src/main.go`
- Test: `src/main_test.go`

- [ ] **Step 1: Write failing config parsing test**

Add to `src/main_test.go` near config tests:

```go
func TestLoadConfig_ProjectGroups(t *testing.T) {
	content := `
toggl:
  api_key: "test-key"
  workspace_id: 123
  project_id: 456
openai:
  api_key: "sk-test"
  model: "gpt-4"
git:
  user: "testuser"
projects:
  cortex:
    project_id: 111
    repositories:
      - "~/Projects/lkq/cortex"
  voicesense:
    project_id: 222
    repositories:
      - "/tmp/voice"
`
	var c Config
	if err := yaml.Unmarshal([]byte(content), &c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Projects["cortex"].ProjectID != 111 {
		t.Fatalf("expected cortex project ID 111, got %d", c.Projects["cortex"].ProjectID)
	}
	if c.Projects["voicesense"].Repositories[0] != "/tmp/voice" {
		t.Fatalf("expected voicesense repo path, got %q", c.Projects["voicesense"].Repositories[0])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./src -run TestLoadConfig_ProjectGroups -count=1`

Expected: FAIL because `Config` has no `Projects` field.

- [ ] **Step 3: Implement minimal config support**

Add below `Config` or above it in `src/main.go`:

```go
type ProjectConfig struct {
	ProjectID    int      `yaml:"project_id"`
	Repositories []string `yaml:"repositories"`
}
```

Add to `Config`:

```go
	Projects map[string]ProjectConfig `yaml:"projects"`
```

- [ ] **Step 4: Run passing test**

Run: `go test ./src -run TestLoadConfig_ProjectGroups -count=1`

Expected: PASS.

- [ ] **Step 5: Write failing tilde expansion test**

Add:

```go
func TestExpandRepoPath_ExpandsHomeTilde(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	got := expandRepoPath("~/Projects/lkq/cortex")
	want := filepath.Join(home, "Projects", "lkq", "cortex")
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}
```

- [ ] **Step 6: Run test to verify it fails**

Run: `go test ./src -run TestExpandRepoPath_ExpandsHomeTilde -count=1`

Expected: FAIL because `expandRepoPath` is undefined.

- [ ] **Step 7: Implement minimal path expansion**

Add to helper functions in `src/main.go`:

```go
func expandRepoPath(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			if path == "~" {
				return home
			}
			return filepath.Join(home, path[2:])
		}
	}
	return path
}
```

- [ ] **Step 8: Run task tests**

Run: `go test ./src -run 'TestLoadConfig_ProjectGroups|TestExpandRepoPath_ExpandsHomeTilde' -count=1`

Expected: PASS.

---

### Task 2: Structured project commits and midpoint split ranges

**Files:**
- Modify: `src/main.go`
- Test: `src/main_test.go`

- [ ] **Step 1: Write failing structured commit collection test**

Add a test creating two git repos with fixed timestamps and config project groups. It should call `collectProjectCommits(entry)` and expect project names, project IDs, subjects, and times.

Run: `go test ./src -run TestCollectProjectCommits_CollectsProjectMetadata -count=1`

Expected: FAIL because `collectProjectCommits` is undefined.

- [ ] **Step 2: Implement structured commit collection**

Add:

```go
type ProjectCommit struct {
	ProjectName string
	ProjectID   int
	RepoPath    string
	RepoName    string
	Subject     string
	Time        time.Time
}
```

Implement `collectProjectCommits(entry TogglTimeEntry) []ProjectCommit` using `git log --pretty=format:%cI%x09%s`, `expandRepoPath`, existing warning style, and sorting by time.

- [ ] **Step 3: Run collection test**

Run: `go test ./src -run TestCollectProjectCommits_CollectsProjectMetadata -count=1`

Expected: PASS.

- [ ] **Step 4: Write failing midpoint split test**

Add `TestCalculateProjectSplits_MidpointAllocationProducesContiguousRanges` using timer `09:00 -> 13:00` and commits at `09:30 cortex`, `11:30 voicesense`, `12:30 voicesense`. Expect cortex `09:00 -> 10:30`, voicesense `10:30 -> 13:00`.

Run: `go test ./src -run TestCalculateProjectSplits_MidpointAllocationProducesContiguousRanges -count=1`

Expected: FAIL because split calculation is undefined.

- [ ] **Step 5: Implement split calculation**

Add:

```go
type ProjectSplit struct {
	ProjectName string
	ProjectID   int
	Start       time.Time
	Stop        time.Time
	Duration    time.Duration
}
```

Implement `calculateProjectSplits(entry TogglTimeEntry, commits []ProjectCommit) []ProjectSplit` with midpoint ownership and contiguous final ranges preserving original start/stop exactly.

- [ ] **Step 6: Run split tests**

Run: `go test ./src -run 'TestCollectProjectCommits_CollectsProjectMetadata|TestCalculateProjectSplits_MidpointAllocationProducesContiguousRanges' -count=1`

Expected: PASS.

---

### Task 3: Toggl split persistence helpers

**Files:**
- Modify: `src/main.go`
- Test: `src/main_test.go`

- [ ] **Step 1: Write failing create-entry helper test**

Add `TestCreateTimeEntry_PostsStoppedEntry` using `setupTogglServer`, call `createTimeEntry(split, "description", workspaceID)`, and assert POST path/body includes `workspace_id`, `project_id`, `start`, `stop`, positive `duration`, and `created_with`.

Run: `go test ./src -run TestCreateTimeEntry_PostsStoppedEntry -count=1`

Expected: FAIL because helper is undefined.

- [ ] **Step 2: Implement helper**

Implement `createTimeEntry(split ProjectSplit, description string, workspaceID int) error` with POST to `workspaces/%d/time_entries`.

- [ ] **Step 3: Run helper test**

Run: `go test ./src -run TestCreateTimeEntry_PostsStoppedEntry -count=1`

Expected: PASS.

---

### Task 4: Integrate project splitting into `toggl stop`

**Files:**
- Modify: `src/main.go`
- Test: `src/main_test.go`

- [ ] **Step 1: Write failing single-project stop integration test**

Add `TestStopCmd_ProjectConfigSingleProjectUpdatesOriginalEntry` with one cortex repo commit. Mock Toggl should observe current GET, stop PUT, final update PUT. Assert final update body has cortex project ID and no POST is made.

Run: `go test ./src -run TestStopCmd_ProjectConfigSingleProjectUpdatesOriginalEntry -count=1`

Expected: FAIL because stop does not use `cfg.Projects`.

- [ ] **Step 2: Implement minimal single-project integration**

In `stopCmd`, when `len(cfg.Projects) > 0`, collect project commits. If splits length is one, set entry project/start/stop/description and call an update helper; preserve worklog deletion only after success.

- [ ] **Step 3: Run single-project test**

Run: `go test ./src -run TestStopCmd_ProjectConfigSingleProjectUpdatesOriginalEntry -count=1`

Expected: PASS.

- [ ] **Step 4: Write failing multi-project stop integration test**

Add `TestStopCmd_ProjectConfigMultipleProjectsSplitsEntries` with commits producing cortex 09:00-10:30 and voicesense 10:30-13:00. Mock Toggl should observe final PUT for original and POST for extra entry. Assert exact contiguous ranges and project IDs.

Run: `go test ./src -run TestStopCmd_ProjectConfigMultipleProjectsSplitsEntries -count=1`

Expected: FAIL because extra entry creation is not wired.

- [ ] **Step 5: Implement multi-project integration**

Add `applyProjectSplits(entry TogglTimeEntry, splits []ProjectSplit, description string) error`. It updates original entry with first split and creates entries for remaining splits.

- [ ] **Step 6: Run multi-project integration test**

Run: `go test ./src -run TestStopCmd_ProjectConfigMultipleProjectsSplitsEntries -count=1`

Expected: PASS.

- [ ] **Step 7: Write failing create-failure worklog preservation test**

Add `TestStopCmd_ProjectSplitCreateFailurePreservesWorklog` where extra POST fails by closing the connection. Assert command returns error and `worklogPath()` still exists.

Run: `go test ./src -run TestStopCmd_ProjectSplitCreateFailurePreservesWorklog -count=1`

Expected: FAIL if worklog is removed too early.

- [ ] **Step 8: Implement/fix worklog preservation**

Ensure `os.Remove(worklogPath())` happens only after all update/create operations succeed.

- [ ] **Step 9: Run stop integration tests**

Run: `go test ./src -run 'TestStopCmd_ProjectConfig|TestStopCmd_ProjectSplitCreateFailurePreservesWorklog' -count=1`

Expected: PASS.

---

### Task 5: Documentation and full verification

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Update README**

Document the `projects:` config map and explain that `toggl stop` can split one timer into multiple project-specific entries when project commits are detected. Mention legacy `repositories:` still works for summary-only behavior.

- [ ] **Step 2: Run formatter**

Run: `make fmt`

Expected: `go fmt ./...` succeeds.

- [ ] **Step 3: Run tests**

Run: `make test`

Expected: all tests pass.

- [ ] **Step 4: Run build**

Run: `make build`

Expected: binary builds successfully at `./bin/toggl`.

- [ ] **Step 5: Final status**

Run: `git status --short`

Expected: only intended files changed: `src/main.go`, `src/main_test.go`, `README.md`, design doc, and this plan doc.

---

## Self-review

- Spec coverage: config, path expansion, structured commits, midpoint allocation, stop integration, create helper, worklog preservation, docs, and verification are covered.
- Placeholder scan: no TBD/TODO placeholders.
- Type consistency: `ProjectConfig`, `ProjectCommit`, `ProjectSplit`, `collectProjectCommits`, `calculateProjectSplits`, and `createTimeEntry` names are consistent across tasks.
