## ADDED Requirements

### Requirement: Collect legacy repository commit subjects
The CLI SHALL collect git commit subjects from configured legacy repositories for a stopped session window.

#### Scenario: Matching legacy commits exist
- **WHEN** a repository in `repositories` contains commits authored by `git.user` between the stopped entry start and stop timestamps
- **THEN** the CLI invokes `git -C <repo> log --all --since=<start> --until=<stop> --author=<git.user> --pretty=format:%s`
- **THEN** the collected commit text includes the repository name and matching commit subjects

#### Scenario: Legacy repository has no matching output
- **WHEN** git returns no commit subjects for a configured repository
- **THEN** the CLI omits that repository from the collected commit sections

### Requirement: Collect project commit metadata
The CLI SHALL collect project-aware commit metadata from configured project repositories when project configuration exists.

#### Scenario: Matching project commits exist
- **WHEN** a repository under `projects.<name>.repositories` contains commits authored by `git.user` in the stopped entry window
- **THEN** the CLI invokes `git -C <repo> log --all --since=<start> --until=<stop> --author=<git.user> --pretty=format:%cI%x09%s`
- **THEN** each valid line becomes a project commit with project name, project ID, repository name, repository path, commit subject, and commit timestamp

#### Scenario: Multiple project commits are collected
- **WHEN** multiple project commits are collected from configured projects
- **THEN** the CLI sorts them by commit time, then project name, project ID, repository name, and subject

### Requirement: Warn and skip inaccessible repositories
The CLI SHALL continue processing when a configured repository path cannot be read as a git repository.

#### Scenario: Repository path does not exist
- **WHEN** git collection reaches a configured repository path that does not exist
- **THEN** the CLI writes a warning to stderr identifying the repository and reason `path not found`
- **THEN** the CLI continues with remaining repositories

#### Scenario: Repository is not a git repository
- **WHEN** git reports that a configured repository is not a git repository
- **THEN** the CLI writes a warning to stderr identifying the repository and reason `not a git repository`
- **THEN** the CLI continues with remaining repositories

#### Scenario: Git command fails for another reason
- **WHEN** git collection fails for a reason other than missing path or non-git repository
- **THEN** the CLI writes a warning to stderr with the git error text
- **THEN** the CLI continues with remaining repositories

### Requirement: Skip malformed project git log lines
The CLI SHALL skip malformed project git log output lines without failing the stop workflow.

#### Scenario: Project git log line lacks a timestamp and subject separator
- **WHEN** a project git log output line cannot be split into timestamp and subject fields
- **THEN** the CLI writes a warning to stderr and does not create a project commit for that line

#### Scenario: Project git log timestamp cannot be parsed
- **WHEN** a project git log output line has an invalid RFC3339 timestamp
- **THEN** the CLI writes a warning to stderr and does not create a project commit for that line
