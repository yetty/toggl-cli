package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOpenAISummarizeSendsExplicitSummaryInstructions(t *testing.T) {
	oldCfg, oldOpenAIBaseURL := cfg, openAIBaseURL
	defer func() {
		cfg, openAIBaseURL = oldCfg, oldOpenAIBaseURL
	}()

	cfg.OpenAI.Model = "test-model"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode OpenAI request: %v", err)
		}

		var systemMessage string
		for _, message := range request.Messages {
			if message.Role == "system" {
				systemMessage = message.Content
				break
			}
		}
		for _, instruction := range []string{
			"one to three highest-impact outcomes",
			"state a concrete action and the affected capability, feature, or defect",
			"issue identifier only when it identifies a selected high-impact outcome",
			"OpenSpec proposals and archives, merge commits, dependency updates",
			"Write one readable line",
			"maximum 280 characters",
		} {
			if !strings.Contains(systemMessage, instruction) {
				t.Errorf("system message missing %q: %q", instruction, systemMessage)
			}
		}

		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]string{"content": "Added project filtering"},
			}},
		})
	}))
	defer server.Close()
	openAIBaseURL = server.URL

	description, err := openAISummarize("feat: filter projects")
	if err != nil {
		t.Fatalf("openAISummarize returned error: %v", err)
	}
	if description != "Added project filtering" {
		t.Fatalf("description = %q, want unchanged response", description)
	}
}

func TestCalendarConfigEnabledOnlyWhenRequiredFieldsPresent(t *testing.T) {
	var c Config
	if c.Calendar.Enabled() {
		t.Fatal("calendar tracking should be disabled when calendar config is absent")
	}

	c.Calendar.ID = "primary@example.com"
	c.Calendar.APIKey = "calendar-api-key"
	c.Calendar.EventNames = []string{"Deep Work"}
	c.Calendar.MonthlyHourBudget = 42
	if !c.Calendar.Enabled() {
		t.Fatal("calendar tracking should be enabled when required fields are present")
	}
}

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

func TestDedupeCommitsRemovesCrossSourceDuplicates(t *testing.T) {
	when := time.Date(2026, 6, 9, 10, 30, 0, 0, time.UTC)
	commits := []ProjectCommit{
		{ProjectName: "voicesense", ProjectID: 1, RepoName: "svc", Subject: "feat: same", Time: when},
		{ProjectName: "voicesense", ProjectID: 1, RepoName: "svc", Subject: "feat: same", Time: when},
		{ProjectName: "voicesense", ProjectID: 1, RepoName: "other", Subject: "feat: same", Time: when},
		{ProjectName: "voicesense", ProjectID: 1, RepoName: "svc", Subject: "feat: other", Time: when},
	}
	deduped := dedupeCommits(commits)
	if len(deduped) != 2 {
		t.Fatalf("deduped %d commits, want 2: %+v", len(deduped), deduped)
	}
}

func TestFetchForgejoIssuesFiltersToResolvedRepositories(t *testing.T) {
	oldCfg := cfg
	defer func() { cfg = oldCfg }()

	cfg = Config{}
	cfg.Forgejo.APIKey = "token"

	entry := TogglTimeEntry{
		Start: time.Date(2026, 6, 9, 10, 0, 0, 0, time.UTC),
		Stop:  time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC),
	}

	relationships := []string{"created", "assigned", "review_requested", "reviewed"}
	seenRelationships := map[string]bool{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/repos/issues/search" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		active := ""
		for _, param := range relationships {
			if r.URL.Query().Get(param) == "true" {
				if active != "" {
					t.Fatalf("request combined relationship filters: %v", r.URL.Query())
				}
				active = param
			}
		}
		if active == "" {
			t.Fatalf("request missing a relationship filter: %v", r.URL.Query())
		}
		seenRelationships[active] = true
		if r.URL.Query().Get("mentioned") != "" {
			t.Fatal("mentioned should not be requested")
		}
		if r.URL.Query().Get("state") != "all" {
			t.Fatalf("state = %q, want all", r.URL.Query().Get("state"))
		}
		if r.URL.Query().Get("since") != "2026-06-09T10:00:00Z" {
			t.Fatalf("since = %q", r.URL.Query().Get("since"))
		}
		if r.URL.Query().Get("before") != "2026-06-09T12:00:00Z" {
			t.Fatalf("before = %q", r.URL.Query().Get("before"))
		}
		if r.URL.Query().Get("limit") != "50" {
			t.Fatalf("limit = %q, want 50", r.URL.Query().Get("limit"))
		}
		if r.URL.Query().Get("page") != "1" {
			t.Fatalf("page = %q, want 1", r.URL.Query().Get("page"))
		}
		if active != "created" {
			json.NewEncoder(w).Encode([]map[string]any{})
			return
		}
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
				"repository": map[string]any{"full_name": "Voicesense/Voicesense-Web"},
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
	for _, relationship := range relationships {
		if !seenRelationships[relationship] {
			t.Fatalf("relationship %q was never queried", relationship)
		}
	}
	if len(issues) != 2 {
		t.Fatalf("got %d issues, want 2: %+v", len(issues), issues)
	}
	if issues[0].Number != 12 || issues[1].Number != 45 {
		t.Fatalf("unexpected issue numbers: %+v", issues)
	}
	if issues[0].Title != "Persist quota decisions" {
		t.Fatalf("title = %q", issues[0].Title)
	}
	if !issues[0].UpdatedAt.Equal(time.Date(2026, 6, 9, 11, 0, 0, 0, time.UTC)) {
		t.Fatalf("updated_at = %s", issues[0].UpdatedAt)
	}
}

func forgejoIssueItem(number int, fullName string) map[string]any {
	return map[string]any{
		"number":     number,
		"title":      "Tracked issue",
		"updated_at": "2026-06-09T11:00:00Z",
		"repository": map[string]any{"full_name": fullName},
	}
}

func TestFetchForgejoIssuesPagesThroughAllResults(t *testing.T) {
	oldCfg := cfg
	defer func() { cfg = oldCfg }()
	cfg = Config{}
	cfg.Forgejo.APIKey = "token"

	entry := TogglTimeEntry{
		Start: time.Date(2026, 6, 9, 10, 0, 0, 0, time.UTC),
		Stop:  time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC),
	}

	fullPage := func(start int) []map[string]any {
		items := make([]map[string]any, 0, forgejoPageSize)
		for i := 0; i < forgejoPageSize; i++ {
			items = append(items, forgejoIssueItem(start+i, "voicesense/voicesense-backend"))
		}
		return items
	}

	createdRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("created") != "true" {
			json.NewEncoder(w).Encode([]map[string]any{})
			return
		}
		createdRequests++
		switch r.URL.Query().Get("page") {
		case "1":
			json.NewEncoder(w).Encode(fullPage(1))
		case "2":
			json.NewEncoder(w).Encode(fullPage(51))
		case "3":
			json.NewEncoder(w).Encode([]map[string]any{forgejoIssueItem(101, "voicesense/voicesense-backend")})
		default:
			t.Errorf("unexpected page %s", r.URL.Query().Get("page"))
			json.NewEncoder(w).Encode([]map[string]any{})
		}
	}))
	defer server.Close()
	cfg.Forgejo.URL = server.URL

	issues, err := fetchForgejoIssues(entry, []ForgejoRepo{{Owner: "voicesense", Name: "voicesense-backend"}})
	if err != nil {
		t.Fatalf("fetchForgejoIssues returned error: %v", err)
	}
	if len(issues) != 2*forgejoPageSize+1 {
		t.Fatalf("got %d issues, want %d", len(issues), 2*forgejoPageSize+1)
	}
	if createdRequests != 3 {
		t.Fatalf("created requests = %d, want 3", createdRequests)
	}
}

