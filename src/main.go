package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
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

const worklogFile = "toggl-worklog.txt"

const openAISystemPrompt = `You are an expert assistant that summarizes Git commits into concise, human-readable descriptions for Toggl time entries.
Always clearly state what work was done, in which repository/project, in 1-3 short sentences per repository.
Focus on actions taken, features implemented, bugs fixed, and avoid listing commit hashes.`

// --- Global vars ---

var (
	cfg           Config
	cfgPath       string
	togglBaseURL  = "https://api.track.toggl.com/api/v9/"
	openAIBaseURL = "https://api.openai.com/v1/chat/completions"
)

// --- Config loading ---

func loadConfig() error {
	usr, _ := user.Current()
	cfgPath = filepath.Join(usr.HomeDir, ".toggl.yaml")
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
	entry.Stop = time.Now()
	body, _ := json.Marshal(entry)
	_, err := togglRequest("PUT", fmt.Sprintf("workspaces/%d/time_entries/%d", entry.Workspace, entry.ID), bytes.NewBuffer(body))
	return err
}

func collectCommits(entry TogglTimeEntry) []string {
	var allCommits []string
	for _, repo := range cfg.Repositories {
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

func readWorklog() string {
	if data, err := os.ReadFile(worklogPath()); err == nil {
		return string(data)
	}
	return ""
}

func buildPromptText(commits []string, worklog string) string {
	commitsText := strings.Join(commits, "\n\n")
	if worklog != "" {
		return fmt.Sprintf("Git commits:\n%s\n\nWork log:\n%s", commitsText, worklog)
	}
	return commitsText
}

func getDescription(promptText string) string {
	prompt := fmt.Sprintf("Summarize these git commits and work log:\n\n%s", promptText)
	summary, err := openAISummarize(prompt)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		fmt.Printf("\nCollected data:\n%s\n\n", promptText)
		fmt.Print("AI summarization failed. Enter description manually (or press Enter to skip): ")
		scanner := bufio.NewScanner(os.Stdin)
		scanner.Scan()
		summary = strings.TrimSpace(scanner.Text())
		if summary == "" {
			fmt.Println("Skipped description. Check your OpenAI API key in ~/.toggl.yaml")
		}
	}
	return summary
}

func updateEntryDescription(entry TogglTimeEntry, description string) error {
	entry.Description = description
	body, _ := json.Marshal(entry)
	_, err := togglRequest("PUT", fmt.Sprintf("workspaces/%d/time_entries/%d", entry.Workspace, entry.ID), bytes.NewBuffer(body))
	return err
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
			worklog := readWorklog()
			promptText := buildPromptText(commits, worklog)
			description := getDescription(promptText)

			if err := updateEntryDescription(entry, description); err != nil {
				return err
			}

			os.Remove(worklogPath())

			fmt.Println("Stopped tracking. Summary saved.")
			return nil
		},
	}
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
	root.AddCommand(startCmd(), stopCmd(), logCmd(), whoamiCmd(), projectsCmd())
	if err := root.Execute(); err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
}
