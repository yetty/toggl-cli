# Forgejo Activity Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Include the user's Forgejo activity (commits, pull requests, issues) in Toggl work summaries produced by `toggl stop`, `fill-empty-descriptions`, and `repair-summaries`.

**Architecture:** Add a `forgejo` config section and a collector that resolves Forgejo repositories from the existing local repo config (via `git remote get-url origin`) plus an optional explicit list. The collector returns a `ForgejoActivity` with two disjoint views: project-mapped `SplitCommits` for the time-splitting path, and `PromptSections` for every other summarization path. Local `git log` collection is preserved unchanged.

**Tech Stack:** Go 1.25, Cobra, `net/http`, `net/url`, `encoding/json`, `httptest` for tests. Single package `main` in `src/main.go` and `src/main_test.go`.

**Spec:** `docs/superpowers/specs/2026-09-22-forgejo-activity-design.md`

---

## File Structure

- `src/main.go` — all production code (single package, existing convention).
- `src/main_test.go` — all tests (existing convention).
- `README.md` — user-facing docs.
- `openspec/changes/add-forgejo-activity/` — OpenSpec change artifacts.

No new files for production code; the repository deliberately keeps everything in one `main` package. Functions are grouped by responsibility within the file.

---

### Task 1: Forgejo configuration

**Files:**
- Modify: `src/main.go` (Config struct ~line 27-43, new `ForgejoConfig`, `loadConfig` ~line 154-171)
- Test: `src/main_test.go`

- [ ] **Step 1: Write the failing test**

Add to `src/main_test.go`:

```go
func TestForgejoConfigEnabledOnlyWhenRequiredFieldsPresent(t *testing.T) {
	var c Config
	if c.Forgejo.Enabled() {
		t.Fatal("forgejo integration should be disabled when config is absent")
	}

	c.Forgejo.URL = "https://infra.example.com"
	if c.Forgejo.Enabled() {
		t.Fatal("forgejo integration should be disabled without an api key")
	}

	c.Forgejo.APIKey = "token"
	if !c.Forgejo.Enabled() {
		t.Fatal("forgejo integration should be enabled when url and api key are present")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./src/ -run TestForgejoConfigEnabledOnlyWhenRequiredFieldsPresent -v`
Expected: FAIL — `c.Forgejo undefined` (compile error).

- [ ] **Step 3: Write minimal implementation**

In `src/main.go`, add the field to `Config` (inside the struct, after `Projects`):

```go
	Forgejo      ForgejoConfig            `yaml:"forgejo"`
```

Add the type after `CalendarConfig`:

```go
type ForgejoConfig struct {
	URL          string   `yaml:"url"`
	APIKey       string   `yaml:"api_key"`
	Repositories []string `yaml:"repositories"`
}

func (c ForgejoConfig) Enabled() bool {
	return c.URL != "" && c.APIKey != ""
}
```

In `loadConfig`, after the `OPENAI_API_KEY` override, add:

```go
	if key := os.Getenv("FORGEJO_API_KEY"); key != "" {
		cfg.Forgejo.APIKey = key
	}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./src/ -run TestForgejoConfigEnabledOnlyWhenRequiredFieldsPresent -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add src/main.go src/main_test.go
git commit -m "Add forgejo configuration section"
```

---

### Task 2: Parse Forgejo git remotes

**Files:**
- Modify: `src/main.go` (helpers near `expandRepoPath`, ~line 262-274)
- Test: `src/main_test.go`

- [ ] **Step 1: Write the failing test**

Add to `src/main_test.go`:

```go
func TestParseForgejoRemote(t *testing.T) {
	cases := []struct {
		name      string
		raw       string
		wantOwner string
		wantName  string
		wantOK    bool
	}{
		{name: "https with git suffix", raw: "https://infra.senseloom.com/voicesense/voicesense-backend.git", wantOwner: "voicesense", wantName: "voicesense-backend", wantOK: true},
		{name: "https without suffix", raw: "https://infra.senseloom.com/voicesense/voicesense-web", wantOwner: "voicesense", wantName: "voicesense-web", wantOK: true},
		{name: "ssh scp style", raw: "git@infra.senseloom.com:voicesense/senseloom-infra.git", wantOwner: "voicesense", wantName: "senseloom-infra", wantOK: true},
		{name: "ssh url style", raw: "ssh://git@infra.senseloom.com/voicesense/redat-mock.git", wantOwner: "voicesense", wantName: "redat-mock", wantOK: true},
		{name: "different host", raw: "git@github.com:yetty/toggl-cli.git", wantOK: false},
		{name: "too few path segments", raw: "https://infra.senseloom.com/voicesense.git", wantOK: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			owner, name, ok := parseForgejoRemote(tc.raw, "infra.senseloom.com")
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if owner != tc.wantOwner || name != tc.wantName {
				t.Fatalf("got %q/%q, want %q/%q", owner, name, tc.wantOwner, tc.wantName)
			}
		})
	}
}

func TestSplitOwnerRepo(t *testing.T) {
	owner, name, ok := splitOwnerRepo("voicesense/voicesense-backend")
	if !ok || owner != "voicesense" || name != "voicesense-backend" {
		t.Fatalf("got %q/%q/%v, want voicesense/voicesense-backend/true", owner, name, ok)
	}
	if _, _, ok := splitOwnerRepo("voicesense"); ok {
		t.Fatal("single-segment value should not parse")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./src/ -run 'TestParseForgejoRemote|TestSplitOwnerRepo' -v`
Expected: FAIL — `parseForgejoRemote undefined`.

- [ ] **Step 3: Write minimal implementation**

In `src/main.go`, add after `expandRepoPath`:

