package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
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
