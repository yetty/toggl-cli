package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var version = "dev"

// --- Types and constants ---

type Config struct {
	Toggl struct {
		APIKey      string `yaml:"api_key"`
		WorkspaceID int    `yaml:"workspace_id"`
		ProjectID   int    `yaml:"project_id"`
	} `yaml:"toggl"`
	OpenAI struct {
		APIKey string `yaml:"api_key"`
		Model  string `yaml:"model"`
	} `yaml:"openai"`
	Git struct {
		User string `yaml:"user"`
	} `yaml:"git"`
	Repositories []string                 `yaml:"repositories"`
	Projects     map[string]ProjectConfig `yaml:"projects"`
	Forgejo      ForgejoConfig            `yaml:"forgejo"`
	Calendar     CalendarConfig           `yaml:"calendar"`
}

type CalendarConfig struct {
	ID                string   `yaml:"id"`
	APIKey            string   `yaml:"api_key"`
	EventNames        []string `yaml:"event_names"`
	MonthlyHourBudget float64  `yaml:"monthly_hour_budget"`
	ProjectIDs        []int    `yaml:"project_ids"`
}

type ForgejoConfig struct {
	URL          string   `yaml:"url"`
	APIKey       string   `yaml:"api_key"`
	Repositories []string `yaml:"repositories"`
}

func (c ForgejoConfig) Enabled() bool {
	return c.URL != "" && c.APIKey != ""
}

func (c CalendarConfig) Enabled() bool {
	return c.ID != "" && c.APIKey != "" && len(c.EventNames) > 0 && c.MonthlyHourBudget > 0
}

type ProjectConfig struct {
	ProjectID    int      `yaml:"project_id"`
	Repositories []string `yaml:"repositories"`
}

type TogglTimeEntry struct {
	ID          int       `json:"id"`
	Start       time.Time `json:"start"`
	Stop        time.Time `json:"stop"`
	Workspace   int       `json:"workspace_id"`
	Project     int       `json:"project_id"`
	Description string    `json:"description"`
}

type ProjectCommit struct {
	ProjectName string
	ProjectID   int
	RepoName    string
	RepoPath    string
	Subject     string
	Time        time.Time
}

type ProjectSplit struct {
	EntryID     int
	ProjectName string
	ProjectID   int
	Description string
	Start       time.Time
	Stop        time.Time
	Duration    time.Duration
}

type CalendarEvent struct {
	Summary string
	Start   time.Time
	End     time.Time
}

func (e CalendarEvent) Duration() time.Duration {
	if e.End.Before(e.Start) {
		return 0
	}
	return e.End.Sub(e.Start)
}

type WorkloadStatus struct {
	Worked            time.Duration
	Planned           time.Duration
	ElapsedPlanned    time.Duration
	TodayPlanned      time.Duration
	FuturePlanned     time.Duration
	RemainingRequired time.Duration
	RecommendedToday  time.Duration
	MonthlyHourBudget float64
}

type googleCalendarDateTime struct {
	DateTime string `json:"dateTime"`
	Date     string `json:"date"`
}

type googleCalendarEvent struct {
	Summary string                 `json:"summary"`
	Start   googleCalendarDateTime `json:"start"`
	End     googleCalendarDateTime `json:"end"`
}

type googleCalendarEventsResponse struct {
	Items []googleCalendarEvent `json:"items"`
}

const worklogFile = "toggl-worklog.txt"

const badSummaryText = "Sure! Please provide the details of your commits so I can generate a concise summary for you."

const openAISystemPrompt = `You summarize Git commits and work logs for Toggl time entry descriptions.
Infer the one to three highest-impact outcomes from the supplied commits and work log.
For each selected outcome, state a concrete action and the affected capability, feature, or defect.
Include an issue identifier only when it identifies a selected high-impact outcome; do not list every issue or pull request.
Prefer substantive implementation work over process noise. Ignore OpenSpec proposals and archives, merge commits, dependency updates, localisation churn, and configuration-only changes unless no substantive work is present.
Write one readable line, maximum 280 characters. Use concise clauses separated by semicolons where useful.
Avoid vague phrases such as "OpenSpec changes", "various updates", "implemented improvements", and "merged PRs" unless unavoidable.`

// --- Global vars ---

var (
	cfg             Config
	cfgPath         string
	togglBaseURL    = "https://api.track.toggl.com/api/v9/"
	openAIBaseURL   = "https://api.openai.com/v1/chat/completions"
	calendarBaseURL = "https://www.googleapis.com/calendar/v3/"
	nowFunc         = time.Now
)

// --- Config loading ---

func loadConfig() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	cfgPath = filepath.Join(homeDir, ".toggl.yaml")
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return err
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return err
	}
	if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		cfg.OpenAI.APIKey = key
	}
	if key := os.Getenv("FORGEJO_API_KEY"); key != "" {
		cfg.Forgejo.APIKey = key
	}
	return nil
}

// --- API clients ---