```go
func stripHostPort(host string) string {
	if i := strings.LastIndex(host, ":"); i >= 0 {
		return host[:i]
	}
	return host
}

func parseForgejoRemote(raw, forgejoHost string) (string, string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", false
	}

	var remoteHost, path string
	switch {
	case strings.Contains(raw, "://"):
		parsed, err := url.Parse(raw)
		if err != nil {
			return "", "", false
		}
		remoteHost = parsed.Hostname()
		path = strings.TrimPrefix(parsed.Path, "/")
	case strings.Contains(raw, ":"):
		i := strings.Index(raw, ":")
		left := raw[:i]
		path = raw[i+1:]
		if j := strings.LastIndex(left, "@"); j >= 0 {
			left = left[j+1:]
		}
		remoteHost = left
	default:
		return "", "", false
	}

	if !strings.EqualFold(stripHostPort(remoteHost), stripHostPort(forgejoHost)) {
		return "", "", false
	}

	path = strings.TrimSuffix(strings.TrimSuffix(path, "/"), ".git")
	parts := strings.Split(path, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func splitOwnerRepo(value string) (string, string, bool) {
	parts := strings.Split(strings.TrimSpace(value), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./src/ -run 'TestParseForgejoRemote|TestSplitOwnerRepo' -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add src/main.go src/main_test.go
git commit -m "Add Forgejo git remote parsing"
```

---

### Task 3: Resolve Forgejo repositories

**Files:**
- Modify: `src/main.go` (new types/functions after Task 2 helpers)
- Test: `src/main_test.go`

- [ ] **Step 1: Write the failing test**

Add to `src/main_test.go`:

```go
func TestResolveForgejoRepositoriesMergesSourcesAndMappings(t *testing.T) {
	oldCfg, oldRemote := cfg, gitRemoteURL
	defer func() { cfg, gitRemoteURL = oldCfg, oldRemote }()

	cfg = Config{}
	cfg.Forgejo.URL = "https://infra.senseloom.com"
	cfg.Forgejo.APIKey = "token"
	cfg.Forgejo.Repositories = []string{"voicesense/voicesense-backend"}
	cfg.Repositories = []string{"/repos/legacy"}
	cfg.Projects = map[string]ProjectConfig{
		"cortex": {
			ProjectID:    219802887,
			Repositories: []string{"/repos/cortex"},
		},
	}

	gitRemoteURL = func(repo string) (string, error) {
		switch repo {
		case "/repos/legacy":
			return "https://infra.senseloom.com/voicesense/voicesense-backend.git", nil
		case "/repos/cortex":
			return "git@infra.senseloom.com:lkq/cortex.git", nil
		default:
			return "https://github.com/someone/else.git", nil
		}
	}

	repos := resolveForgejoRepositories()
	if len(repos) != 2 {
		t.Fatalf("resolved %d repositories, want 2: %+v", len(repos), repos)
	}

	byName := map[string]ForgejoRepo{}
	for _, repo := range repos {
		byName[repo.FullName()] = repo
	}

	backend, ok := byName["voicesense/voicesense-backend"]
	if !ok {
		t.Fatalf("missing voicesense-backend: %+v", repos)
	}
	if backend.ProjectID != 0 {
		t.Fatalf("explicit-only repository should stay unmapped, got project %d", backend.ProjectID)
	}

	cortex, ok := byName["lkq/cortex"]
	if !ok {
		t.Fatalf("missing lkq/cortex: %+v", repos)
	}
	if cortex.ProjectName != "cortex" || cortex.ProjectID != 219802887 {
		t.Fatalf("cortex mapping = %q/%d, want cortex/219802887", cortex.ProjectName, cortex.ProjectID)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./src/ -run TestResolveForgejoRepositoriesMergesSourcesAndMappings -v`
Expected: FAIL — `gitRemoteURL undefined` / `resolveForgejoRepositories undefined`.

- [ ] **Step 3: Write minimal implementation**

In `src/main.go`, add after `splitOwnerRepo`:

```go
type ForgejoRepo struct {
	Owner       string
	Name        string
	ProjectName string
	ProjectID   int
}

func (r ForgejoRepo) FullName() string {
	return r.Owner + "/" + r.Name
}

var gitRemoteURL = func(repo string) (string, error) {
	out, err := exec.Command("git", "-C", repo, "remote", "get-url", "origin").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func forgejoHost() string {
	if cfg.Forgejo.URL == "" {
		return ""
	}
	parsed, err := url.Parse(cfg.Forgejo.URL)
	if err != nil || parsed.Hostname() == "" {
		return ""
	}
	return strings.ToLower(parsed.Hostname())
}

func resolveForgejoRepositories() []ForgejoRepo {
	host := forgejoHost()
	if host == "" {
		return nil
	}

	order := []string{}
	byName := map[string]ForgejoRepo{}
	add := func(owner, name, projectName string, projectID int) {
		key := strings.ToLower(owner + "/" + name)
		if existing, ok := byName[key]; ok {
			if existing.ProjectID == 0 && projectID != 0 {
				existing.ProjectName = projectName
				existing.ProjectID = projectID
				byName[key] = existing
			}
			return
		}
		byName[key] = ForgejoRepo{Owner: owner, Name: name, ProjectName: projectName, ProjectID: projectID}
		order = append(order, key)
	}

	for _, full := range cfg.Forgejo.Repositories {
		owner, name, ok := splitOwnerRepo(full)
		if !ok {
			fmt.Fprintf(os.Stderr, "Warning: skipping malformed forgejo repository %q (expected owner/repo)\n", full)
			continue
		}
		add(owner, name, "", 0)
	}

	for _, path := range cfg.Repositories {
		owner, name, ok := forgejoRepoFromLocalPath(expandRepoPath(path), host)
		if !ok {
			continue
		}
		add(owner, name, "", 0)
	}

	projectNames := make([]string, 0, len(cfg.Projects))
	for name := range cfg.Projects {
		projectNames = append(projectNames, name)
	}
	sort.Strings(projectNames)
	for _, projectName := range projectNames {
		project := cfg.Projects[projectName]
		for _, path := range project.Repositories {
			owner, name, ok := forgejoRepoFromLocalPath(expandRepoPath(path), host)
			if !ok {
				continue
			}
			add(owner, name, projectName, project.ProjectID)
		}
	}

	result := make([]ForgejoRepo, 0, len(order))
	for _, key := range order {
		result = append(result, byName[key])
	}
	return result
}

func forgejoRepoFromLocalPath(path, host string) (string, string, bool) {
	remote, err := gitRemoteURL(path)
	if err != nil {
		return "", "", false
	}
	return parseForgejoRemote(remote, host)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./src/ -run TestResolveForgejoRepositoriesMergesSourcesAndMappings -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add src/main.go src/main_test.go
git commit -m "Resolve Forgejo repositories from config and git remotes"
```

