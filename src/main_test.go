package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// --- Test helpers ---

func setupTogglServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(handler)
	old := togglBaseURL
	togglBaseURL = srv.URL + "/"
	t.Cleanup(func() {
		togglBaseURL = old
		srv.Close()
	})
	return srv
}

func setupOpenAIServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(handler)
	old := openAIBaseURL
	openAIBaseURL = srv.URL
	t.Cleanup(func() {
		openAIBaseURL = old
		srv.Close()
	})
	return srv
}

func setCfg(t *testing.T, c Config) {
	t.Helper()
	old := cfg
	cfg = c
	t.Cleanup(func() { cfg = old })
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	gitEnv(t, dir, nil, args...)
}

func gitEnv(t *testing.T, dir string, env []string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func mustTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("parse time %q: %v", value, err)
	}
	return parsed
}

// --- TestLoadConfig ---

func TestLoadConfig_Valid(t *testing.T) {
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
repositories:
  - "/tmp/repo1"
  - "/tmp/repo2"
`
	var c Config
	if err := yaml.Unmarshal([]byte(content), &c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Toggl.APIKey != "test-key" {
		t.Errorf("expected api_key 'test-key', got %q", c.Toggl.APIKey)
	}
	if c.Toggl.WorkspaceID != 123 {
		t.Errorf("expected workspace_id 123, got %d", c.Toggl.WorkspaceID)
	}
	if c.Toggl.ProjectID != 456 {
		t.Errorf("expected project_id 456, got %d", c.Toggl.ProjectID)
	}
	if c.OpenAI.APIKey != "sk-test" {
		t.Errorf("expected openai api_key 'sk-test', got %q", c.OpenAI.APIKey)
	}
	if c.OpenAI.Model != "gpt-4" {
		t.Errorf("expected model 'gpt-4', got %q", c.OpenAI.Model)
	}
	if c.Git.User != "testuser" {
		t.Errorf("expected git user 'testuser', got %q", c.Git.User)
	}
	if len(c.Repositories) != 2 {
		t.Errorf("expected 2 repositories, got %d", len(c.Repositories))
	}
}

func TestLoadConfig_ProjectGroups(t *testing.T) {
	content := `
projects:
  cortex:
    project_id: 123
    repositories:
      - "~/Projects/lkq/cortex"
  voicesense:
    project_id: 456
    repositories:
      - "~/Projects/lkq/voicesense"
`
	var c Config
	if err := yaml.Unmarshal([]byte(content), &c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cortex := c.Projects["cortex"]
	if cortex.ProjectID != 123 {
		t.Errorf("expected cortex project_id 123, got %d", cortex.ProjectID)
	}
	if len(cortex.Repositories) != 1 || cortex.Repositories[0] != "~/Projects/lkq/cortex" {
		t.Errorf("unexpected cortex repositories: %#v", cortex.Repositories)
	}

	voicesense := c.Projects["voicesense"]
	if voicesense.ProjectID != 456 {
		t.Errorf("expected voicesense project_id 456, got %d", voicesense.ProjectID)
	}
	if len(voicesense.Repositories) != 1 || voicesense.Repositories[0] != "~/Projects/lkq/voicesense" {
		t.Errorf("unexpected voicesense repositories: %#v", voicesense.Repositories)
	}
}

func TestLoadConfig_MissingFile(t *testing.T) {
	data, err := os.ReadFile("/tmp/nonexistent-toggl-config-12345.yaml")
	if err == nil {
		t.Fatalf("expected error reading missing file, got data: %s", data)
	}
}

func TestLoadConfig_InvalidYAML(t *testing.T) {
	content := `invalid: yaml: [broken`
	var c Config
	err := yaml.Unmarshal([]byte(content), &c)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestLoadConfig_UsesHomeEnvironment(t *testing.T) {
	oldCfg := cfg
	t.Cleanup(func() { cfg = oldCfg })

	home := t.TempDir()
	t.Setenv("HOME", home)
	content := `
toggl:
  api_key: "test-key"
  workspace_id: 123
openai:
  api_key: "sk-test"
  model: "gpt-4"
git:
  user: "env-home-user"
repositories: []
`
	if err := os.WriteFile(filepath.Join(home, ".toggl.yaml"), []byte(content), 0644); err != nil {
		t.Fatalf("write temp config: %v", err)
	}

	if err := loadConfig(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Git.User != "env-home-user" {
		t.Fatalf("expected config from HOME, got git.user %q", cfg.Git.User)
	}
}

func TestExpandRepoPath_ExpandsHomeTilde(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	got := expandRepoPath("~/Projects/lkq/cortex")
	want := filepath.Join(home, "Projects", "lkq", "cortex")
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}

	got = expandRepoPath("~")
	if got != home {
		t.Fatalf("expected %q, got %q", home, got)
	}
}

func TestCollectProjectCommits_CollectsProjectMetadata(t *testing.T) {
	setCfg(t, Config{})
	cortexRepo := t.TempDir()
	voicesenseRepo := t.TempDir()

	for _, repo := range []string{cortexRepo, voicesenseRepo} {
		git(t, repo, "init")
		git(t, repo, "config", "user.name", "Test User")
		git(t, repo, "config", "user.email", "test@example.com")
	}

	if err := os.WriteFile(filepath.Join(cortexRepo, "file.txt"), []byte("cortex"), 0644); err != nil {
		t.Fatalf("write cortex file: %v", err)
	}
	git(t, cortexRepo, "add", "file.txt")
	gitEnv(t, cortexRepo, []string{
		"GIT_AUTHOR_DATE=2024-01-01T09:30:00Z",
		"GIT_COMMITTER_DATE=2024-01-01T09:30:00Z",
	}, "commit", "-m", "Build cortex flow")

	if err := os.WriteFile(filepath.Join(voicesenseRepo, "file.txt"), []byte("voicesense"), 0644); err != nil {
		t.Fatalf("write voicesense file: %v", err)
	}
	git(t, voicesenseRepo, "add", "file.txt")
	gitEnv(t, voicesenseRepo, []string{
		"GIT_AUTHOR_DATE=2024-01-01T09:30:00Z",
		"GIT_COMMITTER_DATE=2024-01-01T09:30:00Z",
	}, "commit", "-m", "Improve voicesense export")

	cfg.Git.User = "Test User"
	cfg.Projects = map[string]ProjectConfig{
		"cortex": {
			ProjectID:    111,
			Repositories: []string{cortexRepo},
		},
		"voicesense": {
			ProjectID:    222,
			Repositories: []string{voicesenseRepo},
		},
	}

	entry := TogglTimeEntry{
		Start: mustTime(t, "2024-01-01T09:00:00Z"),
		Stop:  mustTime(t, "2024-01-01T11:00:00Z"),
	}

	commits := collectProjectCommits(entry)
	if len(commits) != 2 {
		t.Fatalf("expected 2 commits, got %d: %#v", len(commits), commits)
	}

	expected := []ProjectCommit{
		{
			ProjectName: "cortex",
			ProjectID:   111,
			RepoName:    filepath.Base(cortexRepo),
			RepoPath:    cortexRepo,
			Subject:     "Build cortex flow",
			Time:        mustTime(t, "2024-01-01T09:30:00Z"),
		},
		{
			ProjectName: "voicesense",
			ProjectID:   222,
			RepoName:    filepath.Base(voicesenseRepo),
			RepoPath:    voicesenseRepo,
			Subject:     "Improve voicesense export",
			Time:        mustTime(t, "2024-01-01T09:30:00Z"),
		},
	}

	for i, want := range expected {
		got := commits[i]
		if got.ProjectName != want.ProjectName || got.ProjectID != want.ProjectID || got.RepoName != want.RepoName || got.RepoPath != want.RepoPath || got.Subject != want.Subject || !got.Time.Equal(want.Time) {
			t.Fatalf("commit %d mismatch\nwant: %#v\n got: %#v", i, want, got)
		}
	}
}

func TestCalculateProjectSplits_MidpointAllocationProducesContiguousRanges(t *testing.T) {
	entry := TogglTimeEntry{
		Start: mustTime(t, "2024-01-01T09:00:00Z"),
		Stop:  mustTime(t, "2024-01-01T13:00:00Z"),
	}
	commits := []ProjectCommit{
		{ProjectName: "cortex", ProjectID: 111, Time: mustTime(t, "2024-01-01T09:30:00Z")},
		{ProjectName: "voicesense", ProjectID: 222, Time: mustTime(t, "2024-01-01T11:30:00Z")},
		{ProjectName: "voicesense", ProjectID: 222, Time: mustTime(t, "2024-01-01T12:30:00Z")},
	}

	splits := calculateProjectSplits(entry, commits)
	if len(splits) != 2 {
		t.Fatalf("expected 2 splits, got %d: %#v", len(splits), splits)
	}

	expected := []ProjectSplit{
		{
			ProjectName: "cortex",
			ProjectID:   111,
			Start:       mustTime(t, "2024-01-01T09:00:00Z"),
			Stop:        mustTime(t, "2024-01-01T10:30:00Z"),
			Duration:    90 * time.Minute,
		},
		{
			ProjectName: "voicesense",
			ProjectID:   222,
			Start:       mustTime(t, "2024-01-01T10:30:00Z"),
			Stop:        mustTime(t, "2024-01-01T13:00:00Z"),
			Duration:    150 * time.Minute,
		},
	}

	for i, want := range expected {
		got := splits[i]
		if got.ProjectName != want.ProjectName || got.ProjectID != want.ProjectID || !got.Start.Equal(want.Start) || !got.Stop.Equal(want.Stop) || got.Duration != want.Duration {
			t.Fatalf("split %d mismatch\nwant: %#v\n got: %#v", i, want, got)
		}
	}
	if !splits[0].Start.Equal(entry.Start) {
		t.Fatalf("first split starts at %s, want %s", splits[0].Start, entry.Start)
	}
	for i := 0; i < len(splits)-1; i++ {
		if !splits[i].Stop.Equal(splits[i+1].Start) {
			t.Fatalf("gap or overlap between split %d and %d: %s != %s", i, i+1, splits[i].Stop, splits[i+1].Start)
		}
	}
	if !splits[len(splits)-1].Stop.Equal(entry.Stop) {
		t.Fatalf("last split stops at %s, want %s", splits[len(splits)-1].Stop, entry.Stop)
	}
}

func TestCalculateProjectSplits_AggregatesAlternatingProjectOwnership(t *testing.T) {
	entry := TogglTimeEntry{
		Start: mustTime(t, "2024-01-01T09:00:00Z"),
		Stop:  mustTime(t, "2024-01-01T13:00:00Z"),
	}
	commits := []ProjectCommit{
		{ProjectName: "cortex", ProjectID: 111, Time: mustTime(t, "2024-01-01T09:30:00Z")},
		{ProjectName: "voicesense", ProjectID: 222, Time: mustTime(t, "2024-01-01T10:30:00Z")},
		{ProjectName: "cortex", ProjectID: 111, Time: mustTime(t, "2024-01-01T11:30:00Z")},
	}

	splits := calculateProjectSplits(entry, commits)
	expected := []ProjectSplit{
		{
			ProjectName: "cortex",
			ProjectID:   111,
			Start:       mustTime(t, "2024-01-01T09:00:00Z"),
			Stop:        mustTime(t, "2024-01-01T12:00:00Z"),
			Duration:    3 * time.Hour,
		},
		{
			ProjectName: "voicesense",
			ProjectID:   222,
			Start:       mustTime(t, "2024-01-01T12:00:00Z"),
			Stop:        mustTime(t, "2024-01-01T13:00:00Z"),
			Duration:    time.Hour,
		},
	}

	if len(splits) != len(expected) {
		t.Fatalf("expected %d splits, got %d: %#v", len(expected), len(splits), splits)
	}
	for i, want := range expected {
		got := splits[i]
		if got.ProjectName != want.ProjectName || got.ProjectID != want.ProjectID || !got.Start.Equal(want.Start) || !got.Stop.Equal(want.Stop) || got.Duration != want.Duration {
			t.Fatalf("split %d mismatch\nwant: %#v\n got: %#v", i, want, got)
		}
	}
}

func TestCreateTimeEntry_PostsStoppedEntry(t *testing.T) {
	setCfg(t, Config{})
	cfg.Toggl.APIKey = "key"

	var method, path string
	var body []byte
	setupTogglServer(t, func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		path = r.URL.Path
		var err error
		body, err = io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		w.Write([]byte(`{"id":1}`))
	})

	start := mustTime(t, "2024-01-01T09:00:00Z")
	stop := mustTime(t, "2024-01-01T10:30:00Z")
	split := ProjectSplit{
		ProjectName: "voicesense",
		ProjectID:   222,
		Start:       start,
		Stop:        stop,
		Duration:    90 * time.Minute,
	}

	if err := createTimeEntry(split, "Session summary", 100); err != nil {
		t.Fatalf("createTimeEntry returned error: %v", err)
	}

	if method != "POST" {
		t.Fatalf("expected POST, got %s", method)
	}
	if !strings.Contains(path, "/workspaces/100/time_entries") {
		t.Fatalf("expected time_entries path for workspace 100, got %s", path)
	}

	var got struct {
		WorkspaceID int    `json:"workspace_id"`
		ProjectID   int    `json:"project_id"`
		Description string `json:"description"`
		CreatedWith string `json:"created_with"`
		Start       string `json:"start"`
		Stop        string `json:"stop"`
		Duration    int    `json:"duration"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode request body %s: %v", body, err)
	}
	if got.WorkspaceID != 100 {
		t.Errorf("expected workspace_id 100, got %d", got.WorkspaceID)
	}
	if got.ProjectID != 222 {
		t.Errorf("expected project_id 222, got %d", got.ProjectID)
	}
	if got.Description != "Session summary" {
		t.Errorf("expected description %q, got %q", "Session summary", got.Description)
	}
	if got.CreatedWith != "toggl-cli" {
		t.Errorf("expected created_with toggl-cli, got %q", got.CreatedWith)
	}
	if got.Start != start.Format(time.RFC3339) {
		t.Errorf("expected start %s, got %s", start.Format(time.RFC3339), got.Start)
	}
	if got.Stop != stop.Format(time.RFC3339) {
		t.Errorf("expected stop %s, got %s", stop.Format(time.RFC3339), got.Stop)
	}
	if got.Duration != int((90 * time.Minute).Seconds()) {
		t.Errorf("expected duration 5400 seconds, got %d", got.Duration)
	}
	if got.Duration <= 0 {
		t.Errorf("expected positive duration, got %d", got.Duration)
	}
}

