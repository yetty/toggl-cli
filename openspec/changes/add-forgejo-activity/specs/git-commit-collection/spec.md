## ADDED Requirements

### Requirement: Collect Forgejo activity
The CLI SHALL collect the user's Forgejo commits, pull requests, and issues for the stopped session window when Forgejo is configured, in addition to local git collection.

#### Scenario: Forgejo repositories are resolved from local remotes
- **WHEN** a path in `repositories` or `projects.<name>.repositories` has an `origin` remote on the configured Forgejo host
- **THEN** the CLI includes that `owner/repo` when collecting Forgejo activity

#### Scenario: Forgejo commits match author and window
- **WHEN** a resolved repository contains a commit in the session window authored by a `git.user` identity
- **THEN** the CLI includes the commit's first message line in the collected activity

#### Scenario: Unmapped repository is excluded from splitting
- **WHEN** a resolved Forgejo repository has no `projects.<name>` mapping
- **THEN** its activity contributes to summary text only and never produces a project split

#### Scenario: Forgejo request fails
- **WHEN** a Forgejo request returns a non-success status or a transport error
- **THEN** the CLI writes a warning to stderr and continues with locally collected activity