---

### Task 4: Fetch Forgejo commits

**Files:**
- Modify: `src/main.go` (new API types and functions; base request helper)
- Test: `src/main_test.go`

- [ ] **Step 1: Write the failing test**

Add to `src/main_test.go`:

```go
func TestFetchForgejoCommitsFiltersByAuthorAndWindow(t *testing.T) {
	oldCfg := cfg
	defer func() { cfg = oldCfg }()

	cfg = Config{}
	cfg.Forgejo.URL = "http://placeholder"
	cfg.Forgejo.APIKey = "token"
	cfg.Git.User = `Juda Kaleta\|juda@example.com\|Forgejo Actions`

	entry := TogglTimeEntry{
		Start: time.Date(2026, 6, 9, 10, 0, 0, 0, time.UTC),
		Stop:  time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC),
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "token token" {
			t.Fatalf("Authorization = %q, want %q", got, "token token")
		}
		if r.URL.Path != "/api/v1/repos/voicesense/voicesense-backend/commits" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode([]map[string]any{
			{
				"sha": "a",
				"commit": map[string]any{
					"message":   "feat: quota checks\n\nmore detail",
					"author":    map[string]any{"name": "Juda Kaleta", "email": "juda@example.com", "date": "2026-06-09T10:30:00Z"},
					"committer": map[string]any{"name": "Juda Kaleta", "email": "juda@example.com", "date": "2026-06-09T10:30:00Z"},
				},
				"author": map[string]any{"login": "juda"},
			},
			{
				"sha": "b",
				"commit": map[string]any{
					"message":   "chore: other person",
					"author":    map[string]any{"name": "Someone Else", "email": "other@example.com", "date": "2026-06-09T10:45:00Z"},
					"committer": map[string]any{"name": "Someone Else", "email": "other@example.com", "date": "2026-06-09T10:45:00Z"},
				},
				"author": map[string]any{"login": "other"},
			},
			{
				"sha": "c",
				"commit": map[string]any{
					"message":   "feat: outside window",
					"author":    map[string]any{"name": "Juda Kaleta", "email": "juda@example.com", "date": "2026-06-08T09:00:00Z"},
					"committer": map[string]any{"name": "Juda Kaleta", "email": "juda@example.com", "date": "2026-06-08T09:00:00Z"},
				},
				"author": map[string]any{"login": "juda"},
			},
		})
	}))
	defer server.Close()
	cfg.Forgejo.URL = server.URL

	repo := ForgejoRepo{Owner: "voicesense", Name: "voicesense-backend", ProjectName: "voicesense", ProjectID: 204198137}
	commits, err := fetchForgejoCommits(entry, repo)
	if err != nil {
		t.Fatalf("fetchForgejoCommits returned error: %v", err)
	}
	if len(commits) != 1 {
		t.Fatalf("got %d commits, want 1: %+v", len(commits), commits)
	}
	if commits[0].Subject != "feat: quota checks" {
		t.Fatalf("subject = %q, want first line only", commits[0].Subject)
	}
	if commits[0].ProjectID != 204198137 || commits[0].RepoName != "voicesense-backend" {
		t.Fatalf("mapping = %d/%q, want 204198137/voicesense-backend", commits[0].ProjectID, commits[0].RepoName)
	}
	if !commits[0].Time.Equal(time.Date(2026, 6, 9, 10, 30, 0, 0, time.UTC)) {
		t.Fatalf("time = %s, want committer date", commits[0].Time)
	}
}

func TestMatchesForgejoAuthor(t *testing.T) {
	oldCfg := cfg
	defer func() { cfg = oldCfg }()
	cfg = Config{}
	cfg.Git.User = `Juda Kaleta\|juda@example.com\|Forgejo Actions`

	if !matchesForgejoAuthor("Juda Kaleta", "juda@example.com", "juda") {
		t.Fatal("expected token match by name")
	}
	if !matchesForgejoAuthor("Forgejo Actions", "bot@example.com", "") {
		t.Fatal("expected token match for automation author name")
	}
	if matchesForgejoAuthor("Someone Else", "other@example.com", "other") {
		t.Fatal("unexpected match for unrelated author")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./src/ -run 'TestFetchForgejoCommitsFiltersByAuthorAndWindow|TestMatchesForgejoAuthor' -v`
Expected: FAIL — `fetchForgejoCommits undefined`.

- [ ] **Step 3: Write minimal implementation**

In `src/main.go`, add after `resolveForgejoRepositories`:

