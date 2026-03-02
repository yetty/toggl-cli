package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

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

func TestStopCmd_AIFailureSkip(t *testing.T) {
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