func TestFetchForgejoIssuesDeduplicatesAcrossRelationships(t *testing.T) {
	oldCfg := cfg
	defer func() { cfg = oldCfg }()
	cfg = Config{}
	cfg.Forgejo.APIKey = "token"

	entry := TogglTimeEntry{
		Start: time.Date(2026, 6, 9, 10, 0, 0, 0, time.UTC),
		Stop:  time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC),
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("created") == "true" || r.URL.Query().Get("assigned") == "true" {
			json.NewEncoder(w).Encode([]map[string]any{forgejoIssueItem(7, "voicesense/voicesense-backend")})
			return
		}
		json.NewEncoder(w).Encode([]map[string]any{})
	}))
	defer server.Close()
	cfg.Forgejo.URL = server.URL

	issues, err := fetchForgejoIssues(entry, []ForgejoRepo{{Owner: "voicesense", Name: "voicesense-backend"}})
	if err != nil {
		t.Fatalf("fetchForgejoIssues returned error: %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("got %d issues, want 1 after dedupe across relationships: %+v", len(issues), issues)
	}
}

func TestFetchForgejoIssuesSkipsEmptyRepositoryAllowlist(t *testing.T) {
	issues, err := fetchForgejoIssues(TogglTimeEntry{}, nil)
	if err != nil {
		t.Fatalf("fetchForgejoIssues returned error: %v", err)
	}
	if issues != nil {
		t.Fatalf("issues = %+v, want nil", issues)
	}
}

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
		{name: "https host with port", raw: "https://infra.senseloom.com:3000/voicesense/voicesense-backend.git", wantOwner: "voicesense", wantName: "voicesense-backend", wantOK: true},
		{name: "case insensitive host", raw: "https://INFRA.SENSELOOM.COM/voicesense/voicesense-web.git", wantOwner: "voicesense", wantName: "voicesense-web", wantOK: true},
		{name: "trailing slash", raw: "https://infra.senseloom.com/voicesense/senseloom-infra/", wantOwner: "voicesense", wantName: "senseloom-infra", wantOK: true},
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

func TestMonthAndDayBoundariesUseLocalTime(t *testing.T) {
	loc := time.FixedZone("local", 2*60*60)
	now := time.Date(2026, time.June, 9, 15, 30, 0, 0, loc)

	monthStart, monthEnd := currentMonthBounds(now)
	if want := time.Date(2026, time.June, 1, 0, 0, 0, 0, loc); !monthStart.Equal(want) {
		t.Fatalf("month start = %s, want %s", monthStart, want)
	}
	if want := time.Date(2026, time.July, 1, 0, 0, 0, 0, loc); !monthEnd.Equal(want) {
		t.Fatalf("month end = %s, want %s", monthEnd, want)
	}

	dayStart, dayEnd := currentDayBounds(now)
	if want := time.Date(2026, time.June, 9, 0, 0, 0, 0, loc); !dayStart.Equal(want) {
		t.Fatalf("day start = %s, want %s", dayStart, want)
	}
	if want := time.Date(2026, time.June, 10, 0, 0, 0, 0, loc); !dayEnd.Equal(want) {
		t.Fatalf("day end = %s, want %s", dayEnd, want)
	}
}

func TestNextMonthBoundsUseLocalTimeAcrossYearBoundary(t *testing.T) {
	loc := time.FixedZone("local", 2*60*60)
	now := time.Date(2026, time.December, 31, 15, 30, 0, 0, loc)

	nextStart, nextEnd := nextMonthBounds(now)
	if want := time.Date(2027, time.January, 1, 0, 0, 0, 0, loc); !nextStart.Equal(want) {
		t.Fatalf("next month start = %s, want %s", nextStart, want)
	}
	if want := time.Date(2027, time.February, 1, 0, 0, 0, 0, loc); !nextEnd.Equal(want) {
		t.Fatalf("next month end = %s, want %s", nextEnd, want)
	}
}

func TestFilterMatchingTimedCalendarEvents(t *testing.T) {
	loc := time.FixedZone("local", 0)
	items := []googleCalendarEvent{
		{Summary: "Deep Work", Start: googleCalendarDateTime{DateTime: "2026-06-02T09:00:00Z"}, End: googleCalendarDateTime{DateTime: "2026-06-02T11:00:00Z"}},
		{Summary: "Meeting", Start: googleCalendarDateTime{DateTime: "2026-06-03T09:00:00Z"}, End: googleCalendarDateTime{DateTime: "2026-06-03T10:00:00Z"}},
		{Summary: "Deep Work", Start: googleCalendarDateTime{Date: "2026-06-04"}, End: googleCalendarDateTime{Date: "2026-06-05"}},
	}

	events, err := filterMatchingTimedEvents(items, []string{"Deep Work"}, loc)
	if err != nil {
		t.Fatalf("filterMatchingTimedEvents returned error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	if got := events[0].Duration(); got != 2*time.Hour {
		t.Fatalf("duration = %s, want 2h", got)
	}
}

func TestCalculateWorkloadStatusExcludesTodayFromRecommendation(t *testing.T) {
	loc := time.FixedZone("local", 0)
	now := time.Date(2026, time.June, 9, 12, 0, 0, 0, loc)
	events := []CalendarEvent{
		{Summary: "Deep Work", Start: time.Date(2026, time.June, 8, 9, 0, 0, 0, loc), End: time.Date(2026, time.June, 8, 11, 0, 0, 0, loc)},
		{Summary: "Deep Work", Start: time.Date(2026, time.June, 9, 14, 0, 0, 0, loc), End: time.Date(2026, time.June, 9, 16, 0, 0, 0, loc)},
		{Summary: "Deep Work", Start: time.Date(2026, time.June, 10, 9, 0, 0, 0, loc), End: time.Date(2026, time.June, 10, 12, 0, 0, 0, loc)},
	}

	status := calculateWorkloadStatus(10, 4*time.Hour, events, now)

	if status.Worked != 4*time.Hour {
		t.Fatalf("worked = %s, want 4h", status.Worked)
	}
	if status.Planned != 7*time.Hour {
		t.Fatalf("planned = %s, want 7h", status.Planned)
	}
	if status.RemainingRequired != 6*time.Hour {
		t.Fatalf("remaining required = %s, want 6h", status.RemainingRequired)
	}
	if status.TodayPlanned != 2*time.Hour {
		t.Fatalf("today planned = %s, want 2h", status.TodayPlanned)
	}
	if status.FuturePlanned != 3*time.Hour {
		t.Fatalf("future planned = %s, want 3h", status.FuturePlanned)
	}
	if status.RecommendedToday != 3*time.Hour {
		t.Fatalf("recommended today = %s, want 3h", status.RecommendedToday)
	}
}

func TestFetchWorkedDurationSumsStoppedConfiguredProjects(t *testing.T) {
	oldBase := togglBaseURL
	defer func() { togglBaseURL = oldBase }()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/me/time_entries") {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode([]TogglTimeEntry{
			{ID: 1, Project: 10, Start: time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC), Stop: time.Date(2026, 6, 1, 11, 0, 0, 0, time.UTC)},
			{ID: 2, Project: 20, Start: time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC), Stop: time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)},
			{ID: 3, Project: 10, Start: time.Date(2026, 6, 2, 9, 0, 0, 0, time.UTC)},
		})
	}))
	defer server.Close()
	togglBaseURL = server.URL + "/"

	got, err := fetchWorkedDuration(time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), []int{10})
	if err != nil {
		t.Fatalf("fetchWorkedDuration returned error: %v", err)
	}
	if got != 2*time.Hour {
		t.Fatalf("worked duration = %s, want 2h", got)
	}
}