```go
const forgejoPageSize = 50

const forgejoMaxPages = 10

type forgejoCommitUser struct {
	Name  string    `json:"name"`
	Email string    `json:"email"`
	Date  time.Time `json:"date"`
}

type forgejoCommit struct {
	SHA    string `json:"sha"`
	Commit struct {
		Message   string            `json:"message"`
		Author    forgejoCommitUser `json:"author"`
		Committer forgejoCommitUser `json:"committer"`
	} `json:"commit"`
	Author *struct {
		Login string `json:"login"`
	} `json:"author"`
}

func forgejoAPIBase() string {
	return strings.TrimRight(cfg.Forgejo.URL, "/") + "/api/v1/"
}

func forgejoRequest(path string) ([]byte, error) {
	req, err := http.NewRequest("GET", forgejoAPIBase()+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "token "+cfg.Forgejo.APIKey)
	req.Header.Set("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("Forgejo API error: %s", resp.Status)
	}
	return io.ReadAll(resp.Body)
}

func forgejoAuthorTokens() []string {
	tokens := []string{}
	for _, token := range strings.Split(cfg.Git.User, `\|`) {
		token = strings.ToLower(strings.TrimSpace(token))
		if token != "" {
			tokens = append(tokens, token)
		}
	}
	return tokens
}

func matchesForgejoAuthor(name, email, login string) bool {
	name = strings.ToLower(name)
	email = strings.ToLower(email)
	login = strings.ToLower(login)
	for _, token := range forgejoAuthorTokens() {
		if login != "" && login == token {
			return true
		}
		if name != "" && strings.Contains(name, token) {
			return true
		}
		if email != "" && strings.Contains(email, token) {
			return true
		}
	}
	return false
}

func firstLine(value string) string {
	if i := strings.IndexByte(value, '\n'); i >= 0 {
		return strings.TrimSpace(value[:i])
	}
	return strings.TrimSpace(value)
}

func fetchForgejoCommits(entry TogglTimeEntry, repo ForgejoRepo) ([]ProjectCommit, error) {
	var commits []ProjectCommit
	for page := 1; page <= forgejoMaxPages; page++ {
		path := fmt.Sprintf("repos/%s/%s/commits?limit=%d&page=%d&stat=false&verification=false&files=false",
			url.PathEscape(repo.Owner), url.PathEscape(repo.Name), forgejoPageSize, page)
		data, err := forgejoRequest(path)
		if err != nil {
			return nil, err
		}
		var items []forgejoCommit
		if err := json.Unmarshal(data, &items); err != nil {
			return nil, err
		}
		if len(items) == 0 {
			break
		}
		oldest := items[len(items)-1].Commit.Committer.Date
		for _, item := range items {
			committedAt := item.Commit.Committer.Date
			if committedAt.Before(entry.Start) || committedAt.After(entry.Stop) {
				continue
			}
			login := ""
			if item.Author != nil {
				login = item.Author.Login
			}
			if !matchesForgejoAuthor(item.Commit.Author.Name, item.Commit.Author.Email, login) {
				continue
			}
			commits = append(commits, ProjectCommit{
				ProjectName: repo.ProjectName,
				ProjectID:   repo.ProjectID,
				RepoName:    repo.Name,
				Subject:     firstLine(item.Commit.Message),
				Time:        committedAt,
			})
		}
		if len(items) < forgejoPageSize || oldest.Before(entry.Start) {
			break
		}
		if page == forgejoMaxPages {
			fmt.Fprintf(os.Stderr, "Warning: Forgejo commits for %s truncated at %d pages\n", repo.FullName(), forgejoMaxPages)
		}
	}
	return commits, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./src/ -run 'TestFetchForgejoCommitsFiltersByAuthorAndWindow|TestMatchesForgejoAuthor' -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add src/main.go src/main_test.go
git commit -m "Fetch Forgejo commits for the session window"
```

---

### Task 5: Fetch Forgejo pull requests and issues

**Files:**
- Modify: `src/main.go` (new type and function; add `strconv` import)
- Test: `src/main_test.go`

- [ ] **Step 1: Write the failing test**

Add to `src/main_test.go`:

```go
func TestFetchForgejoIssuesFiltersToResolvedRepositories(t *testing.T) {
	oldCfg := cfg
	defer func() { cfg = oldCfg }()

	cfg = Config{}
	cfg.Forgejo.APIKey = "token"

	entry := TogglTimeEntry{
		Start: time.Date(2026, 6, 9, 10, 0, 0, 0, time.UTC),
		Stop:  time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC),
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/repos/issues/search" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		for _, param := range []string{"created", "assigned", "review_requested", "reviewed"} {
			if r.URL.Query().Get(param) != "true" {
				t.Fatalf("query param %q = %q, want true", param, r.URL.Query().Get(param))
			}
		}
		if r.URL.Query().Get("mentioned") != "" {
			t.Fatal("mentioned should not be requested")
		}
		json.NewEncoder(w).Encode([]map[string]any{
			{
				"number":     12,
				"title":      "Persist quota decisions",
				"updated_at": "2026-06-09T11:00:00Z",
				"pull_request": map[string]any{"merged": false},
				"repository": map[string]any{"full_name": "voicesense/voicesense-backend"},
			},
			{
				"number":     45,
				"title":      "Retry backoff",
				"updated_at": "2026-06-09T11:30:00Z",
				"repository": map[string]any{"full_name": "voicesense/voicesense-web"},
			},
			{
				"number":     99,
				"title":      "Unrelated repo work",
				"updated_at": "2026-06-09T11:45:00Z",
				"repository": map[string]any{"full_name": "other/other-repo"},
			},
		})
	}))
	defer server.Close()
	cfg.Forgejo.URL = server.URL

	repos := []ForgejoRepo{
		{Owner: "voicesense", Name: "voicesense-backend"},
		{Owner: "voicesense", Name: "voicesense-web"},
	}
	issues, err := fetchForgejoIssues(entry, repos)
	if err != nil {
		t.Fatalf("fetchForgejoIssues returned error: %v", err)
	}
	if len(issues) != 2 {
		t.Fatalf("got %d issues, want 2: %+v", len(issues), issues)
	}
	if issues[0].Number != 12 || issues[1].Number != 45 {
		t.Fatalf("unexpected issue numbers: %+v", issues)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./src/ -run TestFetchForgejoIssuesFiltersToResolvedRepositories -v`
Expected: FAIL — `fetchForgejoIssues undefined`.

- [ ] **Step 3: Write minimal implementation**

In `src/main.go`, add `"strconv"` to the import block (alphabetical order, after `"sort"`). Then add after `fetchForgejoCommits`:

