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
	ProjectName string
	ProjectID   int
	Start       time.Time
	Stop        time.Time
	Duration    time.Duration
}

const worklogFile = "toggl-worklog.txt"

const badSummaryText = "Sure! Please provide the details of your commits so I can generate a concise summary for you."

const openAISystemPrompt = `You are an expert assistant that summarizes Git commits into concise, human-readable descriptions for Toggl time entries.
Always clearly state what work was done, in which repository/project, in 1-3 short sentences per repository.
Focus on actions taken, features implemented, bugs fixed, and avoid listing commit hashes.`

// --- Global vars ---

var (
	cfg           Config
	cfgPath       string
	togglBaseURL  = "https://api.track.toggl.com/api/v9/"
	openAIBaseURL = "https://api.openai.com/v1/chat/completions"
	nowFunc       = time.Now
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

Generate a concise summary suitable for a Toggl time entry.`,
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

func createTimeEntry(split ProjectSplit, description string, workspaceID int) error {
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
	_, err := togglRequestChecked("POST", fmt.Sprintf("workspaces/%d/time_entries", workspaceID), bytes.NewBuffer(body))
	return err
}

func collectCommits(entry TogglTimeEntry) []string {
	var allCommits []string
	for _, repoPath := range cfg.Repositories {
		repo := expandRepoPath(repoPath)
		cmd := exec.Command(
			"git",
			"-C", repo,
			"log",
			fmt.Sprintf("--since=%s", entry.Start.Format(time.RFC3339)),
			fmt.Sprintf("--until=%s", entry.Stop.Format(time.RFC3339)),
			fmt.Sprintf("--author=%s", cfg.Git.User),
			"--pretty=format:%s",
		)
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
			cmd := exec.Command(
				"git",
				"-C", repo,
				"log",
				fmt.Sprintf("--since=%s", entry.Start.Format(time.RFC3339)),
				fmt.Sprintf("--until=%s", entry.Stop.Format(time.RFC3339)),
				fmt.Sprintf("--author=%s", cfg.Git.User),
				"--pretty=format:%cI%x09%s",
			)
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
	fmt.Print(prompt)
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()
	summary := strings.TrimSpace(scanner.Text())
	if summary == "" {
		fmt.Println("Skipped description.")
	}
	return summary
}

func getDescription(promptText string) string {
	if strings.TrimSpace(promptText) == "" {
		return promptManualDescription("No commits or work log entries found. Enter description manually (or press Enter to skip): ")
	}

	prompt := fmt.Sprintf("Summarize these git commits and work log:\n\n%s", promptText)
	summary, err := openAISummarize(prompt)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		fmt.Printf("\nCollected data:\n%s\n\n", promptText)
		summary = promptManualDescription("AI summarization failed. Enter description manually (or press Enter to skip): ")
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

func applyProjectSplits(entry TogglTimeEntry, splits []ProjectSplit, description string) error {
	if len(splits) == 0 {
		return updateEntryDescription(entry, description)
	}
	workspaceID := entry.Workspace
	if workspaceID == 0 {
		workspaceID = cfg.Toggl.WorkspaceID
	}

	first := splits[0]
	entry.Project = first.ProjectID
	entry.Start = first.Start
	entry.Stop = first.Stop
	entry.Description = description
	if err := updateEntryDescription(entry, description); err != nil {
		return fmt.Errorf("original split update failed for project %s (%d): %w", first.ProjectName, first.ProjectID, err)
	}

	for _, split := range splits[1:] {
		if err := createTimeEntry(split, description, workspaceID); err != nil {
			return fmt.Errorf("split creation failed for project %s (%d): %w", split.ProjectName, split.ProjectID, err)
		}
	}
	return nil
}

func printProjectSplitSummary(splits []ProjectSplit) {
	fmt.Println("Stopped tracking. Split into:")
	for _, split := range splits {
		fmt.Printf("- %s: %s\n", split.ProjectName, split.Duration)
	}
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
				Start:       time.Now().UTC().Format(time.RFC3339),
				Duration:    -1,
			}

			body, _ := json.Marshal(reqBody)
			_, err := togglRequest("POST", fmt.Sprintf("workspaces/%d/time_entries", cfg.Toggl.WorkspaceID), bytes.NewBuffer(body))
			if err != nil {
				return err
			}

			fmt.Println("Started tracking")
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

			commits := collectCommits(entry)
			var projectCommits []ProjectCommit
			if len(cfg.Projects) > 0 {
				projectCommits = collectProjectCommits(entry)
				commits = append(commits, formatProjectCommits(projectCommits)...)
			}
			worklog := readWorklog()
			promptText := buildPromptText(commits, worklog)
			description := getDescription(promptText)
			if description == "" {
				os.Remove(worklogPath())
				fmt.Println("Stopped tracking. No summary saved.")
				return nil
			}

			if len(cfg.Projects) > 0 {
				splits := calculateProjectSplits(entry, projectCommits)
				if len(splits) > 0 {
					if err := applyProjectSplits(entry, splits, description); err != nil {
						return err
					}
					os.Remove(worklogPath())
					printProjectSplitSummary(splits)
					return nil
				}
			}

			if err := updateEntryDescription(entry, description); err != nil {
				return err
			}

			os.Remove(worklogPath())

			fmt.Println("Stopped tracking. Summary saved.")
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

func main() {
	if err := loadConfig(); err != nil {
		fmt.Println("Error loading config:", err)
		os.Exit(1)
	}

	root := &cobra.Command{Use: "toggl", Version: version}
	root.AddCommand(startCmd(), stopCmd(), logCmd(), whoamiCmd(), projectsCmd(), repairSummariesCmd())
	if err := root.Execute(); err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
}