func TestStartCommandPrintsCalendarHintWhenConfigured(t *testing.T) {
	oldCfg, oldTogglBase, oldCalendarBase, oldNow := cfg, togglBaseURL, calendarBaseURL, nowFunc
	defer func() {
		cfg, togglBaseURL, calendarBaseURL, nowFunc = oldCfg, oldTogglBase, oldCalendarBase, oldNow
	}()

	nowFunc = func() time.Time { return time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC) }
	cfg.Toggl.WorkspaceID = 123
	cfg.Toggl.ProjectID = 10
	cfg.Calendar.ID = "calendar@example.com"
	cfg.Calendar.APIKey = "key"
	cfg.Calendar.EventNames = []string{"Deep Work"}
	cfg.Calendar.MonthlyHourBudget = 10
	cfg.Calendar.ProjectIDs = []int{10}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/api/workspaces/123/time_entries":
			w.WriteHeader(http.StatusOK)
		case r.Method == "GET" && r.URL.Path == "/api/me/time_entries":
			json.NewEncoder(w).Encode([]TogglTimeEntry{{Project: 10, Start: time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC), Stop: time.Date(2026, 6, 1, 13, 0, 0, 0, time.UTC)}})
		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/calendar/calendars/"):
			json.NewEncoder(w).Encode(googleCalendarEventsResponse{Items: []googleCalendarEvent{{Summary: "Deep Work", Start: googleCalendarDateTime{DateTime: "2026-06-10T09:00:00Z"}, End: googleCalendarDateTime{DateTime: "2026-06-10T12:00:00Z"}}}})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()
	togglBaseURL = server.URL + "/api/"
	calendarBaseURL = server.URL + "/calendar/"

	output, err := captureStdout(func() error { return startCmd().RunE(startCmd(), nil) })
	if err != nil {
		t.Fatalf("start command returned error: %v", err)
	}
	if !strings.Contains(output, "Started tracking") {
		t.Fatalf("output missing start confirmation: %q", output)
	}
	if !strings.Contains(output, "Work today to stay on track: 3h") {
		t.Fatalf("output missing calendar hint: %q", output)
	}
}

func TestStopCommandPrintsNoCalendarOutputWhenUnconfigured(t *testing.T) {
	oldCfg, oldTogglBase, oldOpenAIBase, oldNow := cfg, togglBaseURL, openAIBaseURL, nowFunc
	defer func() {
		cfg, togglBaseURL, openAIBaseURL, nowFunc = oldCfg, oldTogglBase, oldOpenAIBase, oldNow
	}()

	nowFunc = func() time.Time { return time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC) }
	cfg.Toggl.WorkspaceID = 123
	cfg.OpenAI.Model = "test"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/api/me/time_entries/current":
			json.NewEncoder(w).Encode(TogglTimeEntry{ID: 99, Workspace: 123, Project: 10, Start: time.Date(2026, 6, 9, 10, 0, 0, 0, time.UTC)})
		case r.Method == "PUT" && r.URL.Path == "/api/workspaces/123/time_entries/99":
			w.WriteHeader(http.StatusOK)
		case r.Method == "POST" && r.URL.Path == "/openai":
			json.NewEncoder(w).Encode(map[string]any{"choices": []map[string]any{{"message": map[string]string{"content": "Did focused work"}}}})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()
	togglBaseURL = server.URL + "/api/"
	openAIBaseURL = server.URL + "/openai"

	output, err := captureStdout(func() error { return stopCmd().RunE(stopCmd(), nil) })
	if err != nil {
		t.Fatalf("stop command returned error: %v", err)
	}
	if strings.Contains(output, "Calendar workload") {
		t.Fatalf("unexpected calendar output when unconfigured: %q", output)
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

func TestStopCommandSplitsUsingForgejoOnlyCommits(t *testing.T) {
	oldCfg, oldTogglBase, oldOpenAIBase, oldNow, oldRemote := cfg, togglBaseURL, openAIBaseURL, nowFunc, gitRemoteURL
	defer func() {
		cfg, togglBaseURL, openAIBaseURL, nowFunc, gitRemoteURL = oldCfg, oldTogglBase, oldOpenAIBase, oldNow, oldRemote
	}()

	nowFunc = func() time.Time { return time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC) }
	cfg = Config{}
	cfg.Toggl.WorkspaceID = 123
	cfg.OpenAI.Model = "test"
	cfg.Git.User = `juda@example.com`
	cfg.Forgejo.APIKey = "token"
	cfg.Forgejo.Repositories = []string{"extras/tooling"}
	cfg.Projects = map[string]ProjectConfig{
		"voicesense": {ProjectID: 204198137, Repositories: []string{filepath.Join(t.TempDir(), "repo")}},
	}
	gitRemoteURL = func(repo string) (string, error) {
		return "https://127.0.0.1/voicesense/voicesense-backend.git", nil
	}

	var capturedPrompt string
	var updatedEntry TogglTimeEntry
	var createdEntries []TogglTimeEntry
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/api/me/time_entries/current":
			json.NewEncoder(w).Encode(TogglTimeEntry{ID: 99, Workspace: 123, Project: 10, Start: time.Date(2026, 6, 9, 10, 0, 0, 0, time.UTC)})
		case r.Method == "PUT" && r.URL.Path == "/api/workspaces/123/time_entries/99":
			json.NewDecoder(r.Body).Decode(&updatedEntry)
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
			json.NewEncoder(w).Encode(map[string]any{"choices": []map[string]any{{"message": map[string]string{"content": "Shipped Forgejo work"}}}})
		case r.Method == "GET" && r.URL.Path == "/api/v1/repos/voicesense/voicesense-backend/commits":
			json.NewEncoder(w).Encode([]map[string]any{{
				"sha": "a",
				"commit": map[string]any{
					"message":   "feat: forgejo only",
					"author":    map[string]any{"name": "Juda Kaleta", "email": "juda@example.com", "date": "2026-06-09T10:30:00Z"},
					"committer": map[string]any{"name": "Juda Kaleta", "email": "juda@example.com", "date": "2026-06-09T10:30:00Z"},
				},
				"author": map[string]any{"login": "juda"},
			}})
		case r.Method == "POST" && r.URL.Path == "/api/workspaces/123/time_entries":
			var entry TogglTimeEntry
			json.NewDecoder(r.Body).Decode(&entry)
			createdEntries = append(createdEntries, entry)
			json.NewEncoder(w).Encode(TogglTimeEntry{ID: 100, Workspace: 123})
		case r.Method == "GET" && r.URL.Path == "/api/v1/repos/extras/tooling/commits":
			json.NewEncoder(w).Encode([]map[string]any{{
				"sha": "z",
				"commit": map[string]any{
					"message":   "chore: unmapped work",
					"author":    map[string]any{"name": "Juda Kaleta", "email": "juda@example.com", "date": "2026-06-09T10:15:00Z"},
					"committer": map[string]any{"name": "Juda Kaleta", "email": "juda@example.com", "date": "2026-06-09T10:15:00Z"},
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
	if !strings.Contains(capturedPrompt, "feat: forgejo only") {
		t.Fatalf("split prompt missing Forgejo commit: %q", capturedPrompt)
	}
	if updatedEntry.Project != 204198137 {
		t.Fatalf("updated entry project = %d, want 204198137 (no phantom split)", updatedEntry.Project)
	}
	if len(createdEntries) != 0 {
		t.Fatalf("unmapped repository produced %d extra splits, want 0: %+v", len(createdEntries), createdEntries)
	}
	if updatedEntry.Description != "Shipped Forgejo work" {
		t.Fatalf("updated description = %q, want the AI result", updatedEntry.Description)
	}
}

func TestStopCommandPrintsCalendarOverviewWhenConfigured(t *testing.T) {
	oldCfg, oldTogglBase, oldCalendarBase, oldNow := cfg, togglBaseURL, calendarBaseURL, nowFunc
	defer func() {
		cfg, togglBaseURL, calendarBaseURL, nowFunc = oldCfg, oldTogglBase, oldCalendarBase, oldNow
	}()

	nowFunc = func() time.Time { return time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC) }
	cfg.Toggl.WorkspaceID = 123
	cfg.Calendar.ID = "calendar@example.com"
	cfg.Calendar.APIKey = "key"
	cfg.Calendar.EventNames = []string{"Deep Work"}
	cfg.Calendar.MonthlyHourBudget = 10
	cfg.Calendar.ProjectIDs = []int{10}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/api/me/time_entries/current":
			json.NewEncoder(w).Encode(TogglTimeEntry{ID: 99, Workspace: 123, Project: 10, Start: time.Date(2026, 6, 9, 10, 0, 0, 0, time.UTC)})
		case r.Method == "PUT" && r.URL.Path == "/api/workspaces/123/time_entries/99":
			w.WriteHeader(http.StatusOK)
		case r.Method == "GET" && r.URL.Path == "/api/me/time_entries":
			json.NewEncoder(w).Encode([]TogglTimeEntry{{Project: 10, Start: time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC), Stop: time.Date(2026, 6, 1, 13, 0, 0, 0, time.UTC)}})
		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/calendar/calendars/"):
			json.NewEncoder(w).Encode(googleCalendarEventsResponse{Items: []googleCalendarEvent{{Summary: "Deep Work", Start: googleCalendarDateTime{DateTime: "2026-06-10T09:00:00Z"}, End: googleCalendarDateTime{DateTime: "2026-06-10T12:00:00Z"}}}})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()
	togglBaseURL = server.URL + "/api/"
	calendarBaseURL = server.URL + "/calendar/"

	output, err := captureStdout(func() error { return stopCmd().RunE(stopCmd(), nil) })
	if err != nil {
		t.Fatalf("stop command returned error: %v", err)
	}
	if !strings.Contains(output, "Calendar workload") {
		t.Fatalf("output missing calendar overview: %q", output)
	}
	if !strings.Contains(output, "Recommended today:        3h") {
		t.Fatalf("output missing recommendation: %q", output)
	}
}

func TestTrackCommandReturnsErrorWhenCalendarUnconfigured(t *testing.T) {
	oldCfg := cfg
	defer func() { cfg = oldCfg }()
	cfg = Config{}

	_, err := captureStdout(func() error { return trackCmd().RunE(trackCmd(), nil) })
	if err == nil {
		t.Fatal("track command should error when calendar workload tracking is not configured")
	}
	if !strings.Contains(err.Error(), "calendar workload tracking is not configured") {
		t.Fatalf("error = %q, want calendar workload configuration message", err.Error())
	}
}

func TestTrackCommandPrintsCalendarOverviewWhenConfigured(t *testing.T) {
	oldCfg, oldTogglBase, oldCalendarBase, oldNow := cfg, togglBaseURL, calendarBaseURL, nowFunc
	defer func() {
		cfg, togglBaseURL, calendarBaseURL, nowFunc = oldCfg, oldTogglBase, oldCalendarBase, oldNow
	}()

	nowFunc = func() time.Time { return time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC) }
	cfg = Config{}
	cfg.Calendar.ID = "calendar@example.com"
	cfg.Calendar.APIKey = "key"
	cfg.Calendar.EventNames = []string{"Deep Work"}
	cfg.Calendar.MonthlyHourBudget = 10
	cfg.Calendar.ProjectIDs = []int{10}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/api/me/time_entries":
			json.NewEncoder(w).Encode([]TogglTimeEntry{{Project: 10, Start: time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC), Stop: time.Date(2026, 6, 1, 13, 0, 0, 0, time.UTC)}})
		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/calendar/calendars/"):
			json.NewEncoder(w).Encode(googleCalendarEventsResponse{Items: []googleCalendarEvent{{Summary: "Deep Work", Start: googleCalendarDateTime{DateTime: "2026-06-10T09:00:00Z"}, End: googleCalendarDateTime{DateTime: "2026-06-10T12:00:00Z"}}}})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()
	togglBaseURL = server.URL + "/api/"
	calendarBaseURL = server.URL + "/calendar/"

	output, err := captureStdout(func() error { return trackCmd().RunE(trackCmd(), nil) })
	if err != nil {
		t.Fatalf("track command returned error: %v", err)
	}
	if !strings.Contains(output, "Calendar workload") {
		t.Fatalf("output missing calendar overview: %q", output)
	}
	if !strings.Contains(output, "Recommended today:        3h") {
		t.Fatalf("output missing recommendation: %q", output)
	}
}