```go
type forgejoIssue struct {
	Number      int       `json:"number"`
	Title       string    `json:"title"`
	UpdatedAt   time.Time `json:"updated_at"`
	PullRequest *struct{} `json:"pull_request"`
	Repository  struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
}

func fetchForgejoIssues(entry TogglTimeEntry, repos []ForgejoRepo) ([]forgejoIssue, error) {
	allowed := map[string]bool{}
	for _, repo := range repos {
		allowed[strings.ToLower(repo.FullName())] = true
	}

	var result []forgejoIssue
	for page := 1; page <= forgejoMaxPages; page++ {
		params := url.Values{}
		params.Set("state", "all")
		params.Set("since", entry.Start.Format(time.RFC3339))
		params.Set("before", entry.Stop.Format(time.RFC3339))
		params.Set("created", "true")
		params.Set("assigned", "true")
		params.Set("review_requested", "true")
		params.Set("reviewed", "true")
		params.Set("limit", strconv.Itoa(forgejoPageSize))
		params.Set("page", strconv.Itoa(page))

		data, err := forgejoRequest("repos/issues/search?" + params.Encode())
		if err != nil {
			return nil, err
		}
		var items []forgejoIssue
		if err := json.Unmarshal(data, &items); err != nil {
			return nil, err
		}
		if len(items) == 0 {
			break
		}
		for _, item := range items {
			if allowed[strings.ToLower(item.Repository.FullName)] {
				result = append(result, item)
			}
		}
		if len(items) < forgejoPageSize {
			break
		}
		if page == forgejoMaxPages {
			fmt.Fprintf(os.Stderr, "Warning: Forgejo issue search truncated at %d pages\n", forgejoMaxPages)
		}
	}
	return result, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./src/ -run TestFetchForgejoIssuesFiltersToResolvedRepositories -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add src/main.go src/main_test.go
git commit -m "Fetch Forgejo issues and pull requests"
```

---

### Task 6: Assemble Forgejo activity

**Files:**
- Modify: `src/main.go` (new `ForgejoActivity` type and collector)
- Test: `src/main_test.go`

- [ ] **Step 1: Write the failing test**

Add to `src/main_test.go`:

```go
func TestCollectForgejoActivityBuildsSplitAndPromptViews(t *testing.T) {
	oldCfg, oldRemote := cfg, gitRemoteURL
	defer func() { cfg, gitRemoteURL = oldCfg, oldRemote }()

	entry := TogglTimeEntry{
		Start: time.Date(2026, 6, 9, 10, 0, 0, 0, time.UTC),
		Stop:  time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC),
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/repos/voicesense/voicesense-backend/commits":
			json.NewEncoder(w).Encode([]map[string]any{{
				"sha": "a",
				"commit": map[string]any{
					"message":   "feat: quota checks",
					"author":    map[string]any{"name": "Juda Kaleta", "email": "juda@example.com", "date": "2026-06-09T10:30:00Z"},
					"committer": map[string]any{"name": "Juda Kaleta", "email": "juda@example.com", "date": "2026-06-09T10:30:00Z"},
				},
				"author": map[string]any{"login": "juda"},
			}})
		case r.URL.Path == "/api/v1/repos/issues/search":
			json.NewEncoder(w).Encode([]map[string]any{
				{
					"number":       12,
					"title":        "Persist quota decisions",
					"updated_at":   "2026-06-09T11:00:00Z",
					"pull_request": map[string]any{"merged": false},
					"repository":   map[string]any{"full_name": "voicesense/voicesense-backend"},
				},
				{
					"number":     45,
					"title":      "Retry backoff",
					"updated_at": "2026-06-09T11:30:00Z",
					"repository": map[string]any{"full_name": "voicesense/voicesense-backend"},
				},
			})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	cfg = Config{}
	cfg.Git.User = `Juda Kaleta\|juda@example.com\|Forgejo Actions`
	cfg.Forgejo.URL = server.URL
	cfg.Forgejo.APIKey = "token"
	cfg.Projects = map[string]ProjectConfig{
		"voicesense": {ProjectID: 204198137, Repositories: []string{"/repos/voicesense-backend"}},
	}
	gitRemoteURL = func(repo string) (string, error) {
		return "https://127.0.0.1/voicesense/voicesense-backend.git", nil
	}

	activity := collectForgejoActivity(entry)

	if len(activity.SplitCommits) != 3 {
		t.Fatalf("SplitCommits = %d, want 3 (1 commit + 1 PR + 1 issue): %+v", len(activity.SplitCommits), activity.SplitCommits)
	}
	for _, commit := range activity.SplitCommits {
		if commit.ProjectID != 204198137 {
			t.Fatalf("split commit missing project mapping: %+v", commit)
		}
	}

	joined := strings.Join(activity.PromptSections, "\n")
	for _, want := range []string{"Forgejo commits:", "feat: quota checks", "Forgejo pull requests:", "[PR #12] Persist quota decisions", "Forgejo issues:", "[Issue #45] Retry backoff"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("PromptSections missing %q: %q", want, joined)
		}
	}
}

func TestCollectForgejoActivityDisabledMakesNoRequests(t *testing.T) {
	oldCfg := cfg
	defer func() { cfg = oldCfg }()
	cfg = Config{}

	activity := collectForgejoActivity(TogglTimeEntry{})
	if len(activity.SplitCommits) != 0 || len(activity.PromptSections) != 0 {
		t.Fatalf("disabled integration should return empty activity: %+v", activity)
	}
}
```

> Note: the `gitRemoteURL` stub returns a `127.0.0.1` remote so `resolveForgejoRepositories` recognises `voicesense/voicesense-backend`, while the actual API requests go to the httptest server. `forgejoHost()` strips the port, so the server's host (`127.0.0.1`) matches the stub's host.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./src/ -run 'TestCollectForgejoActivity' -v`
Expected: FAIL — `collectForgejoActivity` / `ForgejoActivity` undefined.

- [ ] **Step 3: Write minimal implementation**

In `src/main.go`, add after `fetchForgejoIssues`:

```go
type ForgejoActivity struct {
	SplitCommits   []ProjectCommit
	PromptSections []string
}