// --- TestTogglRequest ---

func TestTogglRequest_GET(t *testing.T) {
	setCfg(t, Config{})
	cfg.Toggl.APIKey = "test-api-key"

	setupTogglServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/me" {
			t.Errorf("expected path /me, got %s", r.URL.Path)
		}
		user, pass, ok := r.BasicAuth()
		if !ok || user != "test-api-key" || pass != "api_token" {
			t.Errorf("expected Basic Auth with test-api-key:api_token, got %s:%s", user, pass)
		}
		w.Write([]byte(`{"data":"ok"}`))
	})

	data, err := togglRequest("GET", "me", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != `{"data":"ok"}` {
		t.Errorf("unexpected body: %s", data)
	}
}

func TestTogglRequest_POST(t *testing.T) {
	setCfg(t, Config{})
	cfg.Toggl.APIKey = "key"

	setupTogglServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != `{"hello":"world"}` {
			t.Errorf("unexpected body: %s", body)
		}
		w.Write([]byte(`{"ok":true}`))
	})

	data, err := togglRequest("POST", "test", strings.NewReader(`{"hello":"world"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != `{"ok":true}` {
		t.Errorf("unexpected response: %s", data)
	}
}

func TestTogglRequest_ServerError(t *testing.T) {
	setCfg(t, Config{})
	cfg.Toggl.APIKey = "key"

	setupTogglServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte("server error"))
	})

	data, err := togglRequest("GET", "fail", nil)
	if err != nil {
		t.Fatalf("unexpected transport error: %v", err)
	}
	// togglRequest doesn't check status codes, just returns the body
	if string(data) != "server error" {
		t.Errorf("unexpected body: %s", data)
	}
}

// --- TestOpenAISummarize ---

func TestOpenAISummarize_Success(t *testing.T) {
	setCfg(t, Config{})
	cfg.OpenAI.APIKey = "sk-test"
	cfg.OpenAI.Model = "gpt-4"

	setupOpenAIServer(t, func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer sk-test" {
			t.Errorf("expected Bearer sk-test, got %s", auth)
		}
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]string{"content": "Test summary"}},
			},
		})
	})

	result, err := openAISummarize("some commits")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Test summary" {
		t.Errorf("expected 'Test summary', got %q", result)
	}
}

func TestOpenAISummarize_Unauthorized(t *testing.T) {
	setCfg(t, Config{})
	cfg.OpenAI.APIKey = "bad-key"
	cfg.OpenAI.Model = "gpt-4"

	setupOpenAIServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
	})

	_, err := openAISummarize("text")
	if err == nil {
		t.Fatal("expected error for 401")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("expected error to mention 401, got: %v", err)
	}
}

func TestOpenAISummarize_EmptyChoices(t *testing.T) {
	setCfg(t, Config{})
	cfg.OpenAI.APIKey = "sk-test"
	cfg.OpenAI.Model = "gpt-4"

	setupOpenAIServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`{"choices":[]}`))
	})

	_, err := openAISummarize("text")
	if err == nil {
		t.Fatal("expected error for empty choices")
	}
	if !strings.Contains(err.Error(), "no completion returned") {
		t.Errorf("expected 'no completion returned', got: %v", err)
	}
}

func TestOpenAISummarize_MalformedJSON(t *testing.T) {
	setCfg(t, Config{})
	cfg.OpenAI.APIKey = "sk-test"
	cfg.OpenAI.Model = "gpt-4"

	setupOpenAIServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`{not json`))
	})

	_, err := openAISummarize("text")
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestOpenAISummarize_AuthHeader(t *testing.T) {
	setCfg(t, Config{})
	cfg.OpenAI.APIKey = "sk-my-secret-key"
	cfg.OpenAI.Model = "gpt-4"

	var gotAuth string
	setupOpenAIServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]string{"content": "ok"}},
			},
		})
	})

	openAISummarize("text")
	if gotAuth != "Bearer sk-my-secret-key" {
		t.Errorf("expected 'Bearer sk-my-secret-key', got %q", gotAuth)
	}
}

// --- TestWhoamiCmd ---

func TestWhoamiCmd_ValidJSON(t *testing.T) {
	setCfg(t, Config{})
	cfg.Toggl.APIKey = "key"

	setupTogglServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data":{"id":42,"fullname":"Test User","email":"test@example.com"}}`))
	})

	cmd := whoamiCmd()
	out := &strings.Builder{}
	cmd.SetOut(out)
	cmd.SetErr(out)
	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := cmd.Execute()

	w.Close()
	os.Stdout = oldStdout
	captured, _ := io.ReadAll(r)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := string(captured)
	if !strings.Contains(output, "42") {
		t.Errorf("expected output to contain user ID 42, got: %s", output)
	}
	if !strings.Contains(output, "Test User") {
		t.Errorf("expected output to contain 'Test User', got: %s", output)
	}
	if !strings.Contains(output, "test@example.com") {
		t.Errorf("expected output to contain email, got: %s", output)
	}
}