func TestTrackCommandPrintsNextMonthPlannedWorkWhenBudgetMet(t *testing.T) {
	oldCfg, oldTogglBase, oldCalendarBase, oldNow := cfg, togglBaseURL, calendarBaseURL, nowFunc
	defer func() {
		cfg, togglBaseURL, calendarBaseURL, nowFunc = oldCfg, oldTogglBase, oldCalendarBase, oldNow
	}()

	nowFunc = func() time.Time { return time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC) }
	cfg = Config{}
	cfg.Calendar.ID = "calendar@example.com"
	cfg.Calendar.APIKey = "key"
	cfg.Calendar.EventNames = []string{"Deep Work"}
	cfg.Calendar.MonthlyHourBudget = 10
	cfg.Calendar.ProjectIDs = []int{10}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/api/me/time_entries":
			json.NewEncoder(w).Encode([]TogglTimeEntry{{Project: 10, Start: time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC), Stop: time.Date(2026, 6, 1, 13, 0, 0, 0, time.UTC)}})
		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/calendar/calendars/"):
			timeMin := r.URL.Query().Get("timeMin")
			switch {
			case strings.HasPrefix(timeMin, "2026-06-01"):
				json.NewEncoder(w).Encode(googleCalendarEventsResponse{Items: []googleCalendarEvent{{Summary: "Deep Work", Start: googleCalendarDateTime{DateTime: "2026-06-10T09:00:00Z"}, End: googleCalendarDateTime{DateTime: "2026-06-10T12:00:00Z"}}}})
			case strings.HasPrefix(timeMin, "2026-07-01"):
				json.NewEncoder(w).Encode(googleCalendarEventsResponse{Items: []googleCalendarEvent{
					{Summary: "Deep Work", Start: googleCalendarDateTime{DateTime: "2026-07-01T09:00:00Z"}, End: googleCalendarDateTime{DateTime: "2026-07-01T15:00:00Z"}},
					{Summary: "Deep Work", Start: googleCalendarDateTime{DateTime: "2026-07-02T09:00:00Z"}, End: googleCalendarDateTime{DateTime: "2026-07-02T13:00:00Z"}},
				}})
			default:
				t.Fatalf("unexpected calendar timeMin %q", timeMin)
			}
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()
	togglBaseURL = server.URL + "/api/"
	calendarBaseURL = server.URL + "/calendar/"

	output, err := captureStdout(func() error { return trackCmd().RunE(trackCmd(), nil) })
	if err != nil {
		t.Fatalf("track command returned error: %v", err)
	}
	if !strings.Contains(output, "Next month planned work: 10h (meets expected 10.0h)") {
		t.Fatalf("output missing next-month meets status: %q", output)
	}
}

