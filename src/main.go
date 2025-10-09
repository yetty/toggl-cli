package main

import (
	"bytes"
	"encoding/json"
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

var (
	cfg     Config
	cfgPath string
)

func loadConfig() error {
	usr, _ := user.Current()
	cfgPath = filepath.Join(usr.HomeDir, ".toggl.yaml")
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(data, &cfg)
}

func togglRequest(method, path string, body io.Reader) ([]byte, error) {
	req, _ := http.NewRequest(method, "https://api.track.toggl.com/api/v9/"+path, body)
	req.SetBasicAuth(cfg.Toggl.APIKey, "api_token")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

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
			body := fmt.Sprintf(`{"workspace_id":%d,"project_id":%d,"created_with":"toggl-cli","start":"%s","duration":-1}`,
				cfg.Toggl.WorkspaceID, cfg.Toggl.ProjectID, time.Now().UTC().Format(time.RFC3339))

			_, err := togglRequest("POST", fmt.Sprintf("workspaces/%d/time_entries", cfg.Toggl.WorkspaceID), bytes.NewBufferString(body))
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
			// Stop current timer
			data, err := togglRequest("GET", "me/time_entries/current", nil)
			if err != nil {
				return err
			}
			var entry TogglTimeEntry
			if err := json.Unmarshal(data, &entry); err != nil {
				return err
			}
			if entry.ID == 0 {
				return fmt.Errorf("no running timer")
			}
			entry.Stop = time.Now()

			body, _ := json.Marshal(entry)
			_, err = togglRequest("PUT", fmt.Sprintf("workspaces/%d/time_entries/%d", entry.Workspace, entry.ID), bytes.NewBuffer(body))
			if err != nil {
				return err
			}

			// Collect commits with repository name
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
					return err
				}
				if len(out) > 0 {
					repoName := filepath.Base(repo) // get last part of path as repo name
					// Prefix commits with repository name
					allCommits = append(allCommits, fmt.Sprintf("Repository: %s\n%s", repoName, string(out)))
				}
			}

			// Read work log messages
			tempDir := os.TempDir()
			logFile := filepath.Join(tempDir, "toggl-worklog.txt")
			var workLogText string

			if logData, err := os.ReadFile(logFile); err == nil {
				workLogText = string(logData)
				// Clear the log file after reading
				os.Remove(logFile)
			}

			// Join all commits into a single string for AI prompt
			commitsText := strings.Join(allCommits, "\n\n")

			// Combine commits and work log for AI prompt
			var promptText string
			if workLogText != "" {
				promptText = fmt.Sprintf("Git commits:\n%s\n\nWork log:\n%s", commitsText, workLogText)
			} else {
				promptText = commitsText
			}

			// Ask OpenAI
			prompt := fmt.Sprintf("Summarize these git commits and work log:\n\n%s", promptText)
			summary, err := openAISummarize(prompt)
			if err != nil {
				return err
			}

			// Update entry description
			entry.Description = summary
			body, _ = json.Marshal(entry)
			_, err = togglRequest("PUT", fmt.Sprintf("workspaces/%d/time_entries/%d", entry.Workspace, entry.ID), bytes.NewBuffer(body))
			if err != nil {
				return err
			}

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

			// Create log entry with timestamp
			logEntry := fmt.Sprintf("[%s] %s\n", timestamp, message)

			// Get temp file path for logs
			tempDir := os.TempDir()
			logFile := filepath.Join(tempDir, "toggl-worklog.txt")

			// Append to log file
			file, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
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

func openAISummarize(text string) (string, error) {
	reqBody := map[string]interface{}{
		"model": cfg.OpenAI.Model,
		"messages": []map[string]string{
			{
				"role": "system",
				"content": `You are an expert assistant that summarizes Git commits into concise, human-readable descriptions for Toggl time entries. 
Always clearly state what work was done, in which repository/project, in 1-3 short sentences per repository. 
Focus on actions taken, features implemented, bugs fixed, and avoid listing commit hashes.`,
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
	req, _ := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewBuffer(buf))
	req.Header.Set("Authorization", "Bearer "+cfg.OpenAI.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
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

func main() {
	if err := loadConfig(); err != nil {
		fmt.Println("Error loading config:", err)
		os.Exit(1)
	}

	root := &cobra.Command{Use: "toggl"}
	root.AddCommand(startCmd(), stopCmd(), logCmd(), whoamiCmd(), projectsCmd())
	if err := root.Execute(); err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
}