func togglRequest(method, path string, body io.Reader) ([]byte, error) {
	req, _ := http.NewRequest(method, togglBaseURL+path, body)
	req.SetBasicAuth(cfg.Toggl.APIKey, "api_token")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

func togglRequestChecked(method, path string, body io.Reader) ([]byte, error) {
	req, _ := http.NewRequest(method, togglBaseURL+path, body)
	req.SetBasicAuth(cfg.Toggl.APIKey, "api_token")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("Toggl API error: %s", resp.Status)
	}
	return data, nil
}

func openAISummarize(text string) (string, error) {
	reqBody := map[string]interface{}{
		"model": cfg.OpenAI.Model,
		"messages": []map[string]string{
			{
				"role":    "system",
				"content": openAISystemPrompt,
			},
			{
				"role": "user",
				"content": `Here are the commits I made in my repositories:

` + text + `

Generate a one-line Toggl description.`,
			},
		},
	}

	buf, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", openAIBaseURL, bytes.NewBuffer(buf))
	req.Header.Set("Authorization", "Bearer "+cfg.OpenAI.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("OpenAI API error: %s", resp.Status)
	}
	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("no completion returned")
	}
	return result.Choices[0].Message.Content, nil
}

// --- Helper functions ---

func worklogPath() string {
	return filepath.Join(os.TempDir(), worklogFile)
}

func expandRepoPath(path string) string {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if path == "~" {
		return home
	}
	return filepath.Join(home, strings.TrimPrefix(path, "~/"))
}

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
	if len(repos) == 0 {
		return nil, nil
	}

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

func currentMonthBounds(now time.Time) (time.Time, time.Time) {
	loc := now.Location()
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
	return start, start.AddDate(0, 1, 0)
}

func nextMonthBounds(now time.Time) (time.Time, time.Time) {
	_, currentEnd := currentMonthBounds(now)
	return currentEnd, currentEnd.AddDate(0, 1, 0)
}

func currentDayBounds(now time.Time) (time.Time, time.Time) {
	loc := now.Location()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	return start, start.AddDate(0, 0, 1)
}

func configuredCalendarProjectIDs() []int {
	if len(cfg.Calendar.ProjectIDs) > 0 {
		return cfg.Calendar.ProjectIDs
	}
	ids := map[int]bool{}
	if cfg.Toggl.ProjectID != 0 {
		ids[cfg.Toggl.ProjectID] = true
	}
	for _, project := range cfg.Projects {
		if project.ProjectID != 0 {
			ids[project.ProjectID] = true
		}
	}
	result := make([]int, 0, len(ids))
	for id := range ids {
		result = append(result, id)
	}
	sort.Ints(result)
	return result
}

func parseCalendarDateTime(value string, loc *time.Location) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, err
	}
	return parsed.In(loc), nil
}

func filterMatchingTimedEvents(items []googleCalendarEvent, eventNames []string, loc *time.Location) ([]CalendarEvent, error) {
	allowed := map[string]bool{}
	for _, name := range eventNames {
		allowed[name] = true
	}

	events := make([]CalendarEvent, 0, len(items))
	for _, item := range items {
		if !allowed[item.Summary] || item.Start.DateTime == "" || item.End.DateTime == "" {
			continue
		}
		start, err := parseCalendarDateTime(item.Start.DateTime, loc)
		if err != nil {
			return nil, err
		}
		end, err := parseCalendarDateTime(item.End.DateTime, loc)
		if err != nil {
			return nil, err
		}
		if !end.After(start) {
			continue
		}
		events = append(events, CalendarEvent{Summary: item.Summary, Start: start, End: end})
	}
	return events, nil
}

func fetchCalendarEvents(start, end time.Time) ([]CalendarEvent, error) {
	params := url.Values{}
	params.Set("key", cfg.Calendar.APIKey)
	params.Set("timeMin", start.Format(time.RFC3339))
	params.Set("timeMax", end.Format(time.RFC3339))
	params.Set("singleEvents", "true")
	params.Set("orderBy", "startTime")
	path := fmt.Sprintf("calendars/%s/events?%s", url.PathEscape(cfg.Calendar.ID), params.Encode())
	req, err := http.NewRequest("GET", calendarBaseURL+path, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("Google Calendar API error: %s", resp.Status)
	}
	var result googleCalendarEventsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return filterMatchingTimedEvents(result.Items, cfg.Calendar.EventNames, start.Location())
}