func TestTrackCommandPrintsNextMonthPlannedWorkWhenBelowBudget(t *testing.T) {
	oldCfg, oldTogglBase, oldCalendarBase, oldNow := cfg, togglBaseURL, calendarBaseURL, nowFunc
	defer func() {
		cfg, togglBaseURL, calendarBaseURL, nowFunc = oldCfg, oldTogglBase, oldCalendarBase, oldNow
	}()

	nowFunc = func() time.Time { return time.Date(2026, 12, 31, 12, 0, 0, 0, time.UTC) }
	cfg = Config{}
	cfg.Calendar.ID = "calendar@example.com"
	cfg.Calendar.APIKey = "key"
	cfg.Calendar.EventNames = []string{"Deep Work"}
	cfg.Calendar.MonthlyHourBudget = 10
	cfg.Calendar.ProjectIDs = []int{10}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/api/me/time_entries":
			json.NewEncoder(w).Encode([]TogglTimeEntry{})
		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/calendar/calendars/"):
			timeMin := r.URL.Query().Get("timeMin")
			switch {
			case strings.HasPrefix(timeMin, "2026-12-01"):
				json.NewEncoder(w).Encode(googleCalendarEventsResponse{})
			case strings.HasPrefix(timeMin, "2027-01-01"):
				json.NewEncoder(w).Encode(googleCalendarEventsResponse{Items: []googleCalendarEvent{{Summary: "Deep Work", Start: googleCalendarDateTime{DateTime: "2027-01-03T09:00:00Z"}, End: googleCalendarDateTime{DateTime: "2027-01-03T13:00:00Z"}}}})
			default:
				t.Fatalf("unexpected calendar timeMin %q", timeMin)
			}
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()
	togglBaseURL = server.URL + "/api/"
	calendarBaseURL = server.URL + "/calendar/"

	output, err := captureStdout(func() error { return trackCmd().RunE(trackCmd(), nil) })
	if err != nil {
		t.Fatalf("track command returned error: %v", err)
	}
	if !strings.Contains(output, "Next month planned work: 4h (below expected 10.0h)") {
		t.Fatalf("output missing next-month below status: %q", output)
	}
}

func TestTrackCommandUsesSingleNowForCurrentAndNextMonth(t *testing.T) {
	oldCfg, oldTogglBase, oldCalendarBase, oldNow := cfg, togglBaseURL, calendarBaseURL, nowFunc
	defer func() {
		cfg, togglBaseURL, calendarBaseURL, nowFunc = oldCfg, oldTogglBase, oldCalendarBase, oldNow
	}()

	calls := 0
	nowFunc = func() time.Time {
		calls++
		if calls == 1 {
			return time.Date(2026, 6, 30, 23, 59, 59, 0, time.UTC)
		}
		return time.Date(2026, 7, 1, 0, 0, 1, 0, time.UTC)
	}
	cfg = Config{}
	cfg.Calendar.ID = "calendar@example.com"
	cfg.Calendar.APIKey = "key"
	cfg.Calendar.EventNames = []string{"Deep Work"}
	cfg.Calendar.MonthlyHourBudget = 10
	cfg.Calendar.ProjectIDs = []int{10}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/api/me/time_entries":
			json.NewEncoder(w).Encode([]TogglTimeEntry{})
		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/calendar/calendars/"):
			timeMin := r.URL.Query().Get("timeMin")
			switch {
			case strings.HasPrefix(timeMin, "2026-06-01"):
				json.NewEncoder(w).Encode(googleCalendarEventsResponse{})
			case strings.HasPrefix(timeMin, "2026-07-01"):
				json.NewEncoder(w).Encode(googleCalendarEventsResponse{Items: []googleCalendarEvent{{Summary: "Deep Work", Start: googleCalendarDateTime{DateTime: "2026-07-03T09:00:00Z"}, End: googleCalendarDateTime{DateTime: "2026-07-03T19:00:00Z"}}}})
			default:
				t.Fatalf("unexpected calendar timeMin %q", timeMin)
			}
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()
	togglBaseURL = server.URL + "/api/"
	calendarBaseURL = server.URL + "/calendar/"

	output, err := captureStdout(func() error { return trackCmd().RunE(trackCmd(), nil) })
	if err != nil {
		t.Fatalf("track command returned error: %v", err)
	}
	if !strings.Contains(output, "Next month planned work: 10h (meets expected 10.0h)") {
		t.Fatalf("output missing next-month status derived from initial now: %q", output)
	}
}

func TestTrackCommandReturnsCalendarErrors(t *testing.T) {
	oldCfg, oldCalendarBase, oldNow := cfg, calendarBaseURL, nowFunc
	defer func() {
		cfg, calendarBaseURL, nowFunc = oldCfg, oldCalendarBase, oldNow
	}()

	nowFunc = func() time.Time { return time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC) }
	cfg = Config{}
	cfg.Calendar.ID = "calendar@example.com"
	cfg.Calendar.APIKey = "key"
	cfg.Calendar.EventNames = []string{"Deep Work"}
	cfg.Calendar.MonthlyHourBudget = 10
	cfg.Calendar.ProjectIDs = []int{10}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "calendar unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	calendarBaseURL = server.URL + "/calendar/"

	_, err := captureStdout(func() error { return trackCmd().RunE(trackCmd(), nil) })
	if err == nil {
		t.Fatal("track command should return calendar errors")
	}
	if !strings.Contains(err.Error(), "Google Calendar API error: 503 Service Unavailable") {
		t.Fatalf("error = %q, want calendar failure reason", err.Error())
	}
}

func TestRootCommandIncludesTrack(t *testing.T) {
	cmd := rootCmd()
	for _, command := range cmd.Commands() {
		if command.Name() == "track" {
			if !strings.Contains(command.Short, "calendar") && !strings.Contains(command.Short, "budget") {
				t.Fatalf("track short description should mention calendar or budget, got %q", command.Short)
			}
			return
		}
	}
	t.Fatal("root command missing track subcommand")
}

func TestRootCommandIncludesFillEmptyDescriptions(t *testing.T) {
	cmd := rootCmd()
	for _, command := range cmd.Commands() {
		if command.Name() == "fill-empty-descriptions" {
			if !strings.Contains(command.Short, "empty") || !strings.Contains(command.Short, "propose") {
				t.Fatalf("fill-empty-descriptions short description should mention empty descriptions and proposals, got %q", command.Short)
			}
			return
		}
	}
	t.Fatal("root command missing fill-empty-descriptions subcommand")
}

func TestFillEmptyDescriptionsUsesDefaultSevenDayRange(t *testing.T) {
	oldCfg, oldTogglBase, oldNow := cfg, togglBaseURL, nowFunc
	defer func() {
		cfg, togglBaseURL, nowFunc = oldCfg, oldTogglBase, oldNow
	}()

	now := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	nowFunc = func() time.Time { return now }
	cfg = Config{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/api/me/time_entries" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
		if got, want := r.URL.Query().Get("start_date"), now.AddDate(0, 0, -7).Format(time.RFC3339); got != want {
			t.Fatalf("start_date = %q, want %q", got, want)
		}
		if got, want := r.URL.Query().Get("end_date"), now.Format(time.RFC3339); got != want {
			t.Fatalf("end_date = %q, want %q", got, want)
		}
		json.NewEncoder(w).Encode([]TogglTimeEntry{})
	}))
	defer server.Close()
	togglBaseURL = server.URL + "/api/"

	output, err := captureStdout(func() error { return fillEmptyDescriptionsCmd().Execute() })
	if err != nil {
		t.Fatalf("fill-empty-descriptions returned error: %v", err)
	}
	if !strings.Contains(output, "No empty descriptions found") {
		t.Fatalf("output missing no-empty message: %q", output)
	}
}