func TestWhoamiCmd_MalformedJSON(t *testing.T) {
	setCfg(t, Config{})
	cfg.Toggl.APIKey = "key"

	setupTogglServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{not json`))
	})

	cmd := whoamiCmd()
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

// --- TestProjectsCmd ---

func TestProjectsCmd_MultipleProjects(t *testing.T) {
	setCfg(t, Config{})
	cfg.Toggl.APIKey = "key"
	cfg.Toggl.WorkspaceID = 100

	setupTogglServer(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "workspaces/100/projects") {
			t.Errorf("expected path to contain workspaces/100/projects, got %s", r.URL.Path)
		}
		w.Write([]byte(`[{"id":1,"name":"Project A"},{"id":2,"name":"Project B"}]`))
	})

	cmd := projectsCmd()
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := cmd.Execute()

	w.Close()
	os.Stdout = oldStdout
	captured, _ := io.ReadAll(r)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := string(captured)
	if !strings.Contains(output, "Project A") {
		t.Errorf("expected output to contain 'Project A', got: %s", output)
	}
	if !strings.Contains(output, "Project B") {
		t.Errorf("expected output to contain 'Project B', got: %s", output)
	}
	if !strings.Contains(output, "1") || !strings.Contains(output, "2") {
		t.Errorf("expected output to contain project IDs, got: %s", output)
	}
}

func TestProjectsCmd_EmptyList(t *testing.T) {
	setCfg(t, Config{})
	cfg.Toggl.APIKey = "key"
	cfg.Toggl.WorkspaceID = 100

	setupTogglServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[]`))
	})

	cmd := projectsCmd()
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := cmd.Execute()

	w.Close()
	os.Stdout = oldStdout
	captured, _ := io.ReadAll(r)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := string(captured)
	if !strings.Contains(output, "No projects found") {
		t.Errorf("expected 'No projects found', got: %s", output)
	}
}

func TestProjectsCmd_APIError(t *testing.T) {
	setCfg(t, Config{})
	cfg.Toggl.APIKey = "key"
	cfg.Toggl.WorkspaceID = 100

	setupTogglServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`not json`))
	})

	cmd := projectsCmd()
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for malformed response")
	}
}

// --- TestStartCmd ---

func TestStartCmd_Success(t *testing.T) {
	setCfg(t, Config{})
	cfg.Toggl.APIKey = "key"
	cfg.Toggl.WorkspaceID = 100
	cfg.Toggl.ProjectID = 200

	var gotBody string
	setupTogglServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		w.Write([]byte(`{"id":1}`))
	})

	cmd := startCmd()
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := cmd.Execute()

	w.Close()
	os.Stdout = oldStdout
	io.ReadAll(r)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(gotBody, `"workspace_id":100`) {
		t.Errorf("expected body to contain workspace_id:100, got: %s", gotBody)
	}
	if !strings.Contains(gotBody, `"project_id":200`) {
		t.Errorf("expected body to contain project_id:200, got: %s", gotBody)
	}
	if !strings.Contains(gotBody, `"duration":-1`) {
		t.Errorf("expected body to contain duration:-1, got: %s", gotBody)
	}
}

func TestStartCmd_APIError(t *testing.T) {
	setCfg(t, Config{})
	cfg.Toggl.APIKey = "key"
	cfg.Toggl.WorkspaceID = 100

	// Close server immediately to cause a connection error
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	old := togglBaseURL
	togglBaseURL = srv.URL + "/"
	srv.Close()
	t.Cleanup(func() { togglBaseURL = old })

	cmd := startCmd()
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when server is unreachable")
	}
}

// --- TestStopCmd ---

func TestStopCmd_NoRunningTimer(t *testing.T) {
	setCfg(t, Config{})
	cfg.Toggl.APIKey = "key"

	setupTogglServer(t, func(w http.ResponseWriter, r *http.Request) {
		// Return entry with ID 0 (no timer)
		w.Write([]byte(`{"id":0}`))
	})

	cmd := stopCmd()
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for no running timer")
	}
	if !strings.Contains(err.Error(), "no running timer") {
		t.Errorf("expected 'no running timer' error, got: %v", err)
	}
}

