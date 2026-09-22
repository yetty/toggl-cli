## ADDED Requirements

### Requirement: Additionally source activity from Forgejo
When Forgejo is configured, the CLI SHALL additionally collect the user's Forgejo commits, pull requests, and issues for the session window alongside local git commits.

#### Scenario: Forgejo repositories are resolved from local remotes
- **WHEN** a path in `repositories` or `projects.<name>.repositories` has an `origin` remote on the configured Forgejo host
- **THEN** the CLI includes that `owner/repo` when collecting Forgejo activity

#### Scenario: Unmapped repository is excluded from splitting
- **WHEN** a resolved Forgejo repository has no `projects.<name>` mapping
- **THEN** its activity is excluded from project time splitting and contributes to summary text only