func TestFillEmptyDescriptionsUsesExplicitRangeAndRejectsPartialRange(t *testing.T) {
	oldCfg, oldTogglBase := cfg, togglBaseURL
	defer func() { cfg, togglBaseURL = oldCfg, oldTogglBase }()
	cfg = Config{}

	start := "2026-06-01T00:00:00Z"
	end := "2026-06-08T00:00:00Z"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("start_date"); got != start {
			t.Fatalf("start_date = %q, want %q", got, start)
		}
		if got := r.URL.Query().Get("end_date"); got != end {
			t.Fatalf("end_date = %q, want %q", got, end)
		}
		json.NewEncoder(w).Encode([]TogglTimeEntry{})
	}))
	defer server.Close()
	togglBaseURL = server.URL + "/api/"

	cmd := fillEmptyDescriptionsCmd()
	cmd.SetArgs([]string{"--start", start, "--end", end})
	if _, err := captureStdout(func() error { return cmd.Execute() }); err != nil {
		t.Fatalf("fill-empty-descriptions with explicit range returned error: %v", err)
	}

	cmd = fillEmptyDescriptionsCmd()
	cmd.SetArgs([]string{"--start", start})
	_, err := captureStdout(func() error { return cmd.Execute() })
	if err == nil {
		t.Fatal("fill-empty-descriptions should reject one-sided range flags")
	}
	if !strings.Contains(err.Error(), "--start and --end must be provided together") {
		t.Fatalf("error = %q, want paired range flag message", err.Error())
	}

	cmd = fillEmptyDescriptionsCmd()
	cmd.SetArgs([]string{"--start", "not-a-date", "--end", end})
	_, err = captureStdout(func() error { return cmd.Execute() })
	if err == nil {
		t.Fatal("fill-empty-descriptions should reject invalid RFC3339 start flag")
	}
	if !strings.Contains(err.Error(), "invalid --start") {
		t.Fatalf("error = %q, want invalid --start message", err.Error())
	}
}

func TestIsFillableEmptyDescriptionEntryRequiresBlankDescriptionAndClosedWindow(t *testing.T) {
	start := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)
	stop := start.Add(time.Hour)

	cases := []struct {
		name  string
		entry TogglTimeEntry
		want  bool
	}{
		{name: "empty stopped", entry: TogglTimeEntry{Start: start, Stop: stop}, want: true},
		{name: "whitespace stopped", entry: TogglTimeEntry{Start: start, Stop: stop, Description: " \n\t "}, want: true},
		{name: "described stopped", entry: TogglTimeEntry{Start: start, Stop: stop, Description: "Already done"}, want: false},
		{name: "no stop", entry: TogglTimeEntry{Start: start}, want: false},
		{name: "stop before start", entry: TogglTimeEntry{Start: start, Stop: start.Add(-time.Minute)}, want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isFillableEmptyDescriptionEntry(tc.entry); got != tc.want {
				t.Fatalf("isFillableEmptyDescriptionEntry() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestFillEmptyDescriptionsConfirmsUpdatesAndReportsCounts(t *testing.T) {
	oldCfg, oldTogglBase := cfg, togglBaseURL
	defer func() { cfg, togglBaseURL = oldCfg, oldTogglBase }()
	cfg = Config{}
	cfg.Toggl.WorkspaceID = 123

	start := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)
	entries := []TogglTimeEntry{
		{ID: 1, Workspace: 123, Project: 10, Start: start, Stop: start.Add(time.Hour)},
		{ID: 2, Workspace: 123, Project: 10, Start: start, Stop: start.Add(time.Hour), Description: "Already filled"},
		{ID: 3, Workspace: 123, Project: 10, Start: start, Stop: start.Add(time.Hour), Description: "  "},
		{ID: 4, Workspace: 123, Project: 10, Start: start},
		{ID: 5, Workspace: 123, Project: 10, Start: start, Stop: start.Add(time.Hour)},
	}
	var updated []TogglTimeEntry
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/api/me/time_entries":
			json.NewEncoder(w).Encode(entries)
		case r.Method == "PUT" && r.URL.Path == "/api/workspaces/123/time_entries/1":
			var entry TogglTimeEntry
			if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
				t.Fatalf("failed to decode update body: %v", err)
			}
			updated = append(updated, entry)
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()
	togglBaseURL = server.URL + "/api/"

	cmd := fillEmptyDescriptionsCmd()
	cmd.SetArgs([]string{"--start", "2026-06-01T00:00:00Z", "--end", "2026-06-08T00:00:00Z"})
	output, err := captureStdoutWithStdin("Manual one\ny\nManual two\nn\n\n", func() error { return cmd.Execute() })
	if err != nil {
		t.Fatalf("fill-empty-descriptions returned error: %v", err)
	}

	if len(updated) != 1 {
		t.Fatalf("updated %d entries, want 1", len(updated))
	}
	if got := updated[0].Description; got != "Manual one" {
		t.Fatalf("updated description = %q, want Manual one", got)
	}
	for _, want := range []string{
		"Entry 1 (2026-06-01T09:00:00Z - 2026-06-01T10:00:00Z)",
		"Proposed description: Manual one",
		"Skipped entry 5.",
		"Scanned entries: 5",
		"Blank descriptions: 4",
		"Proposed descriptions: 2",
		"Updated entries: 1",
		"Skipped entries: 3",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q: %q", want, output)
		}
	}
}