func TestStopCmd_HappyPath(t *testing.T) {
	setCfg(t, Config{})
	cfg.Toggl.APIKey = "key"
	cfg.Toggl.WorkspaceID = 100
	cfg.Repositories = []string{} // no repos to avoid git shell-outs

	// Create a temp worklog file
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "toggl-worklog.txt")
	os.WriteFile(logFile, []byte("[2024-01-01T00:00:00Z] test log\n"), 0644)

	// Patch os.TempDir behavior by creating the file where stopCmd expects it
	realLogFile := filepath.Join(os.TempDir(), "toggl-worklog.txt")
	os.WriteFile(realLogFile, []byte("[2024-01-01T00:00:00Z] test log\n"), 0644)
	t.Cleanup(func() { os.Remove(realLogFile) })

	requestCount := 0
	setupTogglServer(t, func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		switch {
		case r.Method == "GET" && strings.Contains(r.URL.Path, "current"):
			w.Write([]byte(`{"id":999,"workspace_id":100,"start":"2024-01-01T00:00:00Z"}`))
		case r.Method == "PUT":
			w.Write([]byte(`{"id":999}`))
		default:
			w.Write([]byte(`{}`))
		}
	})

	setupOpenAIServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]string{"content": "AI summary of work"}},
			},
		})
	})

	cmd := stopCmd()
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := cmd.Execute()

	w.Close()
	os.Stdout = oldStdout
	captured, _ := io.ReadAll(r)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := string(captured)
	if !strings.Contains(output, "Summary saved") {
		t.Errorf("expected output to contain 'Summary saved', got: %s", output)
	}
	// Worklog should be deleted after successful save
	if _, err := os.Stat(realLogFile); !os.IsNotExist(err) {
		t.Error("expected worklog file to be deleted after successful save")
	}
}

func TestStopCmd_ProjectConfigSingleProjectUpdatesOriginalEntry(t *testing.T) {
	setCfg(t, Config{})
	cfg.Toggl.APIKey = "key"
	cfg.Toggl.WorkspaceID = 100
	cfg.OpenAI.APIKey = "sk-test"
	cfg.OpenAI.Model = "gpt-4"
	cfg.Git.User = "Test User"

	repo := t.TempDir()
	git(t, repo, "init")
	git(t, repo, "config", "user.name", "Test User")
	git(t, repo, "config", "user.email", "test@example.com")
	if err := os.WriteFile(filepath.Join(repo, "file.txt"), []byte("content"), 0644); err != nil {
		t.Fatalf("write repo file: %v", err)
	}
	git(t, repo, "add", "file.txt")
	gitEnv(t, repo, []string{
		"GIT_AUTHOR_DATE=2024-01-01T09:30:00Z",
		"GIT_COMMITTER_DATE=2024-01-01T09:30:00Z",
	}, "commit", "-m", "Build cortex flow")
	cfg.Projects = map[string]ProjectConfig{
		"cortex": {ProjectID: 111, Repositories: []string{repo}},
	}

	logFile := worklogPath()
	if err := os.WriteFile(logFile, []byte("[2024-01-01T09:45:00Z] worklog entry\n"), 0644); err != nil {
		t.Fatalf("write worklog: %v", err)
	}
	t.Cleanup(func() { os.Remove(logFile) })

	start := mustTime(t, "2024-01-01T09:00:00Z")
	var putBodies []TogglTimeEntry
	postCount := 0
	setupTogglServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && strings.Contains(r.URL.Path, "current"):
			json.NewEncoder(w).Encode(TogglTimeEntry{ID: 999, Workspace: 100, Start: start})
		case r.Method == "PUT":
			var body TogglTimeEntry
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode PUT body: %v", err)
			}
			putBodies = append(putBodies, body)
			w.Write([]byte(`{"id":999}`))
		case r.Method == "POST":
			postCount++
			w.Write([]byte(`{"id":1000}`))
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	setupOpenAIServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{{"message": map[string]string{"content": "AI summary"}}},
		})
	})

	oldStdout := os.Stdout
	outR, outW, _ := os.Pipe()
	os.Stdout = outW
	err := stopCmd().Execute()
	outW.Close()
	os.Stdout = oldStdout
	io.ReadAll(outR)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(putBodies) != 2 {
		t.Fatalf("expected stop PUT and update PUT, got %d PUTs: %#v", len(putBodies), putBodies)
	}
	if postCount != 0 {
		t.Fatalf("expected no POST requests, got %d", postCount)
	}
	stopped := putBodies[0]
	updated := putBodies[1]
	if updated.Project != 111 {
		t.Fatalf("expected final update project_id 111, got %d in %#v", updated.Project, updated)
	}
	if !updated.Start.Equal(start) {
		t.Fatalf("expected final update start %s, got %s", start, updated.Start)
	}
	if !updated.Stop.Equal(stopped.Stop) {
		t.Fatalf("expected final update stop %s, got %s", stopped.Stop, updated.Stop)
	}
	if updated.Description != "AI summary" {
		t.Fatalf("expected description AI summary, got %q", updated.Description)
	}
}

func TestStopCmd_ProjectConfigUsesProjectCommitsInSummary(t *testing.T) {
	setCfg(t, Config{})
	cfg.Toggl.APIKey = "key"
	cfg.Toggl.WorkspaceID = 100
	cfg.OpenAI.APIKey = "sk-test"
	cfg.OpenAI.Model = "gpt-4"
	cfg.Git.User = "Test User"

	repo := t.TempDir()
	git(t, repo, "init")
	git(t, repo, "config", "user.name", "Test User")
	git(t, repo, "config", "user.email", "test@example.com")
	if err := os.WriteFile(filepath.Join(repo, "file.txt"), []byte("content"), 0644); err != nil {
		t.Fatalf("write repo file: %v", err)
	}
	git(t, repo, "add", "file.txt")
	gitEnv(t, repo, []string{
		"GIT_AUTHOR_DATE=2024-01-01T09:30:00Z",
		"GIT_COMMITTER_DATE=2024-01-01T09:30:00Z",
	}, "commit", "-m", "Project-only commit subject")
	cfg.Projects = map[string]ProjectConfig{
		"cortex": {ProjectID: 111, Repositories: []string{repo}},
	}
	os.Remove(worklogPath())
	t.Cleanup(func() { os.Remove(worklogPath()) })

	start := mustTime(t, "2024-01-01T09:00:00Z")
	setupTogglServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && strings.Contains(r.URL.Path, "current"):
			json.NewEncoder(w).Encode(TogglTimeEntry{ID: 999, Workspace: 100, Start: start})
		case r.Method == "PUT":
			w.Write([]byte(`{"id":999}`))
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	openAICalled := false
	var openAIBody string
	setupOpenAIServer(t, func(w http.ResponseWriter, r *http.Request) {
		openAICalled = true
		body, _ := io.ReadAll(r.Body)
		openAIBody = string(body)
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{{"message": map[string]string{"content": "AI summary"}}},
		})
	})

	oldStdin := os.Stdin
	inR, inW, _ := os.Pipe()
	inW.Write([]byte("manual fallback should not be used\n"))
	inW.Close()
	os.Stdin = inR
	t.Cleanup(func() { os.Stdin = oldStdin })

	oldStdout := os.Stdout
	outR, outW, _ := os.Pipe()
	os.Stdout = outW
	err := stopCmd().Execute()
	outW.Close()
	os.Stdout = oldStdout
	io.ReadAll(outR)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !openAICalled {
		t.Fatal("expected OpenAI to be called for project commits without worklog or legacy repositories")
	}
	if !strings.Contains(openAIBody, "Project-only commit subject") {
		t.Fatalf("expected OpenAI request body to include project commit subject, got: %s", openAIBody)
	}
}