func fetchWorkedDuration(start, end time.Time, projectIDs []int) (time.Duration, error) {
	entries, err := listTimeEntries(start.Format(time.RFC3339), end.Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	allowed := map[int]bool{}
	for _, id := range projectIDs {
		allowed[id] = true
	}
	var total time.Duration
	for _, entry := range entries {
		if !allowed[entry.Project] || entry.Stop.IsZero() || !entry.Stop.After(entry.Start) {
			continue
		}
		total += entry.Stop.Sub(entry.Start)
	}
	return total, nil
}

func calculateWorkloadStatus(monthlyHourBudget float64, worked time.Duration, events []CalendarEvent, now time.Time) WorkloadStatus {
	dayStart, dayEnd := currentDayBounds(now)
	budget := time.Duration(monthlyHourBudget * float64(time.Hour))
	status := WorkloadStatus{Worked: worked, MonthlyHourBudget: monthlyHourBudget}
	for _, event := range events {
		duration := event.Duration()
		status.Planned += duration
		if event.End.After(dayStart) && event.Start.Before(dayEnd) {
			status.TodayPlanned += duration
		} else if !event.Start.Before(dayEnd) {
			status.FuturePlanned += duration
		} else {
			status.ElapsedPlanned += duration
		}
	}
	status.RemainingRequired = budget - worked
	if status.RemainingRequired < 0 {
		status.RemainingRequired = 0
	}
	status.RecommendedToday = status.RemainingRequired - status.FuturePlanned
	if status.RecommendedToday < 0 {
		status.RecommendedToday = 0
	}
	return status
}

func sumCalendarEventDurations(events []CalendarEvent) time.Duration {
	var total time.Duration
	for _, event := range events {
		total += event.Duration()
	}
	return total
}

func loadWorkloadStatus() (WorkloadStatus, error) {
	return loadWorkloadStatusAt(nowFunc())
}

func loadWorkloadStatusAt(now time.Time) (WorkloadStatus, error) {
	monthStart, monthEnd := currentMonthBounds(now)
	events, err := fetchCalendarEvents(monthStart, monthEnd)
	if err != nil {
		return WorkloadStatus{}, fmt.Errorf("calendar workload: %w", err)
	}
	worked, err := fetchWorkedDuration(monthStart, monthEnd, configuredCalendarProjectIDs())
	if err != nil {
		return WorkloadStatus{}, fmt.Errorf("calendar workload: %w", err)
	}
	return calculateWorkloadStatus(cfg.Calendar.MonthlyHourBudget, worked, events, now), nil
}

func loadNextMonthPlannedDuration() (time.Duration, error) {
	return loadNextMonthPlannedDurationAt(nowFunc())
}

func loadNextMonthPlannedDurationAt(now time.Time) (time.Duration, error) {
	nextMonthStart, nextMonthEnd := nextMonthBounds(now)
	events, err := fetchCalendarEvents(nextMonthStart, nextMonthEnd)
	if err != nil {
		return 0, fmt.Errorf("calendar workload: %w", err)
	}
	return sumCalendarEventDurations(events), nil
}

func formatDurationHours(d time.Duration) string {
	if d%time.Hour == 0 {
		return fmt.Sprintf("%dh", int(d/time.Hour))
	}
	return fmt.Sprintf("%.1fh", d.Hours())
}

func printCalendarStartHint(status WorkloadStatus) {
	if status.RecommendedToday == 0 {
		fmt.Println("Work today to stay on track: no additional work needed")
		return
	}
	fmt.Printf("Work today to stay on track: %s\n", formatDurationHours(status.RecommendedToday))
}

func printCalendarOverview(status WorkloadStatus) {
	fmt.Println("┌─ Calendar workload ─────────────────────────")
	fmt.Printf("│ Worked this month:        %s / %.1fh\n", formatDurationHours(status.Worked), status.MonthlyHourBudget)
	fmt.Printf("│ Planned calendar time:    %s\n", formatDurationHours(status.Planned))
	fmt.Printf("│ Remaining required work:  %s\n", formatDurationHours(status.RemainingRequired))
	fmt.Printf("│ Future planned work:      %s\n", formatDurationHours(status.FuturePlanned))
	fmt.Printf("│ Recommended today:        %s\n", formatDurationHours(status.RecommendedToday))
	fmt.Println("└──────────────────────────────────────────────")
}

func printTrackCalendarOverview(status WorkloadStatus, nextMonthPlanned time.Duration) {
	result := "below"
	if nextMonthPlanned >= time.Duration(status.MonthlyHourBudget*float64(time.Hour)) {
		result = "meets"
	}
	fmt.Println("┌─ Calendar workload ─────────────────────────")
	fmt.Printf("│ Worked this month:        %s / %.1fh\n", formatDurationHours(status.Worked), status.MonthlyHourBudget)
	fmt.Printf("│ Planned calendar time:    %s\n", formatDurationHours(status.Planned))
	fmt.Printf("│ Remaining required work:  %s\n", formatDurationHours(status.RemainingRequired))
	fmt.Printf("│ Future planned work:      %s\n", formatDurationHours(status.FuturePlanned))
	fmt.Printf("│ Recommended today:        %s\n", formatDurationHours(status.RecommendedToday))
	fmt.Printf("│ Next month planned work: %s (%s expected %.1fh)\n", formatDurationHours(nextMonthPlanned), result, status.MonthlyHourBudget)
	fmt.Println("└──────────────────────────────────────────────")
}

func printCalendarResult(status WorkloadStatus, err error) {
	if err != nil {
		fmt.Printf("Calendar workload error: %v\n", err)
		return
	}
	printCalendarOverview(status)
}

func gitLogBaseArgs(repo string) []string {
	return []string{"-C", repo, "log", "--all"}
}

func getCurrentEntry() (TogglTimeEntry, error) {
	data, err := togglRequest("GET", "me/time_entries/current", nil)
	if err != nil {
		return TogglTimeEntry{}, err
	}
	var entry TogglTimeEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return TogglTimeEntry{}, err
	}
	if entry.ID == 0 {
		return TogglTimeEntry{}, fmt.Errorf("no running timer")
	}
	return entry, nil
}