func TestFillEmptyDescriptionsIncludesForgejoActivity(t *testing.T) {
	oldCfg, oldTogglBase, oldOpenAIBase := cfg, togglBaseURL, openAIBaseURL
	defer func() { cfg, togglBaseURL, openAIBaseURL = oldCfg, oldTogglBase, oldOpenAIBase }()

	start := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)
	cfg = Config{}
	cfg.Toggl.WorkspaceID = 123
	cfg.OpenAI.Model = "test"
	cfg.Git.User = `juda@example.com`
	cfg.Forgejo.APIKey = "token"
	cfg.Forgejo.Repositories = []string{"voicesense/voicesense-backend"}

	var capturedPrompt string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/api/me/time_entries":
			json.NewEncoder(w).Encode([]TogglTimeEntry{{ID: 1, Workspace: 123, Project: 10, Start: start, Stop: start.Add(time.Hour)}})
		case r.Method == "PUT" && r.URL.Path == "/api/workspaces/123/time_entries/1":
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
			json.NewEncoder(w).Encode(map[string]any{"choices": []map[string]any{{"message": map[string]string{"content": "Remote work summary"}}}})
		case r.Method == "GET" && r.URL.Path == "/api/v1/repos/voicesense/voicesense-backend/commits":
			json.NewEncoder(w).Encode([]map[string]any{{
				"sha": "a",
				"commit": map[string]any{
					"message":   "feat: forgejo fill work",
					"author":    map[string]any{"name": "Juda Kaleta", "email": "juda@example.com", "date": "2026-06-01T09:30:00Z"},
					"committer": map[string]any{"name": "Juda Kaleta", "email": "juda@example.com", "date": "2026-06-01T09:30:00Z"},
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

	cmd := fillEmptyDescriptionsCmd()
	cmd.SetArgs([]string{"--start", "2026-06-01T00:00:00Z", "--end", "2026-06-08T00:00:00Z"})
	if _, err := captureStdoutWithStdin("y\n", func() error { return cmd.Execute() }); err != nil {
		t.Fatalf("fill-empty-descriptions returned error: %v", err)
	}
	if !strings.Contains(capturedPrompt, "Forgejo commits:") {
		t.Fatalf("fill-empty prompt missing Forgejo activity: %q", capturedPrompt)
	}
}

func TestRepairSummariesIncludesForgejoActivity(t *testing.T) {
	oldCfg, oldTogglBase, oldOpenAIBase := cfg, togglBaseURL, openAIBaseURL
	defer func() { cfg, togglBaseURL, openAIBaseURL = oldCfg, oldTogglBase, oldOpenAIBase }()

	start := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)
	cfg = Config{}
	cfg.Toggl.WorkspaceID = 123
	cfg.OpenAI.Model = "test"
	cfg.Git.User = `juda@example.com`
	cfg.Forgejo.APIKey = "token"
	cfg.Forgejo.Repositories = []string{"voicesense/voicesense-backend"}

	var capturedPrompt string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/api/me/time_entries":
			json.NewEncoder(w).Encode([]TogglTimeEntry{{ID: 1, Workspace: 123, Project: 10, Start: start, Stop: start.Add(time.Hour), Description: badSummaryText}})
		case r.Method == "PUT" && r.URL.Path == "/api/workspaces/123/time_entries/1":
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
			json.NewEncoder(w).Encode(map[string]any{"choices": []map[string]any{{"message": map[string]string{"content": "Repaired summary"}}}})
		case r.Method == "GET" && r.URL.Path == "/api/v1/repos/voicesense/voicesense-backend/commits":
			json.NewEncoder(w).Encode([]map[string]any{{
				"sha": "a",
				"commit": map[string]any{
					"message":   "feat: forgejo repair work",
					"author":    map[string]any{"name": "Juda Kaleta", "email": "juda@example.com", "date": "2026-06-01T09:30:00Z"},
					"committer": map[string]any{"name": "Juda Kaleta", "email": "juda@example.com", "date": "2026-06-01T09:30:00Z"},
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

	cmd := repairSummariesCmd()
	cmd.SetArgs([]string{"--start", "2026-06-01T00:00:00Z", "--end", "2026-06-08T00:00:00Z"})
	output, err := captureStdout(func() error { return cmd.Execute() })
	if err != nil {
		t.Fatalf("repair-summaries returned error: %v", err)
	}
	if !strings.Contains(capturedPrompt, "Forgejo commits:") {
		t.Fatalf("repair prompt missing Forgejo activity: %q", capturedPrompt)
	}
	if !strings.Contains(output, "Repaired 1 entries.") {
		t.Fatalf("output missing repair count: %q", output)
	}
}

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

func TestResolveForgejoRepositoriesUpgradesMappingForExplicitEntry(t *testing.T) {
	oldCfg, oldRemote := cfg, gitRemoteURL
	defer func() { cfg, gitRemoteURL = oldCfg, oldRemote }()

	cfg = Config{}
	cfg.Forgejo.URL = "https://infra.senseloom.com"
	cfg.Forgejo.APIKey = "token"
	cfg.Forgejo.Repositories = []string{"voicesense/voicesense-backend"}
	cfg.Projects = map[string]ProjectConfig{
		"voicesense": {
			ProjectID:    204198137,
			Repositories: []string{"/repos/voicesense-backend"},
		},
	}
	gitRemoteURL = func(repo string) (string, error) {
		return "https://infra.senseloom.com/voicesense/voicesense-backend.git", nil
	}

	repos := resolveForgejoRepositories()
	if len(repos) != 1 {
		t.Fatalf("resolved %d repositories, want 1: %+v", len(repos), repos)
	}
	if repos[0].ProjectName != "voicesense" || repos[0].ProjectID != 204198137 {
		t.Fatalf("mapping = %q/%d, want voicesense/204198137", repos[0].ProjectName, repos[0].ProjectID)
	}
}

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

func forgejoCommitItem(date string) map[string]any {
	return map[string]any{
		"sha": "x",
		"commit": map[string]any{
			"message":   "feat: work",
			"author":    map[string]any{"name": "Juda Kaleta", "email": "juda@example.com", "date": date},
			"committer": map[string]any{"name": "Juda Kaleta", "email": "juda@example.com", "date": date},
		},
		"author": map[string]any{"login": "juda"},
	}
}

func TestFetchForgejoCommitsPagesUntilWindowExhausted(t *testing.T) {
	oldCfg := cfg
	defer func() { cfg = oldCfg }()
	cfg = Config{}
	cfg.Forgejo.APIKey = "token"
	cfg.Git.User = `juda@example.com`

	entry := TogglTimeEntry{
		Start: time.Date(2026, 6, 9, 10, 0, 0, 0, time.UTC),
		Stop:  time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC),
	}

	pageOne := make([]map[string]any, 0, forgejoPageSize)
	for i := 0; i < forgejoPageSize; i++ {
		pageOne = append(pageOne, forgejoCommitItem("2026-06-09T11:00:00Z"))
	}
	pageTwo := make([]map[string]any, 0, forgejoPageSize)
	for i := 0; i < forgejoPageSize; i++ {
		pageTwo = append(pageTwo, forgejoCommitItem("2026-06-08T09:00:00Z"))
	}

	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		switch r.URL.Query().Get("page") {
		case "1":
			json.NewEncoder(w).Encode(pageOne)
		case "2":
			json.NewEncoder(w).Encode(pageTwo)
		default:
			t.Errorf("unexpected page %s", r.URL.Query().Get("page"))
			json.NewEncoder(w).Encode([]map[string]any{})
		}
	}))
	defer server.Close()
	cfg.Forgejo.URL = server.URL

	commits, err := fetchForgejoCommits(entry, ForgejoRepo{Owner: "o", Name: "r"})
	if err != nil {
		t.Fatalf("fetchForgejoCommits returned error: %v", err)
	}
	if len(commits) != forgejoPageSize {
		t.Fatalf("got %d commits, want %d", len(commits), forgejoPageSize)
	}
	if requests != 2 {
		t.Fatalf("requests = %d, want 2 (stop once oldest predates the window)", requests)
	}
}

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

func TestCollectForgejoActivityDisabledReturnsEmpty(t *testing.T) {
	oldCfg := cfg
	defer func() { cfg = oldCfg }()
	cfg = Config{}

	activity := collectForgejoActivity(TogglTimeEntry{})
	if len(activity.SplitCommits) != 0 || len(activity.PromptSections) != 0 {
		t.Fatalf("disabled integration should return empty activity: %+v", activity)
	}
}

func TestCollectForgejoActivityKeepsCommitsWhenIssueSearchFails(t *testing.T) {
	oldCfg, oldRemote := cfg, gitRemoteURL
	defer func() { cfg, gitRemoteURL = oldCfg, oldRemote }()

	entry := TogglTimeEntry{
		Start: time.Date(2026, 6, 9, 10, 0, 0, 0, time.UTC),
		Stop:  time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC),
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/repos/voicesense/voicesense-backend/commits":
			json.NewEncoder(w).Encode([]map[string]any{{
				"sha": "a",
				"commit": map[string]any{
					"message":   "feat: quota checks",
					"author":    map[string]any{"name": "Juda Kaleta", "email": "juda@example.com", "date": "2026-06-09T10:30:00Z"},
					"committer": map[string]any{"name": "Juda Kaleta", "email": "juda@example.com", "date": "2026-06-09T10:30:00Z"},
				},
				"author": map[string]any{"login": "juda"},
			}})
		case "/api/v1/repos/issues/search":
			w.WriteHeader(http.StatusInternalServerError)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	cfg = Config{}
	cfg.Git.User = `juda@example.com`
	cfg.Forgejo.URL = server.URL
	cfg.Forgejo.APIKey = "token"
	cfg.Projects = map[string]ProjectConfig{
		"voicesense": {ProjectID: 204198137, Repositories: []string{"/repos/voicesense-backend"}},
	}
	gitRemoteURL = func(repo string) (string, error) {
		return "https://127.0.0.1/voicesense/voicesense-backend.git", nil
	}

	activity := collectForgejoActivity(entry)
	if len(activity.SplitCommits) != 1 {
		t.Fatalf("SplitCommits = %d, want 1 preserved commit: %+v", len(activity.SplitCommits), activity.SplitCommits)
	}
	if joined := strings.Join(activity.PromptSections, "\n"); !strings.Contains(joined, "feat: quota checks") {
		t.Fatalf("commit activity not preserved after issue failure: %q", joined)
	}
}

func TestCollectForgejoActivityKeepsUnmappedRepoInPromptOnly(t *testing.T) {
	oldCfg, oldRemote := cfg, gitRemoteURL
	defer func() { cfg, gitRemoteURL = oldCfg, oldRemote }()

	entry := TogglTimeEntry{
		Start: time.Date(2026, 6, 9, 10, 0, 0, 0, time.UTC),
		Stop:  time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC),
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/repos/voicesense/voicesense-web/commits":
			json.NewEncoder(w).Encode([]map[string]any{{
				"sha": "b",
				"commit": map[string]any{
					"message":   "feat: remote only",
					"author":    map[string]any{"name": "Juda Kaleta", "email": "juda@example.com", "date": "2026-06-09T10:45:00Z"},
					"committer": map[string]any{"name": "Juda Kaleta", "email": "juda@example.com", "date": "2026-06-09T10:45:00Z"},
				},
				"author": map[string]any{"login": "juda"},
			}})
		case "/api/v1/repos/issues/search":
			json.NewEncoder(w).Encode([]map[string]any{})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	cfg = Config{}
	cfg.Git.User = `juda@example.com`
	cfg.Forgejo.URL = server.URL
	cfg.Forgejo.APIKey = "token"
	cfg.Forgejo.Repositories = []string{"voicesense/voicesense-web"}

	activity := collectForgejoActivity(entry)
	if len(activity.SplitCommits) != 0 {
		t.Fatalf("unmapped repository must not produce splits: %+v", activity.SplitCommits)
	}
	if joined := strings.Join(activity.PromptSections, "\n"); !strings.Contains(joined, "feat: remote only") {
		t.Fatalf("unmapped repository should contribute prompt text: %q", joined)
	}
}

func TestCollectForgejoActivityMakesNoRequestsWithoutResolvedRepositories(t *testing.T) {
	oldCfg := cfg
	defer func() { cfg = oldCfg }()

	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	cfg = Config{}
	cfg.Forgejo.URL = server.URL
	cfg.Forgejo.APIKey = "token"

	activity := collectForgejoActivity(TogglTimeEntry{})
	if requests != 0 {
		t.Fatalf("requests = %d, want 0", requests)
	}
	if len(activity.SplitCommits) != 0 || len(activity.PromptSections) != 0 {
		t.Fatalf("expected empty activity: %+v", activity)
	}
}

func TestForgejoRequestReturnsErrorOnFailureStatus(t *testing.T) {
	oldCfg := cfg
	defer func() { cfg = oldCfg }()
	cfg = Config{}
	cfg.Forgejo.APIKey = "token"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()
	cfg.Forgejo.URL = server.URL

	_, err := forgejoRequest("repos/o/r/commits")
	if err == nil {
		t.Fatal("expected an error for a non-2xx response")
	}
	if !strings.Contains(err.Error(), "Forgejo API error") {
		t.Fatalf("error = %q, want it to contain %q", err.Error(), "Forgejo API error")
	}
}

func captureStdout(fn func() error) (string, error) {
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		return "", err
	}
	os.Stdout = w
	err = fn()
	w.Close()
	os.Stdout = oldStdout
	var buf bytes.Buffer
	_, copyErr := io.Copy(&buf, r)
	if err != nil {
		return buf.String(), err
	}
	return buf.String(), copyErr
}

func captureStdoutWithStdin(input string, fn func() error) (string, error) {
	oldStdin := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		return "", err
	}
	if _, err := w.WriteString(input); err != nil {
		return "", err
	}
	w.Close()
	os.Stdin = r
	defer func() { os.Stdin = oldStdin }()
	return captureStdout(fn)
}