func TestStopCmd_LegacyRepositoriesExpandTildeAndDoNotSplit(t *testing.T) {
	setCfg(t, Config{})
	cfg.Toggl.APIKey = "key"
	cfg.Toggl.WorkspaceID = 100
	cfg.OpenAI.APIKey = "sk-test"
	cfg.OpenAI.Model = "gpt-4"
	cfg.Git.User = "Test User"

	home := t.TempDir()
	t.Setenv("HOME", home)
	repo := filepath.Join(home, "legacy-repo")
	if err := os.Mkdir(repo, 0755); err != nil {
		t.Fatalf("create repo dir: %v", err)
	}
	git(t, repo, "init")
	git(t, repo, "config", "user.name", "Test User")
	git(t, repo, "config", "user.email", "test@example.com")
	if err := os.WriteFile(filepath.Join(repo, "file.txt"), []byte("legacy"), 0644); err != nil {
		t.Fatalf("write repo file: %v", err)
	}
	git(t, repo, "add", "file.txt")
	gitEnv(t, repo, []string{
		"GIT_AUTHOR_DATE=2024-01-01T09:30:00Z",
		"GIT_COMMITTER_DATE=2024-01-01T09:30:00Z",
	}, "commit", "-m", "Legacy tilde commit")
	cfg.Repositories = []string{"~/legacy-repo"}

	os.Remove(worklogPath())
	t.Cleanup(func() { os.Remove(worklogPath()) })

	start := mustTime(t, "2024-01-01T09:00:00Z")
	postCount := 0
	setupTogglServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && strings.Contains(r.URL.Path, "current"):
			json.NewEncoder(w).Encode(TogglTimeEntry{ID: 999, Workspace: 100, Project: 456, Start: start})
		case r.Method == "PUT":
			w.Write([]byte(`{"id":999}`))
		case r.Method == "POST":
			postCount++
			w.Write([]byte(`{"id":1000}`))
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	var openAIBody string
	setupOpenAIServer(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		openAIBody = string(body)
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{{"message": map[string]string{"content": "AI summary"}}},
		})
	})

	oldStdout := os.Stdout
	outR, outW, _ := os.Pipe()
	os.Stdout = outW
	err := stopCmd().Execute()
	outW.Close()
	os.Stdout = oldStdout
	io.ReadAll(outR)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(openAIBody, "Legacy tilde commit") {
		t.Fatalf("expected OpenAI prompt to include legacy tilde commit, got: %s", openAIBody)
	}
	if postCount != 0 {
		t.Fatalf("expected no split POST requests for legacy repositories, got %d", postCount)
	}
}

func TestStopCmd_ProjectConfigWithoutProjectCommitsKeepsOriginalProjectAndSavesSummary(t *testing.T) {
	setCfg(t, Config{})
	cfg.Toggl.APIKey = "key"
	cfg.Toggl.WorkspaceID = 100
	cfg.Toggl.ProjectID = 456
	cfg.OpenAI.APIKey = "sk-test"
	cfg.OpenAI.Model = "gpt-4"
	cfg.Git.User = "Test User"

	repo := t.TempDir()
	git(t, repo, "init")
	git(t, repo, "config", "user.name", "Test User")
	git(t, repo, "config", "user.email", "test@example.com")
	cfg.Projects = map[string]ProjectConfig{
		"cortex": {ProjectID: 111, Repositories: []string{repo}},
	}

	logFile := worklogPath()
	if err := os.WriteFile(logFile, []byte("[2024-01-01T09:45:00Z] worklog-only path\n"), 0644); err != nil {
		t.Fatalf("write worklog: %v", err)
	}
	t.Cleanup(func() { os.Remove(logFile) })

	start := mustTime(t, "2024-01-01T09:00:00Z")
	var putBodies []TogglTimeEntry
	postCount := 0
	setupTogglServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && strings.Contains(r.URL.Path, "current"):
			json.NewEncoder(w).Encode(TogglTimeEntry{ID: 999, Workspace: 100, Project: 456, Start: start})
		case r.Method == "PUT":
			var body TogglTimeEntry
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode PUT body: %v", err)
			}
			putBodies = append(putBodies, body)
			w.Write([]byte(`{"id":999}`))
		case r.Method == "POST":
			postCount++
			w.Write([]byte(`{"id":1000}`))
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	var openAIBody string
	setupOpenAIServer(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		openAIBody = string(body)
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{{"message": map[string]string{"content": "AI summary"}}},
		})
	})

	oldStdout := os.Stdout
	outR, outW, _ := os.Pipe()
	os.Stdout = outW
	err := stopCmd().Execute()
	outW.Close()
	os.Stdout = oldStdout
	io.ReadAll(outR)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if postCount != 0 {
		t.Fatalf("expected no split POST requests when no project commits are found, got %d", postCount)
	}
	if len(putBodies) != 2 {
		t.Fatalf("expected stop PUT and summary PUT, got %d PUTs: %#v", len(putBodies), putBodies)
	}
	updated := putBodies[1]
	if updated.Project != 456 {
		t.Fatalf("expected original/default project 456 to remain, got %d in %#v", updated.Project, updated)
	}
	if updated.Description != "AI summary" {
		t.Fatalf("expected summary saved on original entry, got %q", updated.Description)
	}
	if !strings.Contains(openAIBody, "worklog-only path") {
		t.Fatalf("expected OpenAI prompt to include worklog, got: %s", openAIBody)
	}
	if _, err := os.Stat(logFile); !os.IsNotExist(err) {
		t.Fatalf("expected worklog deleted after summary save, stat error: %v", err)
	}
}

