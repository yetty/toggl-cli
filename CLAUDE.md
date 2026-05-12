# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Development Commands

- `make build` — compile binary to `./bin/toggl`
- `make install` — copy binary to `/usr/local/bin/` (requires sudo)
- `make run ARGS="start"` — run the local binary with arguments
- `make fmt` — format code with `go fmt`
- `make test` — run tests with `go test ./...`
- `make cross` — cross-compile for Linux, macOS (Intel/ARM), Windows
- `make clean` — remove build artifacts

## Git Workflow

- Make small, focused commits regularly as work progresses.
- Keep each commit scoped to one coherent change and verify relevant tests before committing.
- Do not bundle unrelated cleanup with feature or bug-fix commits.

## Architecture

Single-file Go CLI (`src/main.go`, ~340 LOC) built with Cobra. All commands, types, and helpers live in one `main` package.

**Commands:** `start`, `stop`, `log`, `whoami`, `projects` — each defined as a function returning `*cobra.Command`, wired together in `main()`.

**Core workflow:** `start` creates a Toggl time entry → user works and optionally runs `log` to append notes to `/tmp/toggl-worklog.txt` → `stop` halts the timer, gathers git commits from configured repositories within the session timeframe, sends commits + work logs to OpenAI for summarization, and writes the summary back as the time entry description.

**External integrations:**
- Toggl Track API v9 (Basic Auth via `togglRequest()`)
- OpenAI Chat Completions API (`openAISummarize()`)
- Local git repos (shell out to `git log`)

**Configuration:** `~/.toggl.yaml` with Toggl credentials, OpenAI key/model, git author name, and repository paths.