func collectForgejoActivity(entry TogglTimeEntry) ForgejoActivity {
	if !cfg.Forgejo.Enabled() {
		return ForgejoActivity{}
	}
	repos := resolveForgejoRepositories()
	if len(repos) == 0 {
		return ForgejoActivity{}
	}

	byName := map[string]ForgejoRepo{}
	for _, repo := range repos {
		byName[strings.ToLower(repo.FullName())] = repo
	}

	var activity ForgejoActivity
	var commitLines []string
	for _, repo := range repos {
		commits, err := fetchForgejoCommits(entry, repo)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: skipping Forgejo repository %s (%v)\n", repo.FullName(), err)
			continue
		}
		for _, commit := range commits {
			commitLines = append(commitLines, fmt.Sprintf("Forgejo repository: %s\n%s", repo.FullName(), commit.Subject))
			if commit.ProjectID != 0 {
				activity.SplitCommits = append(activity.SplitCommits, commit)
			}
		}
	}
	if len(commitLines) > 0 {
		activity.PromptSections = append(activity.PromptSections, "Forgejo commits:\n"+strings.Join(commitLines, "\n\n"))
	}

	issues, err := fetchForgejoIssues(entry, repos)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: skipping Forgejo issues and pull requests (%v)\n", err)
		return activity
	}

	var issueLines, pullLines []string
	for _, item := range issues {
		label := fmt.Sprintf("Issue #%d", item.Number)
		if item.PullRequest != nil {
			label = fmt.Sprintf("PR #%d", item.Number)
		}
		line := fmt.Sprintf("[%s] %s", label, item.Title)
		if item.PullRequest != nil {
			pullLines = append(pullLines, line)
		} else {
			issueLines = append(issueLines, line)
		}
		if repo, ok := byName[strings.ToLower(item.Repository.FullName)]; ok && repo.ProjectID != 0 {
			activity.SplitCommits = append(activity.SplitCommits, ProjectCommit{
				ProjectName: repo.ProjectName,
				ProjectID:   repo.ProjectID,
				RepoName:    repo.Name,
				Subject:     line,
				Time:        item.UpdatedAt,
			})
		}
	}
	if len(pullLines) > 0 {
		activity.PromptSections = append(activity.PromptSections, "Forgejo pull requests:\n"+strings.Join(pullLines, "\n"))
	}
	if len(issueLines) > 0 {
		activity.PromptSections = append(activity.PromptSections, "Forgejo issues:\n"+strings.Join(issueLines, "\n"))
	}
	return activity
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./src/ -run 'TestCollectForgejoActivity' -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add src/main.go src/main_test.go
git commit -m "Assemble Forgejo activity for summaries"
```

---

### Task 7: Prompt sections for non-split callers

**Files:**
- Modify: `src/main.go` (`buildPromptText` ~line 786-795; `fillEmptyDescriptionsCmd` ~line 970; `repairSummariesCmd` ~line 1237)
- Test: `src/main_test.go`

- [ ] **Step 1: Write the failing test**

Add to `src/main_test.go`:

```go
func TestBuildPromptTextWithSectionsOrdering(t *testing.T) {
	text := buildPromptTextWithSections(
		[]string{"Repository: app\nfeat: local work"},
		[]string{"Forgejo commits:\nForgejo repository: o/r\nfeat: remote work"},
		"worked on things",
	)
	for _, want := range []string{"Git commits:", "feat: local work", "Forgejo commits:", "feat: remote work", "Work log:", "worked on things"} {
		if !strings.Contains(text, want) {
			t.Fatalf("prompt missing %q: %q", want, text)
		}
	}
	if strings.Index(text, "Forgejo commits:") < strings.Index(text, "Git commits:") {
		t.Fatalf("Forgejo section should follow local commits: %q", text)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./src/ -run TestBuildPromptTextWithSectionsOrdering -v`
Expected: FAIL — `buildPromptTextWithSections undefined`.

- [ ] **Step 3: Write minimal implementation**

In `src/main.go`, replace `buildPromptText` with:

```go
func buildPromptText(commits []string, worklog string) string {
	return buildPromptTextWithSections(commits, nil, worklog)
}

func buildPromptTextWithSections(commits []string, extraSections []string, worklog string) string {
	var sections []string
	if len(commits) > 0 {
		sections = append(sections, fmt.Sprintf("Git commits:\n%s", strings.Join(commits, "\n\n")))
	}
	sections = append(sections, extraSections...)
	if strings.TrimSpace(worklog) != "" {
		sections = append(sections, fmt.Sprintf("Work log:\n%s", strings.TrimSpace(worklog)))
	}
	return strings.Join(sections, "\n\n")
}
```

In `fillEmptyDescriptionsCmd`, change the loop body:

```go
			commits := collectCommits(entry)
			activity := collectForgejoActivity(entry)
			promptText := buildPromptTextWithSections(commits, activity.PromptSections, "")
```

In `repairSummariesCmd`, change:

```go
				commits := collectCommits(entry)
				activity := collectForgejoActivity(entry)
				promptText := buildPromptTextWithSections(commits, activity.PromptSections, "")
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./src/ -run 'TestBuildPromptTextWithSectionsOrdering|TestFillEmptyDescriptions' -v`
Expected: PASS (existing fill-empty tests still pass because Forgejo is disabled in them).

- [ ] **Step 5: Commit**

```bash
git add src/main.go src/main_test.go
git commit -m "Include Forgejo sections in non-split prompts"
```

---

### Task 8: Wire Forgejo into `stop`

**Files:**
- Modify: `src/main.go` (`stopCmd` ~line 1112-1189; add `filterMappedCommits` near `filterProjectCommits`)
- Test: `src/main_test.go`

- [ ] **Step 1: Write the failing test**

Add to `src/main_test.go`:

```go
func TestFilterMappedCommitsDropsUnmapped(t *testing.T) {
	commits := []ProjectCommit{
		{ProjectName: "a", ProjectID: 1, Subject: "kept"},
		{ProjectName: "", ProjectID: 0, Subject: "dropped"},
	}
	filtered := filterMappedCommits(commits)
	if len(filtered) != 1 || filtered[0].Subject != "kept" {
		t.Fatalf("filtered = %+v, want only mapped commit", filtered)
	}
}

func TestStopCommandIncludesForgejoActivityInPrompt(t *testing.T) {
	oldCfg, oldTogglBase, oldOpenAIBase, oldNow, oldRemote := cfg, togglBaseURL, openAIBaseURL, nowFunc, gitRemoteURL
	defer func() {
		cfg, togglBaseURL, openAIBaseURL, nowFunc, gitRemoteURL = oldCfg, oldTogglBase, oldOpenAIBase, oldNow, oldRemote
	}()

	nowFunc = func() time.Time { return time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC) }
	cfg = Config{}
	cfg.Toggl.WorkspaceID = 123
	cfg.OpenAI.Model = "test"
	cfg.Git.User = `Juda Kaleta\|juda@example.com`
	cfg.Forgejo.APIKey = "token"
	cfg.Forgejo.Repositories = []string{"voicesense/voicesense-backend"}
	cfg.Repositories = nil

	var capturedPrompt string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/api/me/time_entries/current":
			json.NewEncoder(w).Encode(TogglTimeEntry{ID: 99, Workspace: 123, Project: 10, Start: time.Date(2026, 6, 9, 10, 0, 0, 0, time.UTC)})
		case r.Method == "PUT" && r.URL.Path == "/api/workspaces/123/time_entries/99":
			w.WriteHeader(http.StatusOK)
		case r.Method == "POST" && r.URL.Path == "/openai":
			var request struct {
				Messages []struct {
					Role    string `json:"role"`
					Content string `json:"content"`
				} `json:"messages"`
			}
			json.NewDecoder(r.Body).Decode(&request)
			for _, message := range request.Messages {
				if message.Role == "user" {
					capturedPrompt = message.Content
				}
			}
			json.NewEncoder(w).Encode(map[string]any{"choices": []map[string]any{{"message": map[string]string{"content": "Did remote work"}}}})
		case r.Method == "GET" && r.URL.Path == "/api/v1/repos/voicesense/voicesense-backend/commits":
			json.NewEncoder(w).Encode([]map[string]any{{
				"sha": "a",
				"commit": map[string]any{
					"message":   "feat: remote quota checks",
					"author":    map[string]any{"name": "Juda Kaleta", "email": "juda@example.com", "date": "2026-06-09T10:30:00Z"},
					"committer": map[string]any{"name": "Juda Kaleta", "email": "juda@example.com", "date": "2026-06-09T10:30:00Z"},
				},
				"author": map[string]any{"login": "juda"},
			}})
		case r.Method == "GET" && r.URL.Path == "/api/v1/repos/issues/search":
			json.NewEncoder(w).Encode([]map[string]any{})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	togglBaseURL = server.URL + "/api/"
	openAIBaseURL = server.URL + "/openai"
	cfg.Forgejo.URL = server.URL

	_, err := captureStdout(func() error { return stopCmd().RunE(stopCmd(), nil) })
	if err != nil {
		t.Fatalf("stop command returned error: %v", err)
	}
	if !strings.Contains(capturedPrompt, "Forgejo commits:") || !strings.Contains(capturedPrompt, "feat: remote quota checks") {
		t.Fatalf("prompt missing Forgejo activity: %q", capturedPrompt)
	}
}
```

> Note: `stopCmd` reads stdin only when summarization fails, so no stdin stub is required here.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./src/ -run 'TestFilterMappedCommitsDropsUnmapped|TestStopCommandIncludesForgejoActivityInPrompt' -v`
Expected: FAIL — `filterMappedCommits undefined`.

- [ ] **Step 3: Write minimal implementation**

In `src/main.go`, add after `filterProjectCommits`:

```go
func filterMappedCommits(commits []ProjectCommit) []ProjectCommit {
	var filtered []ProjectCommit
	for _, commit := range commits {
		if commit.ProjectID != 0 {
			filtered = append(filtered, commit)
		}
	}
	return filtered
}
```

In `stopCmd`, after the calendar block and before the project-commit block, add:

```go
			activity := collectForgejoActivity(entry)
```

Change the split block to merge and filter:

```go
			if len(cfg.Projects) > 0 {
				combined := append(append([]ProjectCommit(nil), projectCommits...), activity.SplitCommits...)
				splits := calculateProjectSplits(entry, filterMappedCommits(combined))
```

(This replaces the existing `splits := calculateProjectSplits(entry, projectCommits)` line. The surrounding `if len(splits) > 0 { ... }` block is unchanged.)

Change the non-split prompt construction to:

```go
			commits := collectCommits(entry)
			promptText := buildPromptTextWithSections(commits, activity.PromptSections, worklog)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./src/ -run 'TestFilterMappedCommitsDropsUnmapped|TestStopCommand' -v`
Expected: PASS (including the pre-existing stop command tests).

- [ ] **Step 5: Commit**

```bash
git add src/main.go src/main_test.go
git commit -m "Wire Forgejo activity into stop summaries"
```

---

### Task 9: Documentation and OpenSpec change

**Files:**
- Modify: `README.md`
- Create: `openspec/changes/add-forgejo-activity/proposal.md`
- Create: `openspec/changes/add-forgejo-activity/tasks.md`
- Create: `openspec/changes/add-forgejo-activity/specs/forgejo-activity/spec.md`

- [ ] **Step 1: Update the README**

In `README.md`, add a bullet under Features and a configuration subsection. Use this text:

```markdown
* **Forgejo activity**: when `forgejo` is configured, `stop`, `fill-empty-descriptions`, and `repair-summaries` include your commits, pull requests, and issues from the Senseloom Forgejo server in the generated summary.
```

```markdown
### Forgejo integration (optional)

Add your Forgejo server and a personal access token to `~/.toggl.yaml`:

```yaml
forgejo:
  url: https://infra.senseloom.com
  api_key: <personal-access-token>   # or set FORGEJO_API_KEY
  repositories:                      # optional, owner/repo, for repos without a local clone
    - voicesense/voicesense-backend
```

Repositories are discovered automatically from the Forgejo remotes of the paths already listed in `repositories` and `projects.<name>.repositories`. A repository listed only under a project inherits that project's Toggl project mapping and participates in time splitting; repositories without a mapping contribute to the summary text only. Forgejo errors are reported as warnings and never fail `toggl stop`.
```

- [ ] **Step 2: Create the OpenSpec proposal**

Create `openspec/changes/add-forgejo-activity/proposal.md`:

```markdown
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
```

- [ ] **Step 3: Create the OpenSpec spec**

Create `openspec/changes/add-forgejo-activity/specs/forgejo-activity/spec.md`:

```markdown
# forgejo-activity Specification

## Purpose
Collect the authenticated user's commits, pull requests, and issues from a configured Forgejo server for a Toggl session window and make them available to summary generation and project time splitting.

## Requirements
### Requirement: Optional Forgejo configuration
The CLI SHALL integrate with Forgejo only when `forgejo.url` and `forgejo.api_key` are both configured.

#### Scenario: Integration disabled
- **WHEN** `forgejo.url` or `forgejo.api_key` is empty
- **THEN** the CLI makes no Forgejo requests and summary behavior is unchanged

#### Scenario: Environment override
- **WHEN** `FORGEJO_API_KEY` is set
- **THEN** the CLI uses it in place of `forgejo.api_key`

### Requirement: Resolve Forgejo repositories
The CLI SHALL resolve the Forgejo repositories to scan from `forgejo.repositories` and from the Forgejo remotes of paths in `repositories` and `projects.<name>.repositories`.

#### Scenario: Local repository points at the Forgejo server
- **WHEN** `git -C <path> remote get-url origin` resolves to the configured Forgejo host
- **THEN** the CLI includes that `owner/repo` and, when discovered under `projects.<name>`, applies that project's mapping

#### Scenario: Repository has no project mapping
- **WHEN** a resolved repository has no `projects.<name>` mapping
- **THEN** its activity contributes to summary text only and is excluded from time splitting

### Requirement: Collect Forgejo commits
The CLI SHALL collect commits authored by the configured `git.user` identities whose committer timestamp falls within the session window.

#### Scenario: Matching commit exists
- **WHEN** a resolved repository contains a commit in the window whose author name, email, or login matches a `git.user` token
- **THEN** the CLI includes its first message line as a Forgejo commit

### Requirement: Collect Forgejo pull requests and issues
The CLI SHALL collect pull requests and issues the user created, is assigned, was requested to review, or reviewed, updated within the session window.

#### Scenario: Pull request activity
- **WHEN** the search returns a pull request in a resolved repository updated in the window
- **THEN** the CLI includes it as a Forgejo pull request

#### Scenario: Issue activity
- **WHEN** the search returns an issue in a resolved repository updated in the window
- **THEN** the CLI includes it as a Forgejo issue

### Requirement: Tolerate Forgejo failures
The CLI SHALL continue summary generation when Forgejo requests fail.

#### Scenario: API error
- **WHEN** a Forgejo request returns a non-success status or transport error
- **THEN** the CLI writes a warning to stderr and continues with locally collected activity
```

- [ ] **Step 4: Create the OpenSpec tasks file**

Create `openspec/changes/add-forgejo-activity/tasks.md`:

```markdown
## 1. Configuration

- [ ] 1.1 Add `forgejo` config section with `url`, `api_key`, `repositories`
- [ ] 1.2 Support `FORGEJO_API_KEY` override

## 2. Collection

- [ ] 2.1 Parse Forgejo git remotes into `owner/repo`
- [ ] 2.2 Resolve repositories from config and local remotes with project mapping
- [ ] 2.3 Fetch and filter commits by author and window
- [ ] 2.4 Fetch and filter pull requests and issues
- [ ] 2.5 Assemble split and prompt activity views

## 3. Integration

- [ ] 3.1 Include Forgejo sections in `fill-empty-descriptions` and `repair-summaries` prompts
- [ ] 3.2 Merge mapped Forgejo activity into `stop` time splitting
- [ ] 3.3 Include Forgejo sections in the `stop` non-split prompt

## 4. Verification

- [ ] 4.1 Unit tests for parsing, resolution, collection, and integration
- [ ] 4.2 `make fmt`, `make test`, `make build`
- [ ] 4.3 Update README
```

- [ ] **Step 5: Commit**

```bash
git add README.md openspec/changes/add-forgejo-activity
git commit -m "Document Forgejo activity integration"
```

---

### Task 10: Final verification

- [ ] **Step 1: Format**

Run: `make fmt`
Expected: no output; no changes reported.

- [ ] **Step 2: Run the full test suite**

Run: `make test`
Expected: `ok  	judakaleta.cz/toggl/v2/src`

- [ ] **Step 3: Build**

Run: `make build`
Expected: `Building toggl <version>...` and no errors.

- [ ] **Step 4: Confirm a clean tree**

Run: `git status -sb`
Expected: branch shows no uncommitted changes.

- [ ] **Step 5: Verify against the spec**

Confirm each requirement in `docs/superpowers/specs/2026-09-22-forgejo-activity-design.md` maps to implemented code and a passing test. Record any gap as a follow-up task instead of leaving it implicit.