func TestStopCmd_ProjectConfigMultipleProjectsSplitsEntries(t *testing.T) {
	setCfg(t, Config{})
	cfg.Toggl.APIKey = "key"
	cfg.Toggl.WorkspaceID = 100
	cfg.OpenAI.APIKey = "sk-test"
	cfg.OpenAI.Model = "gpt-4"
	cfg.Git.User = "Test User"

	start := mustTime(t, "2024-01-01T09:00:00Z")
	stop := mustTime(t, "2024-01-01T13:00:00Z")
	oldNow := nowFunc
	nowFunc = func() time.Time { return stop }
	t.Cleanup(func() { nowFunc = oldNow })

	cortexRepo := t.TempDir()
	voicesenseRepo := t.TempDir()
	for _, repo := range []string{cortexRepo, voicesenseRepo} {
		git(t, repo, "init")
		git(t, repo, "config", "user.name", "Test User")
		git(t, repo, "config", "user.email", "test@example.com")
	}
	if err := os.WriteFile(filepath.Join(cortexRepo, "file.txt"), []byte("cortex"), 0644); err != nil {
		t.Fatalf("write cortex file: %v", err)
	}
	git(t, cortexRepo, "add", "file.txt")
	gitEnv(t, cortexRepo, []string{
		"GIT_AUTHOR_DATE=2024-01-01T09:30:00Z",
		"GIT_COMMITTER_DATE=2024-01-01T09:30:00Z",
	}, "commit", "-m", "Build cortex flow")
	if err := os.WriteFile(filepath.Join(voicesenseRepo, "file.txt"), []byte("voicesense"), 0644); err != nil {
		t.Fatalf("write voicesense file: %v", err)
	}
	git(t, voicesenseRepo, "add", "file.txt")
	gitEnv(t, voicesenseRepo, []string{
		"GIT_AUTHOR_DATE=2024-01-01T11:30:00Z",
		"GIT_COMMITTER_DATE=2024-01-01T11:30:00Z",
	}, "commit", "-m", "Improve voicesense export")

	cfg.Projects = map[string]ProjectConfig{
		"cortex":     {ProjectID: 111, Repositories: []string{cortexRepo}},
		"voicesense": {ProjectID: 222, Repositories: []string{voicesenseRepo}},
	}
	logFile := worklogPath()
	if err := os.WriteFile(logFile, []byte("[2024-01-01T09:45:00Z] worklog entry\n"), 0644); err != nil {
		t.Fatalf("write worklog: %v", err)
	}
	t.Cleanup(func() { os.Remove(logFile) })

	var putBodies []TogglTimeEntry
	var postBodies []struct {
		WorkspaceID int    `json:"workspace_id"`
		ProjectID   int    `json:"project_id"`
		Description string `json:"description"`
		Start       string `json:"start"`
		Stop        string `json:"stop"`
		Duration    int    `json:"duration"`
	}
	setupTogglServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && strings.Contains(r.URL.Path, "current"):
			json.NewEncoder(w).Encode(TogglTimeEntry{ID: 999, Workspace: 100, Start: start})
		case r.Method == "PUT":
			var body TogglTimeEntry
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode PUT body: %v", err)
			}
			putBodies = append(putBodies, body)
			w.Write([]byte(`{"id":999}`))
		case r.Method == "POST":
			var body struct {
				WorkspaceID int    `json:"workspace_id"`
				ProjectID   int    `json:"project_id"`
				Description string `json:"description"`
				Start       string `json:"start"`
				Stop        string `json:"stop"`
				Duration    int    `json:"duration"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode POST body: %v", err)
			}
			postBodies = append(postBodies, body)
			w.Write([]byte(`{"id":1000}`))
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	setupOpenAIServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{{"message": map[string]string{"content": "AI summary"}}},
		})
	})

	oldStdout := os.Stdout
	outR, outW, _ := os.Pipe()
	os.Stdout = outW
	err := stopCmd().Execute()
	outW.Close()
	os.Stdout = oldStdout
	captured, _ := io.ReadAll(outR)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(putBodies) != 2 {
		t.Fatalf("expected stop PUT and original update PUT, got %d PUTs: %#v", len(putBodies), putBodies)
	}
	if len(postBodies) != 1 {
		t.Fatalf("expected one split POST, got %d: %#v", len(postBodies), postBodies)
	}
	updated := putBodies[1]
	created := postBodies[0]
	wantBoundary := mustTime(t, "2024-01-01T10:30:00Z")
	createdStart := mustTime(t, created.Start)
	createdStop := mustTime(t, created.Stop)
	if updated.Project != 111 || created.ProjectID != 222 {
		t.Fatalf("expected project IDs 111 then 222, got update %d create %d", updated.Project, created.ProjectID)
	}
	if !updated.Start.Equal(start) || !updated.Stop.Equal(wantBoundary) {
		t.Fatalf("expected original update %s-%s, got %s-%s", start, wantBoundary, updated.Start, updated.Stop)
	}
	if !createdStart.Equal(updated.Stop) || !createdStop.Equal(stop) {
		t.Fatalf("expected created entry contiguous %s-%s, got %s-%s", updated.Stop, stop, createdStart, createdStop)
	}
	if updated.Description != "AI summary" || created.Description != "AI summary" {
		t.Fatalf("expected AI summary descriptions, got update %q create %q", updated.Description, created.Description)
	}
	output := string(captured)
	for _, want := range []string{"Stopped tracking. Split into:", "cortex", "1h30m0s", "voicesense", "2h30m0s", "Summary saved."} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected split output to contain %q, got: %s", want, output)
		}
	}
}

func TestStopCmd_ProjectSplitCreateFailurePreservesWorklog(t *testing.T) {
	setCfg(t, Config{})
	cfg.Toggl.APIKey = "key"
	cfg.Toggl.WorkspaceID = 100
	cfg.OpenAI.APIKey = "sk-test"
	cfg.OpenAI.Model = "gpt-4"
	cfg.Git.User = "Test User"

	start := mustTime(t, "2024-01-01T09:00:00Z")
	stop := mustTime(t, "2024-01-01T13:00:00Z")
	oldNow := nowFunc
	nowFunc = func() time.Time { return stop }
	t.Cleanup(func() { nowFunc = oldNow })

	cortexRepo := t.TempDir()
	voicesenseRepo := t.TempDir()
	for _, repo := range []string{cortexRepo, voicesenseRepo} {
		git(t, repo, "init")
		git(t, repo, "config", "user.name", "Test User")
		git(t, repo, "config", "user.email", "test@example.com")
	}
	if err := os.WriteFile(filepath.Join(cortexRepo, "file.txt"), []byte("cortex"), 0644); err != nil {
		t.Fatalf("write cortex file: %v", err)
	}
	git(t, cortexRepo, "add", "file.txt")
	gitEnv(t, cortexRepo, []string{
		"GIT_AUTHOR_DATE=2024-01-01T09:30:00Z",
		"GIT_COMMITTER_DATE=2024-01-01T09:30:00Z",
	}, "commit", "-m", "Build cortex flow")
	if err := os.WriteFile(filepath.Join(voicesenseRepo, "file.txt"), []byte("voicesense"), 0644); err != nil {
		t.Fatalf("write voicesense file: %v", err)
	}
	git(t, voicesenseRepo, "add", "file.txt")
	gitEnv(t, voicesenseRepo, []string{
		"GIT_AUTHOR_DATE=2024-01-01T11:30:00Z",
		"GIT_COMMITTER_DATE=2024-01-01T11:30:00Z",
	}, "commit", "-m", "Improve voicesense export")
	cfg.Projects = map[string]ProjectConfig{
		"cortex":     {ProjectID: 111, Repositories: []string{cortexRepo}},
		"voicesense": {ProjectID: 222, Repositories: []string{voicesenseRepo}},
	}

	logFile := worklogPath()
	if err := os.WriteFile(logFile, []byte("[2024-01-01T09:45:00Z] important worklog\n"), 0644); err != nil {
		t.Fatalf("write worklog: %v", err)
	}
	t.Cleanup(func() { os.Remove(logFile) })

	setupTogglServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && strings.Contains(r.URL.Path, "current"):
			json.NewEncoder(w).Encode(TogglTimeEntry{ID: 999, Workspace: 100, Start: start})
		case r.Method == "PUT":
			w.Write([]byte(`{"id":999}`))
		case r.Method == "POST":
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error":"create failed"}`))
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	setupOpenAIServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{{"message": map[string]string{"content": "AI summary"}}},
		})
	})

	oldStdout := os.Stdout
	outR, outW, _ := os.Pipe()
	os.Stdout = outW
	err := stopCmd().Execute()
	outW.Close()
	os.Stdout = oldStdout
	io.ReadAll(outR)

	if err == nil {
		t.Fatal("expected error when split entry creation fails")
	}
	if !strings.Contains(err.Error(), "split creation failed") || (!strings.Contains(err.Error(), "voicesense") && !strings.Contains(err.Error(), "222")) {
		t.Fatalf("expected split creation error with project context, got: %v", err)
	}
	if _, statErr := os.Stat(logFile); statErr != nil {
		t.Fatalf("expected worklog file to be preserved, stat error: %v", statErr)
	}
}

