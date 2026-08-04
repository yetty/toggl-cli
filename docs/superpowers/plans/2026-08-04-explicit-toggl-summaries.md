# Explicit Toggl Summaries Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Generate concrete Toggl descriptions that select the day's most important outcomes and selectively retain their issue IDs.

**Architecture:** Retain the existing single OpenAI completion and all summary persistence paths. Replace only the system-prompt contract, then test the outbound request to prevent future regressions to vague, process-focused language.

**Tech Stack:** Go, Cobra, `net/http/httptest`, OpenAI Chat Completions API.

---

## File structure

- Modify: `src/main.go:133-135` — define the explicit-summary system-prompt rules used by every `openAISummarize` call.
- Modify: `src/main_test.go` — add a focused HTTP-level regression test for the OpenAI request payload.

### Task 1: Specify and verify the explicit summary contract

**Files:**
- Modify: `src/main_test.go` — add `TestOpenAISummarizeSendsExplicitSummaryInstructions` before `captureStdout`.
- Modify: `src/main.go:133-135` — replace `openAISystemPrompt`.

- [ ] **Step 1: Write the failing request-payload test**

Add this test before `captureStdout` in `src/main_test.go`:

```go
func TestOpenAISummarizeSendsExplicitSummaryInstructions(t *testing.T) {
	oldCfg, oldOpenAIBase := cfg, openAIBaseURL
	defer func() { cfg, openAIBaseURL = oldCfg, oldOpenAIBase }()
	cfg = Config{}
	cfg.OpenAI.Model = "test"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}

		var request struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if len(request.Messages) < 1 {
			t.Fatal("request contains no messages")
		}

		systemPrompt := request.Messages[0].Content
		for _, want := range []string{
			"one to three highest-impact outcomes",
			"issue identifier only when it identifies a selected high-impact outcome",
			"OpenSpec proposals and archives, merge commits, dependency updates",
			"maximum 280 characters",
		} {
			if !strings.Contains(systemPrompt, want) {
				t.Fatalf("system prompt missing %q: %q", want, systemPrompt)
			}
		}

		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]string{"content": "#3917 add dashboard GraphQL data; enable managed Keycloak user sync (#3871)."},
			}},
		})
	}))
	defer server.Close()
	openAIBaseURL = server.URL

	description, err := openAISummarize("Git commits:\n#3917 add dashboard GraphQL data")
	if err != nil {
		t.Fatalf("openAISummarize returned error: %v", err)
	}
	if want := "#3917 add dashboard GraphQL data; enable managed Keycloak user sync (#3871)."; description != want {
		t.Fatalf("description = %q, want %q", description, want)
	}
}
```

- [ ] **Step 2: Run the new test to verify it fails**

Run: `go test ./src -run TestOpenAISummarizeSendsExplicitSummaryInstructions -count=1`

Expected: FAIL because the current prompt does not contain the explicit-selection, selective-ID, process-noise, or 280-character rules.

- [ ] **Step 3: Replace the system prompt with the approved contract**

Replace `openAISystemPrompt` in `src/main.go` with:

```go
const openAISystemPrompt = `You summarize Git commits and work logs for Toggl time entry descriptions.
Infer the one to three highest-impact outcomes from the supplied commits and work log.
For each selected outcome, state a concrete action and the affected capability, feature, or defect.
Include an issue identifier only when it identifies a selected high-impact outcome; do not list every issue or pull request.
Prefer substantive implementation work over process noise. Ignore OpenSpec proposals and archives, merge commits, dependency updates, localisation churn, and configuration-only changes unless no substantive work is present.
Write one readable line, maximum 280 characters. Use concise clauses separated by semicolons where useful.
Avoid vague phrases such as "OpenSpec changes", "various updates", "implemented improvements", and "merged PRs" unless unavoidable.`
```

- [ ] **Step 4: Run the focused test to verify it passes**

Run: `go test ./src -run TestOpenAISummarizeSendsExplicitSummaryInstructions -count=1`

Expected: PASS.

- [ ] **Step 5: Run the complete test suite**

Run: `make test`

Expected: PASS for all packages.

- [ ] **Step 6: Commit the focused change when explicitly requested**

```bash
git add src/main.go src/main_test.go
git commit -m "Improve Toggl summary specificity"
```