func stopEntry(entry *TogglTimeEntry) error {
	entry.Stop = nowFunc()
	body, _ := json.Marshal(entry)
	_, err := togglRequestChecked("PUT", fmt.Sprintf("workspaces/%d/time_entries/%d", entry.Workspace, entry.ID), bytes.NewBuffer(body))
	return err
}

func createTimeEntry(split ProjectSplit, description string, workspaceID int) (int, error) {
	reqBody := struct {
		WorkspaceID int    `json:"workspace_id"`
		ProjectID   int    `json:"project_id"`
		Description string `json:"description"`
		CreatedWith string `json:"created_with"`
		Start       string `json:"start"`
		Stop        string `json:"stop"`
		Duration    int    `json:"duration"`
	}{
		WorkspaceID: workspaceID,
		ProjectID:   split.ProjectID,
		Description: description,
		CreatedWith: "toggl-cli",
		Start:       split.Start.Format(time.RFC3339),
		Stop:        split.Stop.Format(time.RFC3339),
		Duration:    int(split.Duration.Seconds()),
	}

	body, _ := json.Marshal(reqBody)
	data, err := togglRequestChecked("POST", fmt.Sprintf("workspaces/%d/time_entries", workspaceID), bytes.NewBuffer(body))
	if err != nil {
		return 0, err
	}
	var created TogglTimeEntry
	if err := json.Unmarshal(data, &created); err != nil {
		return 0, err
	}
	return created.ID, nil
}

func collectCommits(entry TogglTimeEntry) []string {
	var allCommits []string
	for _, repoPath := range cfg.Repositories {
		repo := expandRepoPath(repoPath)
		args := append(gitLogBaseArgs(repo),
			fmt.Sprintf("--since=%s", entry.Start.Format(time.RFC3339)),
			fmt.Sprintf("--until=%s", entry.Stop.Format(time.RFC3339)),
			fmt.Sprintf("--author=%s", cfg.Git.User),
			"--pretty=format:%s",
		)
		cmd := exec.Command("git", args...)
		out, err := cmd.Output()
		if err != nil {
			name := filepath.Base(repo)
			var reason string
			if _, statErr := os.Stat(repo); os.IsNotExist(statErr) {
				reason = "path not found"
			} else {
				var exitErr *exec.ExitError
				if errors.As(err, &exitErr) && strings.Contains(string(exitErr.Stderr), "not a git repository") {
					reason = "not a git repository"
				} else {
					reason = err.Error()
				}
			}
			fmt.Fprintf(os.Stderr, "Warning: skipping %s (%s)\n", name, reason)
			continue
		}
		if len(out) > 0 {
			repoName := filepath.Base(repo)
			allCommits = append(allCommits, fmt.Sprintf("Repository: %s\n%s", repoName, string(out)))
		}
	}
	return allCommits
}

func collectProjectCommits(entry TogglTimeEntry) []ProjectCommit {
	var commits []ProjectCommit
	projectNames := make([]string, 0, len(cfg.Projects))
	for projectName := range cfg.Projects {
		projectNames = append(projectNames, projectName)
	}
	sort.Strings(projectNames)

	for _, projectName := range projectNames {
		project := cfg.Projects[projectName]
		for _, repoPath := range project.Repositories {
			repo := expandRepoPath(repoPath)
			args := append(gitLogBaseArgs(repo),
				fmt.Sprintf("--since=%s", entry.Start.Format(time.RFC3339)),
				fmt.Sprintf("--until=%s", entry.Stop.Format(time.RFC3339)),
				fmt.Sprintf("--author=%s", cfg.Git.User),
				"--pretty=format:%cI%x09%s",
			)
			cmd := exec.Command("git", args...)
			out, err := cmd.Output()
			if err != nil {
				name := filepath.Base(repo)
				var reason string
				if _, statErr := os.Stat(repo); os.IsNotExist(statErr) {
					reason = "path not found"
				} else {
					var exitErr *exec.ExitError
					if errors.As(err, &exitErr) && strings.Contains(string(exitErr.Stderr), "not a git repository") {
						reason = "not a git repository"
					} else {
						reason = err.Error()
					}
				}
				fmt.Fprintf(os.Stderr, "Warning: skipping %s (%s)\n", name, reason)
				continue
			}

			for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
				if line == "" {
					continue
				}
				parts := strings.SplitN(line, "\t", 2)
				if len(parts) != 2 {
					fmt.Fprintf(os.Stderr, "Warning: skipping malformed git log line in %s: %s\n", filepath.Base(repo), line)
					continue
				}
				commitTime, err := time.Parse(time.RFC3339, parts[0])
				if err != nil {
					fmt.Fprintf(os.Stderr, "Warning: skipping commit with malformed timestamp in %s: %s\n", filepath.Base(repo), parts[0])
					continue
				}
				commits = append(commits, ProjectCommit{
					ProjectName: projectName,
					ProjectID:   project.ProjectID,
					RepoName:    filepath.Base(repo),
					RepoPath:    repo,
					Subject:     parts[1],
					Time:        commitTime,
				})
			}
		}
	}
	sort.Slice(commits, func(i, j int) bool {
		if !commits[i].Time.Equal(commits[j].Time) {
			return commits[i].Time.Before(commits[j].Time)
		}
		if commits[i].ProjectName != commits[j].ProjectName {
			return commits[i].ProjectName < commits[j].ProjectName
		}
		if commits[i].ProjectID != commits[j].ProjectID {
			return commits[i].ProjectID < commits[j].ProjectID
		}
		if commits[i].RepoName != commits[j].RepoName {
			return commits[i].RepoName < commits[j].RepoName
		}
		return commits[i].Subject < commits[j].Subject
	})
	return commits
}