func TestCreateTimeEntryDerivesDurationFromFormattedTimes(t *testing.T) {
	oldCfg, oldBase := cfg, togglBaseURL
	defer func() { cfg, togglBaseURL = oldCfg, oldBase }()
	cfg = Config{}

	var body struct {
		Start    string `json:"start"`
		Stop     string `json:"stop"`
		Duration int    `json:"duration"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		json.NewEncoder(w).Encode(TogglTimeEntry{ID: 5, Workspace: 123})
	}))
	defer server.Close()
	togglBaseURL = server.URL + "/api/"

	start := time.Date(2026, 6, 9, 10, 0, 0, 900_000_000, time.UTC)
	stop := time.Date(2026, 6, 9, 11, 30, 15, 400_000_000, time.UTC)
	split := ProjectSplit{ProjectName: "cortex", ProjectID: 219802887, Start: start, Stop: stop, Duration: stop.Sub(start)}

	if _, err := createTimeEntry(split, "desc", 123); err != nil {
		t.Fatalf("createTimeEntry returned error: %v", err)
	}

	parsedStart, err := time.Parse(time.RFC3339, body.Start)
	if err != nil {
		t.Fatalf("start %q: %v", body.Start, err)
	}
	parsedStop, err := time.Parse(time.RFC3339, body.Stop)
	if err != nil {
		t.Fatalf("stop %q: %v", body.Stop, err)
	}
	if parsedStart.Nanosecond() != 0 || parsedStop.Nanosecond() != 0 {
		t.Fatalf("timestamps must be whole seconds: start=%s stop=%s", body.Start, body.Stop)
	}
	if want := int(parsedStop.Sub(parsedStart).Seconds()); body.Duration != want {
		t.Fatalf("duration %d does not match stop-start %d (start=%s stop=%s)", body.Duration, want, body.Start, body.Stop)
	}
}

func TestTogglRequestCheckedIncludesErrorBody(t *testing.T) {
	oldBase := togglBaseURL
	defer func() { togglBaseURL = oldBase }()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, "Stop and duration mismatch")
	}))
	defer server.Close()
	togglBaseURL = server.URL + "/api/"

	_, err := togglRequestChecked("POST", "workspaces/1/time_entries", nil)
	if err == nil {
		t.Fatal("expected an error for a 400 response")
	}
	if !strings.Contains(err.Error(), "Stop and duration mismatch") {
		t.Fatalf("error = %q, want the response body detail", err.Error())
	}
}

func TestLogfWritesToConfiguredWriter(t *testing.T) {
	oldOut := logOut
	defer func() { logOut = oldOut }()

	var buf bytes.Buffer
	logOut = &buf
	logf("hello %s", "world")

	if !strings.Contains(buf.String(), "hello world") {
		t.Fatalf("log output = %q, want message", buf.String())
	}
}

func TestLogfIsSilentWhenDisabled(t *testing.T) {
	oldOut := logOut
	defer func() { logOut = oldOut }()
	logOut = nil

	logf("must not panic or write anywhere")
}