func TestStopCmd_ProjectSplitOriginalUpdateFailurePreservesWorklogAndSkipsPost(t *testing.T) {
	setCfg(t, Config{})
	cfg.Toggl.APIKey = "key"
	cfg.Toggl.WorkspaceID = 100
	cfg.OpenAI.APIKey = "sk-test"
	cfg.OpenAI.Model = "gpt-4"
	cfg.Git.User = "Test User"

	start := mustTime(t, "2024-01-01T09:00:00Z")
	stop := mustTime(t, "2024-01-01T13:00:00Z")
	oldNow := nowFunc
	nowFunc = func() time.Time { return stop }
	t.Cleanup(func() { nowFunc = oldNow })

	cortexRepo := t.TempDir()
	voicesenseRepo := t.TempDir()
	for _, repo := range []string{cortexRepo, voicesenseRepo} {
		git(t, repo, "init")
		git(t, repo, "config", "user.name", "Test User")
		git(t, repo, "config", "user.email", "test@example.com")
	}
	if err := os.WriteFile(filepath.Join(cortexRepo, "file.txt"), []byte("cortex"), 0644); err != nil {
		t.Fatalf("write cortex file: %v", err)
	}
	git(t, cortexRepo, "add", "file.txt")
	gitEnv(t, cortexRepo, []string{
		"GIT_AUTHOR_DATE=2024-01-01T09:30:00Z",
		"GIT_COMMITTER_DATE=2024-01-01T09:30:00Z",
	}, "commit", "-m", "Build cortex flow")
	if err := os.WriteFile(filepath.Join(voicesenseRepo, "file.txt"), []byte("voicesense"), 0644); err != nil {
		t.Fatalf("write voicesense file: %v", err)
	}
	git(t, voicesenseRepo, "add", "file.txt")
	gitEnv(t, voicesenseRepo, []string{
		"GIT_AUTHOR_DATE=2024-01-01T11:30:00Z",
		"GIT_COMMITTER_DATE=2024-01-01T11:30:00Z",
	}, "commit", "-m", "Improve voicesense export")
	cfg.Projects = map[string]ProjectConfig{
		"cortex":     {ProjectID: 111, Repositories: []string{cortexRepo}},
		"voicesense": {ProjectID: 222, Repositories: []string{voicesenseRepo}},
	}

	logFile := worklogPath()
	if err := os.WriteFile(logFile, []byte("[2024-01-01T09:45:00Z] important worklog\n"), 0644); err != nil {
		t.Fatalf("write worklog: %v", err)
	}
	t.Cleanup(func() { os.Remove(logFile) })

	putCount := 0
	postCount := 0
	setupTogglServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && strings.Contains(r.URL.Path, "current"):
			json.NewEncoder(w).Encode(TogglTimeEntry{ID: 999, Workspace: 100, Start: start})
		case r.Method == "PUT":
			putCount++
			if putCount == 2 {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte(`{"error":"update failed"}`))
				return
			}
			w.Write([]byte(`{"id":999}`))
		case r.Method == "POST":
			postCount++
			w.Write([]byte(`{"id":1000}`))
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	setupOpenAIServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{{"message": map[string]string{"content": "AI summary"}}},
		})
	})

	oldStdout := os.Stdout
	outR, outW, _ := os.Pipe()
	os.Stdout = outW
	err := stopCmd().Execute()
	outW.Close()
	os.Stdout = oldStdout
	io.ReadAll(outR)

	if err == nil {
		t.Fatal("expected error when original split update fails")
	}
	if postCount != 0 {
		t.Fatalf("expected no extra split POST after original update failure, got %d", postCount)
	}
	if _, statErr := os.Stat(logFile); statErr != nil {
		t.Fatalf("expected worklog file to be preserved, stat error: %v", statErr)
	}
}

func TestStopCmd_AIFailureManualInput(t *testing.T) {
	setCfg(t, Config{})
	cfg.Toggl.APIKey = "key"
	cfg.Toggl.WorkspaceID = 100
	cfg.Repositories = []string{}

	setupTogglServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && strings.Contains(r.URL.Path, "current"):
			w.Write([]byte(`{"id":999,"workspace_id":100,"start":"2024-01-01T00:00:00Z"}`))
		case r.Method == "PUT":
			w.Write([]byte(`{"id":999}`))
		default:
			w.Write([]byte(`{}`))
		}
	})

	setupOpenAIServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
	})

	// Pipe manual input into stdin
	oldStdin := os.Stdin
	input := "My manual description\n"
	r, w, _ := os.Pipe()
	w.Write([]byte(input))
	w.Close()
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = oldStdin })

	// Capture stdout
	oldStdout := os.Stdout
	outR, outW, _ := os.Pipe()
	os.Stdout = outW

	cmd := stopCmd()
	err := cmd.Execute()

	outW.Close()
	os.Stdout = oldStdout
	captured, _ := io.ReadAll(outR)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := string(captured)
	if !strings.Contains(output, "Summary saved") {
		t.Errorf("expected 'Summary saved', got: %s", output)
	}
}

func TestStopCmd_NoCommitsOrWorklogPromptsForManualSummary(t *testing.T) {
	setCfg(t, Config{})
	cfg.Toggl.APIKey = "key"
	cfg.Toggl.WorkspaceID = 100
	cfg.Repositories = []string{}

	os.Remove(worklogPath())
	t.Cleanup(func() { os.Remove(worklogPath()) })

	var updatedBody string
	putCount := 0
	setupTogglServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && strings.Contains(r.URL.Path, "current"):
			w.Write([]byte(`{"id":999,"workspace_id":100,"start":"2024-01-01T00:00:00Z"}`))
		case r.Method == "PUT":
			putCount++
			if putCount == 2 {
				body, _ := io.ReadAll(r.Body)
				updatedBody = string(body)
			}
			w.Write([]byte(`{"id":999}`))
		default:
			w.Write([]byte(`{}`))
		}
	})

	openAICalled := false
	setupOpenAIServer(t, func(w http.ResponseWriter, r *http.Request) {
		openAICalled = true
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]string{"content": badSummaryText}},
			},
		})
	})

	oldStdin := os.Stdin
	inR, inW, _ := os.Pipe()
	inW.Write([]byte("Manual no-commit summary\n"))
	inW.Close()
	os.Stdin = inR
	t.Cleanup(func() { os.Stdin = oldStdin })

	oldStdout := os.Stdout
	outR, outW, _ := os.Pipe()
	os.Stdout = outW

	cmd := stopCmd()
	err := cmd.Execute()

	outW.Close()
	os.Stdout = oldStdout
	io.ReadAll(outR)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if openAICalled {
		t.Fatal("expected no OpenAI request when there are no commits or work log entries")
	}
	if !strings.Contains(updatedBody, `"description":"Manual no-commit summary"`) {
		t.Fatalf("expected manual summary to be saved, got body: %s", updatedBody)
	}
}

func TestBuildPromptText_OmitsEmptyCommitSection(t *testing.T) {
	got := buildPromptText(nil, "[2024-01-01T00:00:00Z] investigated issue\n")
	if strings.Contains(got, "Git commits:") {
		t.Fatalf("expected empty git section to be omitted, got: %q", got)
	}
	if !strings.Contains(got, "Work log:") {
		t.Fatalf("expected work log section, got: %q", got)
	}
}

func TestRepairSummariesCmd_RepairsEntriesWithBadSummary(t *testing.T) {
	setCfg(t, Config{})
	cfg.Toggl.APIKey = "key"
	cfg.Toggl.WorkspaceID = 100
	cfg.OpenAI.APIKey = "sk-test"
	cfg.OpenAI.Model = "gpt-4"

	repo := t.TempDir()
	git(t, repo, "init")
	git(t, repo, "config", "user.name", "Test User")
	git(t, repo, "config", "user.email", "test@example.com")
	os.WriteFile(filepath.Join(repo, "file.txt"), []byte("content"), 0644)
	git(t, repo, "add", "file.txt")
	gitEnv(t, repo, []string{
		"GIT_AUTHOR_DATE=2024-01-01T12:00:00Z",
		"GIT_COMMITTER_DATE=2024-01-01T12:00:00Z",
	}, "commit", "-m", "Fix commit processing")
	cfg.Repositories = []string{repo}
	cfg.Git.User = "Test User"

	var updatedBodies []string
	setupTogglServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && strings.Contains(r.URL.Path, "me/time_entries"):
			entries := []TogglTimeEntry{
				{ID: 1, Workspace: 100, Start: mustTime(t, "2024-01-01T11:00:00Z"), Stop: mustTime(t, "2024-01-01T13:00:00Z"), Description: badSummaryText},
				{ID: 2, Workspace: 100, Start: mustTime(t, "2024-01-01T11:00:00Z"), Stop: mustTime(t, "2024-01-01T13:00:00Z"), Description: "Already good"},
			}
			json.NewEncoder(w).Encode(entries)
		case r.Method == "PUT" && strings.Contains(r.URL.Path, "time_entries/1"):
			body, _ := io.ReadAll(r.Body)
			updatedBodies = append(updatedBodies, string(body))
			w.Write([]byte(`{"id":1}`))
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	setupOpenAIServer(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "Fix commit processing") {
			t.Fatalf("expected commit subject in OpenAI prompt, got: %s", body)
		}
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]string{"content": "Fixed commit processing."}},
			},
		})
	})

	cmd := repairSummariesCmd()
	cmd.SetArgs([]string{"--start", "2024-01-01T00:00:00Z", "--end", "2024-01-02T00:00:00Z"})
	oldStdout := os.Stdout
	outR, outW, _ := os.Pipe()
	os.Stdout = outW

	err := cmd.Execute()

	outW.Close()
	os.Stdout = oldStdout
	io.ReadAll(outR)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(updatedBodies) != 1 {
		t.Fatalf("expected one repaired entry, got %d", len(updatedBodies))
	}
	if !strings.Contains(updatedBodies[0], `"description":"Fixed commit processing."`) {
		t.Fatalf("expected repaired summary in update body, got: %s", updatedBodies[0])
	}
}