func formatProjectCommits(commits []ProjectCommit) []string {
	formatted := make([]string, 0, len(commits))
	for _, commit := range commits {
		formatted = append(formatted, fmt.Sprintf("Project: %s\nRepository: %s\n%s", commit.ProjectName, commit.RepoName, commit.Subject))
	}
	return formatted
}

func filterProjectCommits(commits []ProjectCommit, split ProjectSplit) []ProjectCommit {
	var filtered []ProjectCommit
	for _, commit := range commits {
		if commit.ProjectName == split.ProjectName && commit.ProjectID == split.ProjectID {
			filtered = append(filtered, commit)
		}
	}
	return filtered
}

func describeProjectSplits(splits []ProjectSplit, commits []ProjectCommit) ([]ProjectSplit, bool) {
	for i, split := range splits {
		projectCommits := filterProjectCommits(commits, split)
		promptText := buildPromptText(formatProjectCommits(projectCommits), "")
		description := getDescription(promptText)
		if description == "" {
			return splits, false
		}
		splits[i].Description = description
	}
	return splits, true
}

func calculateProjectSplits(entry TogglTimeEntry, commits []ProjectCommit) []ProjectSplit {
	if len(commits) == 0 {
		return nil
	}

	sortedCommits := append([]ProjectCommit(nil), commits...)
	sort.SliceStable(sortedCommits, func(i, j int) bool {
		if !sortedCommits[i].Time.Equal(sortedCommits[j].Time) {
			return sortedCommits[i].Time.Before(sortedCommits[j].Time)
		}
		if sortedCommits[i].ProjectName != sortedCommits[j].ProjectName {
			return sortedCommits[i].ProjectName < sortedCommits[j].ProjectName
		}
		if sortedCommits[i].ProjectID != sortedCommits[j].ProjectID {
			return sortedCommits[i].ProjectID < sortedCommits[j].ProjectID
		}
		if sortedCommits[i].RepoName != sortedCommits[j].RepoName {
			return sortedCommits[i].RepoName < sortedCommits[j].RepoName
		}
		return sortedCommits[i].Subject < sortedCommits[j].Subject
	})

	type projectKey struct {
		ProjectName string
		ProjectID   int
	}

	projectOrder := make([]projectKey, 0, len(sortedCommits))
	durations := map[projectKey]time.Duration{}
	seen := map[projectKey]bool{}
	for i, commit := range sortedCommits {
		key := projectKey{ProjectName: commit.ProjectName, ProjectID: commit.ProjectID}
		if !seen[key] {
			seen[key] = true
			projectOrder = append(projectOrder, key)
		}

		start := entry.Start
		if i > 0 {
			previous := sortedCommits[i-1]
			start = previous.Time.Add(commit.Time.Sub(previous.Time) / 2)
		}
		stop := entry.Stop
		if i < len(sortedCommits)-1 {
			next := sortedCommits[i+1]
			stop = commit.Time.Add(next.Time.Sub(commit.Time) / 2)
		}
		durations[key] += stop.Sub(start)
	}

	splits := make([]ProjectSplit, 0, len(projectOrder))
	start := entry.Start
	for _, project := range projectOrder {
		stop := start.Add(durations[project])
		splits = append(splits, ProjectSplit{
			ProjectName: project.ProjectName,
			ProjectID:   project.ProjectID,
			Start:       start,
			Stop:        stop,
			Duration:    stop.Sub(start),
		})
		start = stop
	}
	splits[len(splits)-1].Stop = entry.Stop
	splits[len(splits)-1].Duration = entry.Stop.Sub(splits[len(splits)-1].Start)
	return splits
}

func readWorklog() string {
	if data, err := os.ReadFile(worklogPath()); err == nil {
		return string(data)
	}
	return ""
}

func buildPromptText(commits []string, worklog string) string {
	var sections []string
	if len(commits) > 0 {
		sections = append(sections, fmt.Sprintf("Git commits:\n%s", strings.Join(commits, "\n\n")))
	}
	if strings.TrimSpace(worklog) != "" {
		sections = append(sections, fmt.Sprintf("Work log:\n%s", strings.TrimSpace(worklog)))
	}
	return strings.Join(sections, "\n\n")
}

