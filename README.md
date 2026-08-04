# Toggl CLI

A simple command-line tool to track time in [Toggl Track](https://track.toggl.com/) and summarize Git commits using OpenAI.

This tool allows you to:

* Start and stop Toggl timers for a specific project.
* Check calendar workload budget status on demand, including next month's planned work against the expected monthly hours.
* Log work activities during your session.
* Automatically collect Git commits between timer start/stop.
* Generate a summary of commits and work logs via OpenAI and save it to the time entry.
* View current Toggl user info.
* List all projects in your workspace.

---

## Features

* **Start/Stop timers** with `toggl start` and `toggl stop`.
* **Calendar workload status**: `toggl track` reports current calendar budget progress plus next-month planned work against the expected monthly hours.
* **Work logging**: `toggl log` saves work activities for session summarization.
* **Commit summarization**: `stop` generates AI summary of Git commits and work logs.
* **User info**: `toggl whoami` prints current Toggl user ID and email.
* **Project listing**: `toggl projects` lists all projects in the workspace with IDs.

---

## Installation

### 1. Build locally

```bash
git clone https://github.com/yourusername/toggl.git
cd toggl
make build
```

Run the binary:

```bash
./toggl start
./toggl stop
```

### 2. Install to your user-local bin

```bash
make install
```

This installs the binary to `~/.local/bin/toggl` without `sudo`. Make sure `~/.local/bin` is on your `PATH`.

Then you can run:

```bash
toggl start
toggl track
toggl log "Working on feature X"
toggl stop
toggl projects
toggl whoami
```

---

## Configuration

All configuration is stored in `~/.toggl.yaml`.

Example:

```yaml
toggl:
  api_key: "your_toggl_api_token"
  workspace_id: 8320206
  project_id: 204198137

openai:
  api_key: "your_openai_api_key"
  model: "gpt-4o-mini"

git:
  user: "John Doe"   # filter commits by this user

repositories:
  - "/path/to/repo1"
  - "/path/to/repo2"

projects:
  cortex:
    project_id: 204198137
    repositories:
      - "~/Projects/lkq/cortex"
  voicesense:
    project_id: 204198138
    repositories:
      - "~/Projects/lkq/voicesense"

calendar:
  id: "your_calendar_id@example.com"
  api_key: "your_google_calendar_api_key"
  event_names:
    - "Deep Work"
    - "Client Work"
  monthly_hour_budget: 120
  project_ids:
    - 204198137
```

* **Toggl**: provide your API token, workspace ID, and default project ID.
* **OpenAI**: provide your API key and model name.
* **Git**: only commits authored by this user will be summarized.
* **Projects**: optional map of project names to Toggl project IDs and Git repositories. When project commits are detected during `toggl stop`, the CLI can split one stopped timer into multiple project-specific Toggl entries with contiguous time ranges.
* **Repositories**: legacy list of Git repositories to scan for commits. This still works for summary-only behavior when `projects:` is not configured or no project commits are detected.
* **Calendar**: optional Google Calendar workload tracking. Use a calendar that is public/shareable enough for Google Calendar API-key reads, then configure its calendar ID, exact event names to count as planned work, a fixed monthly hour budget, and the Toggl project IDs that count as actual worked time. This powers on-demand workload status checks with `toggl track`, including a next-month planned-hours line that indicates whether planned work meets the monthly budget, plus calendar hints after `start`/`stop`. When `project_ids` is omitted, the CLI falls back to configured Toggl project IDs.

---

## Commands

| Command          | Description                                                   |
| ---------------- | ------------------------------------------------------------- |
| `toggl start`    | Start a new Toggl time entry for the configured project.      |
| `toggl track`    | Show current calendar workload budget status.                 |
| `toggl log`      | Log a work activity message for the current session.          |
| `toggl stop`     | Stop the current timer, summarize commits and logs, save to Toggl. |
| `toggl whoami`   | Show current Toggl user info.                                 |
| `toggl projects` | List all projects in the workspace with IDs.                  |

---

## Example Workflow

```bash
# Start a timer
toggl start

# Log work activities as you go
toggl log "Fixed authentication bug"
toggl log "Added user validation"
toggl log "Updated documentation"

# Do some work in your Git repositories...

# Stop the timer, summarize commits and logs, save summary to Toggl
toggl stop

# View your Toggl user info
toggl whoami

# List all projects in workspace
toggl projects
```

---

## Notes

* The CLI uses Toggl REST API v9.
* Make sure your API token has access to the configured project.
* The stop command requires Git repositories to exist and be accessible locally.
* Work logs are stored temporarily and automatically cleaned up after each stop command.