func TestRepairSummariesCmd_RepairsAlternateBadSummary(t *testing.T) {
	setCfg(t, Config{})
	cfg.Toggl.APIKey = "key"
	cfg.Toggl.WorkspaceID = 100
	cfg.OpenAI.APIKey = "sk-test"
	cfg.OpenAI.Model = "gpt-4"
	cfg.Repositories = []string{}

	var updated bool
	setupTogglServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && strings.Contains(r.URL.Path, "me/time_entries"):
			entries := []TogglTimeEntry{
				{ID: 1, Workspace: 100, Start: mustTime(t, "2026-05-04T07:03:05Z"), Stop: mustTime(t, "2026-05-04T11:58:19Z"), Description: "Sure! Please provide the details of your commits so I can generate a summary for your Toggl time entry."},
			}
			json.NewEncoder(w).Encode(entries)
		case r.Method == "PUT" && strings.Contains(r.URL.Path, "time_entries/1"):
			updated = true
			w.Write([]byte(`{"id":1}`))
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	oldStdin := os.Stdin
	inR, inW, _ := os.Pipe()
	inW.Write([]byte("Recovered alternate summary\n"))
	inW.Close()
	os.Stdin = inR
	t.Cleanup(func() { os.Stdin = oldStdin })

	cmd := repairSummariesCmd()
	cmd.SetArgs([]string{"--start", "2026-05-01T00:00:00Z", "--end", "2026-06-01T00:00:00Z"})
	oldStdout := os.Stdout
	outR, outW, _ := os.Pipe()
	os.Stdout = outW

	err := cmd.Execute()

	outW.Close()
	os.Stdout = oldStdout
	io.ReadAll(outR)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !updated {
		t.Fatal("expected alternate bad summary to be repaired")
	}
}

func TestStopCmd_AIFailureSkip(t *testing.T) {
	setCfg(t, Config{})
	cfg.Toggl.APIKey = "key"
	cfg.Toggl.WorkspaceID = 100
	cfg.Repositories = []string{}

	logFile := worklogPath()
	os.WriteFile(logFile, []byte("[2024-01-01T00:00:00Z] work to summarize\n"), 0644)
	t.Cleanup(func() { os.Remove(logFile) })

	setupTogglServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && strings.Contains(r.URL.Path, "current"):
			w.Write([]byte(`{"id":999,"workspace_id":100,"start":"2024-01-01T00:00:00Z"}`))
		case r.Method == "PUT":
			w.Write([]byte(`{"id":999}`))
		default:
			w.Write([]byte(`{}`))
		}
	})

	setupOpenAIServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
	})

	// Pipe empty line (user presses Enter to skip)
	oldStdin := os.Stdin
	r, w, _ := os.Pipe()
	w.Write([]byte("\n"))
	w.Close()
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = oldStdin })

	oldStdout := os.Stdout
	outR, outW, _ := os.Pipe()
	os.Stdout = outW

	cmd := stopCmd()
	err := cmd.Execute()

	outW.Close()
	os.Stdout = oldStdout
	captured, _ := io.ReadAll(outR)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := string(captured)
	if !strings.Contains(output, "Check your OpenAI API key") {
		t.Errorf("expected API key warning, got: %s", output)
	}
}

func TestStopCmd_WorklogPreservedOnPUTFailure(t *testing.T) {
	setCfg(t, Config{})
	cfg.Toggl.APIKey = "key"
	cfg.Toggl.WorkspaceID = 100
	cfg.Repositories = []string{}

	// Create worklog file
	realLogFile := filepath.Join(os.TempDir(), "toggl-worklog.txt")
	os.WriteFile(realLogFile, []byte("[2024-01-01T00:00:00Z] important log\n"), 0644)
	t.Cleanup(func() { os.Remove(realLogFile) })

	putCount := 0
	setupTogglServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && strings.Contains(r.URL.Path, "current"):
			w.Write([]byte(`{"id":999,"workspace_id":100,"start":"2024-01-01T00:00:00Z"}`))
		case r.Method == "PUT":
			putCount++
			if putCount == 1 {
				// First PUT (stop timer) succeeds
				w.Write([]byte(`{"id":999}`))
			} else {
				// Second PUT (save description) - close connection to cause error
				hj, ok := w.(http.Hijacker)
				if ok {
					conn, _, _ := hj.Hijack()
					conn.Close()
				}
			}
		default:
			w.Write([]byte(`{}`))
		}
	})

	setupOpenAIServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]string{"content": "summary"}},
			},
		})
	})

	cmd := stopCmd()
	oldStdout := os.Stdout
	_, outW, _ := os.Pipe()
	os.Stdout = outW

	err := cmd.Execute()

	outW.Close()
	os.Stdout = oldStdout

	if err == nil {
		t.Fatal("expected error when final PUT fails")
	}
	// Worklog should still exist since the save failed
	if _, statErr := os.Stat(realLogFile); os.IsNotExist(statErr) {
		t.Error("expected worklog file to be preserved when description PUT fails")
	}
}

// --- TestLogCmd ---

func TestLogCmd_SingleMessage(t *testing.T) {
	// We test the log command by executing it and checking the file
	// The log command writes to os.TempDir()/toggl-worklog.txt
	logFile := filepath.Join(os.TempDir(), "toggl-worklog.txt")
	os.Remove(logFile) // clean slate
	t.Cleanup(func() { os.Remove(logFile) })

	cmd := logCmd()
	cmd.SetArgs([]string{"test message"})

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := cmd.Execute()

	w.Close()
	os.Stdout = oldStdout
	io.ReadAll(r)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "test message") {
		t.Errorf("expected log to contain 'test message', got: %s", content)
	}
	if !strings.Contains(content, "[") || !strings.Contains(content, "]") {
		t.Errorf("expected log to contain timestamp brackets, got: %s", content)
	}
}

func TestLogCmd_TwoMessages(t *testing.T) {
	logFile := filepath.Join(os.TempDir(), "toggl-worklog.txt")
	os.Remove(logFile)
	t.Cleanup(func() { os.Remove(logFile) })

	// First message
	cmd1 := logCmd()
	cmd1.SetArgs([]string{"first entry"})
	oldStdout := os.Stdout
	_, w, _ := os.Pipe()
	os.Stdout = w
	if err := cmd1.Execute(); err != nil {
		w.Close()
		os.Stdout = oldStdout
		t.Fatalf("first log failed: %v", err)
	}
	w.Close()
	os.Stdout = oldStdout

	// Second message
	cmd2 := logCmd()
	cmd2.SetArgs([]string{"second entry"})
	_, w, _ = os.Pipe()
	os.Stdout = w
	if err := cmd2.Execute(); err != nil {
		w.Close()
		os.Stdout = oldStdout
		t.Fatalf("second log failed: %v", err)
	}
	w.Close()
	os.Stdout = oldStdout

	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "first entry") {
		t.Errorf("expected 'first entry' in log, got: %s", content)
	}
	if !strings.Contains(content, "second entry") {
		t.Errorf("expected 'second entry' in log, got: %s", content)
	}
	lines := strings.Split(strings.TrimSpace(content), "\n")
	if len(lines) != 2 {
		t.Errorf("expected 2 lines, got %d: %s", len(lines), content)
	}
}

func TestLogCmd_NoArgument(t *testing.T) {
	cmd := logCmd()
	cmd.SetArgs([]string{})
	// Silence usage output
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when no argument provided")
	}
}