func promptManualDescription(prompt string) string {
	return promptManualDescriptionFromReader(prompt, bufio.NewReader(os.Stdin))
}

func promptManualDescriptionFromReader(prompt string, reader *bufio.Reader) string {
	fmt.Print(prompt)
	line, _ := reader.ReadString('\n')
	summary := strings.TrimSpace(line)
	if summary == "" {
		fmt.Println("Skipped description.")
	}
	return summary
}

func getDescription(promptText string) string {
	return getDescriptionFromReader(promptText, bufio.NewReader(os.Stdin))
}

func getDescriptionFromReader(promptText string, reader *bufio.Reader) string {
	if strings.TrimSpace(promptText) == "" {
		return promptManualDescriptionFromReader("No commits or work log entries found. Enter description manually (or press Enter to skip): ", reader)
	}

	prompt := fmt.Sprintf("Summarize these git commits and work log:\n\n%s", promptText)
	summary, err := openAISummarize(prompt)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		fmt.Printf("\nCollected data:\n%s\n\n", promptText)
		summary = promptManualDescriptionFromReader("AI summarization failed. Enter description manually (or press Enter to skip): ", reader)
		if summary == "" {
			fmt.Println("Skipped description. Check your OpenAI API key in ~/.toggl.yaml")
		}
	}
	return summary
}

func isBadSummary(description string) bool {
	description = strings.TrimSpace(description)
	if description == badSummaryText {
		return true
	}
	return description == "Sure! Please provide the details of your commits so I can generate a summary for your Toggl time entry."
}

func updateEntryDescription(entry TogglTimeEntry, description string) error {
	entry.Description = description
	workspaceID := entry.Workspace
	if workspaceID == 0 {
		workspaceID = cfg.Toggl.WorkspaceID
	}
	body, _ := json.Marshal(entry)
	_, err := togglRequestChecked("PUT", fmt.Sprintf("workspaces/%d/time_entries/%d", workspaceID, entry.ID), bytes.NewBuffer(body))
	return err
}

func applyProjectSplits(entry TogglTimeEntry, splits []ProjectSplit, description string) ([]ProjectSplit, error) {
	if len(splits) == 0 {
		return nil, updateEntryDescription(entry, description)
	}
	workspaceID := entry.Workspace
	if workspaceID == 0 {
		workspaceID = cfg.Toggl.WorkspaceID
	}

	first := splits[0]
	entry.Project = first.ProjectID
	entry.Start = first.Start
	entry.Stop = first.Stop
	firstDescription := first.Description
	if firstDescription == "" {
		firstDescription = description
	}
	entry.Description = firstDescription
	if err := updateEntryDescription(entry, firstDescription); err != nil {
		return nil, fmt.Errorf("original split update failed for project %s (%d): %w", first.ProjectName, first.ProjectID, err)
	}
	splits[0].EntryID = entry.ID

	for i := 1; i < len(splits); i++ {
		splitDescription := splits[i].Description
		if splitDescription == "" {
			splitDescription = description
		}
		entryID, err := createTimeEntry(splits[i], splitDescription, workspaceID)
		if err != nil {
			return nil, fmt.Errorf("split creation failed for project %s (%d): %w", splits[i].ProjectName, splits[i].ProjectID, err)
		}
		splits[i].EntryID = entryID
	}
	return splits, nil
}

func printProjectSplitSummary(splits []ProjectSplit) {
	fmt.Println("Stopped tracking. Split into:")
	for _, split := range splits {
		fmt.Printf("- entry %d | %s (project %d) | %s -> %s | %s\n", split.EntryID, split.ProjectName, split.ProjectID, split.Start.Format(time.RFC3339), split.Stop.Format(time.RFC3339), split.Duration)
	}
	fmt.Println("Summary saved.")
}

func printEntrySummary(entry TogglTimeEntry) {
	fmt.Println("Stopped tracking. Entry:")
	fmt.Printf("- entry %d | project %d | %s -> %s | %s\n", entry.ID, entry.Project, entry.Start.Format(time.RFC3339), entry.Stop.Format(time.RFC3339), entry.Stop.Sub(entry.Start))
	fmt.Println("Summary saved.")
}

func listTimeEntries(start, end string) ([]TogglTimeEntry, error) {
	path := fmt.Sprintf("me/time_entries?start_date=%s&end_date=%s", url.QueryEscape(start), url.QueryEscape(end))
	data, err := togglRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}
	var entries []TogglTimeEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func isFillableEmptyDescriptionEntry(entry TogglTimeEntry) bool {
	return strings.TrimSpace(entry.Description) == "" && !entry.Stop.IsZero() && entry.Stop.After(entry.Start)
}

func confirmBackfillDescription(reader *bufio.Reader) bool {
	fmt.Print("Save this description? [y/N]: ")
	line, _ := reader.ReadString('\n')
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes"
}

func fillEmptyDescriptionsCmd() *cobra.Command {
	var startDate, endDate string
	cmd := &cobra.Command{
		Use:   "fill-empty-descriptions",
		Short: "Find empty Toggl descriptions and propose replacements",
		RunE: func(cmd *cobra.Command, args []string) error {
			if (startDate == "") != (endDate == "") {
				return fmt.Errorf("--start and --end must be provided together")
			}
			if startDate == "" {
				now := nowFunc()
				startDate = now.AddDate(0, 0, -7).Format(time.RFC3339)
				endDate = now.Format(time.RFC3339)
			}
			if _, err := time.Parse(time.RFC3339, startDate); err != nil {
				return fmt.Errorf("invalid --start: %w", err)
			}
			if _, err := time.Parse(time.RFC3339, endDate); err != nil {
				return fmt.Errorf("invalid --end: %w", err)
			}

			entries, err := listTimeEntries(startDate, endDate)
			if err != nil {
				return err
			}

			reader := bufio.NewReader(os.Stdin)
			scanned := len(entries)
			blank := 0
			proposed := 0
			updated := 0
			skipped := 0

			for _, entry := range entries {
				if strings.TrimSpace(entry.Description) != "" {
					continue
				}
				blank++
				if !isFillableEmptyDescriptionEntry(entry) {
					skipped++
					continue
				}

				commits := collectCommits(entry)
				promptText := buildPromptText(commits, "")
				description := getDescriptionFromReader(promptText, reader)
				if description == "" {
					fmt.Printf("Skipped entry %d.\n", entry.ID)
					skipped++
					continue
				}

				proposed++
				fmt.Printf("Entry %d (%s - %s)\n", entry.ID, entry.Start.Format(time.RFC3339), entry.Stop.Format(time.RFC3339))
				fmt.Printf("Proposed description: %s\n", description)
				if !confirmBackfillDescription(reader) {
					skipped++
					continue
				}

				if err := updateEntryDescription(entry, description); err != nil {
					return err
				}
				updated++
			}

			if blank == 0 {
				fmt.Println("No empty descriptions found.")
			}
			fmt.Printf("Scanned entries: %d\n", scanned)
			fmt.Printf("Blank descriptions: %d\n", blank)
			fmt.Printf("Proposed descriptions: %d\n", proposed)
			fmt.Printf("Updated entries: %d\n", updated)
			fmt.Printf("Skipped entries: %d\n", skipped)
			return nil
		},
	}
	cmd.Flags().StringVar(&startDate, "start", "", "start date/time to scan (RFC3339)")
	cmd.Flags().StringVar(&endDate, "end", "", "end date/time to scan (RFC3339)")
	return cmd
}

// --- Commands ---

func whoamiCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Show current Toggl user info",
		RunE: func(cmd *cobra.Command, args []string) error {
			data, err := togglRequest("GET", "me", nil)
			if err != nil {
				return err
			}

			var resp struct {
				Data struct {
					ID       int    `json:"id"`
					Email    string `json:"email"`
					Fullname string `json:"fullname"`
				} `json:"data"`
			}
			if err := json.Unmarshal(data, &resp); err != nil {
				return err
			}

			fmt.Printf("User ID: %d\nFull Name: %s\nEmail: %s\n", resp.Data.ID, resp.Data.Fullname, resp.Data.Email)
			return nil
		},
	}
}

func projectsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "projects",
		Short: "List all projects with their IDs",
		RunE: func(cmd *cobra.Command, args []string) error {
			path := fmt.Sprintf("workspaces/%d/projects", cfg.Toggl.WorkspaceID)
			data, err := togglRequest("GET", path, nil)
			if err != nil {
				return err
			}

			var projects []struct {
				ID   int    `json:"id"`
				Name string `json:"name"`
			}

			if err := json.Unmarshal(data, &projects); err != nil {
				return err
			}

			if len(projects) == 0 {
				fmt.Println("No projects found in this workspace.")
				return nil
			}

			fmt.Println("Projects:")
			for _, p := range projects {
				fmt.Printf("ID: %d  Name: %s\n", p.ID, p.Name)
			}

			return nil
		},
	}
}

func startCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Start Toggl tracker",
		RunE: func(cmd *cobra.Command, args []string) error {
			reqBody := struct {
				WorkspaceID int    `json:"workspace_id"`
				ProjectID   int    `json:"project_id"`
				CreatedWith string `json:"created_with"`
				Start       string `json:"start"`
				Duration    int    `json:"duration"`
			}{
				WorkspaceID: cfg.Toggl.WorkspaceID,
				ProjectID:   cfg.Toggl.ProjectID,
				CreatedWith: "toggl-cli",
				Start:       nowFunc().UTC().Format(time.RFC3339),
				Duration:    -1,
			}

			body, _ := json.Marshal(reqBody)
			_, err := togglRequest("POST", fmt.Sprintf("workspaces/%d/time_entries", cfg.Toggl.WorkspaceID), bytes.NewBuffer(body))
			if err != nil {
				return err
			}

			fmt.Println("Started tracking")
			if cfg.Calendar.Enabled() {
				status, err := loadWorkloadStatus()
				if err != nil {
					fmt.Printf("Calendar workload error: %v\n", err)
				} else {
					printCalendarStartHint(status)
				}
			}
			return nil
		},
	}
}

func stopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop Toggl tracker and summarize commits",
		RunE: func(cmd *cobra.Command, args []string) error {
			entry, err := getCurrentEntry()
			if err != nil {
				return err
			}

			if err := stopEntry(&entry); err != nil {
				return err
			}

			var calendarStatus WorkloadStatus
			var calendarErr error
			if cfg.Calendar.Enabled() {
				calendarStatus, calendarErr = loadWorkloadStatus()
			}

			var projectCommits []ProjectCommit
			if len(cfg.Projects) > 0 {
				projectCommits = collectProjectCommits(entry)
			}
			worklog := readWorklog()

			if len(cfg.Projects) > 0 {
				splits := calculateProjectSplits(entry, projectCommits)
				if len(splits) > 0 {
					var ok bool
					splits, ok = describeProjectSplits(splits, projectCommits)
					if !ok {
						os.Remove(worklogPath())
						fmt.Println("Stopped tracking. No summary saved.")
						if cfg.Calendar.Enabled() {
							printCalendarResult(calendarStatus, calendarErr)
						}
						return nil
					}
					appliedSplits, err := applyProjectSplits(entry, splits, "")
					if err != nil {
						return err
					}
					os.Remove(worklogPath())
					printProjectSplitSummary(appliedSplits)
					if cfg.Calendar.Enabled() {
						printCalendarResult(calendarStatus, calendarErr)
					}
					return nil
				}
			}

			commits := collectCommits(entry)
			promptText := buildPromptText(commits, worklog)
			description := getDescription(promptText)
			if description == "" {
				os.Remove(worklogPath())
				fmt.Println("Stopped tracking. No summary saved.")
				if cfg.Calendar.Enabled() {
					printCalendarResult(calendarStatus, calendarErr)
				}
				return nil
			}

			if err := updateEntryDescription(entry, description); err != nil {
				return err
			}

			os.Remove(worklogPath())

			printEntrySummary(entry)
			if cfg.Calendar.Enabled() {
				printCalendarResult(calendarStatus, calendarErr)
			}
			return nil
		},
	}
}

func trackCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "track",
		Short: "Show calendar workload budget status",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !cfg.Calendar.Enabled() {
				return fmt.Errorf("calendar workload tracking is not configured")
			}

			now := nowFunc()
			status, err := loadWorkloadStatusAt(now)
			if err != nil {
				return err
			}
			nextMonthPlanned, err := loadNextMonthPlannedDurationAt(now)
			if err != nil {
				return err
			}
			printTrackCalendarOverview(status, nextMonthPlanned)
			return nil
		},
	}
}

func repairSummariesCmd() *cobra.Command {
	var startDate, endDate string
	cmd := &cobra.Command{
		Use:   "repair-summaries",
		Short: "Repair Toggl entries saved with the empty-commit AI response",
		RunE: func(cmd *cobra.Command, args []string) error {
			if startDate == "" || endDate == "" {
				return fmt.Errorf("--start and --end are required")
			}

			entries, err := listTimeEntries(startDate, endDate)
			if err != nil {
				return err
			}

			repaired := 0
			for _, entry := range entries {
				if !isBadSummary(entry.Description) {
					continue
				}

				fmt.Printf("Repairing entry %d (%s - %s)\n", entry.ID, entry.Start.Format(time.RFC3339), entry.Stop.Format(time.RFC3339))
				commits := collectCommits(entry)
				promptText := buildPromptText(commits, "")
				description := getDescription(promptText)
				if description == "" {
					fmt.Printf("Skipped entry %d.\n", entry.ID)
					continue
				}

				if err := updateEntryDescription(entry, description); err != nil {
					return err
				}
				repaired++
			}

			fmt.Printf("Repaired %d entries.\n", repaired)
			return nil
		},
	}
	cmd.Flags().StringVar(&startDate, "start", "", "start date/time to scan (RFC3339)")
	cmd.Flags().StringVar(&endDate, "end", "", "end date/time to scan (RFC3339)")
	return cmd
}

func logCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "log [MESSAGE]",
		Short: "Log a message for current work session",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			message := args[0]
			timestamp := time.Now().Format(time.RFC3339)
			logEntry := fmt.Sprintf("[%s] %s\n", timestamp, message)

			file, err := os.OpenFile(worklogPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			if err != nil {
				return fmt.Errorf("failed to open log file: %v", err)
			}
			defer file.Close()

			if _, err := file.WriteString(logEntry); err != nil {
				return fmt.Errorf("failed to write to log file: %v", err)
			}

			fmt.Printf("Logged: %s\n", message)
			return nil
		},
	}
}

// --- Main ---

func rootCmd() *cobra.Command {
	root := &cobra.Command{Use: "toggl", Version: version}
	root.AddCommand(startCmd(), stopCmd(), trackCmd(), logCmd(), whoamiCmd(), projectsCmd(), repairSummariesCmd(), fillEmptyDescriptionsCmd())
	return root
}

func main() {
	if err := loadConfig(); err != nil {
		fmt.Println("Error loading config:", err)
		os.Exit(1)
	}

	if err := rootCmd().Execute(); err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
}
