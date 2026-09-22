## ADDED Requirements

### Requirement: Optional Forgejo configuration
The CLI SHALL integrate with Forgejo only when `forgejo.url` and `forgejo.api_key` are both configured.

#### Scenario: Integration disabled
- **WHEN** `forgejo.url` or `forgejo.api_key` is empty
- **THEN** the CLI makes no Forgejo requests and summary behavior is unchanged

#### Scenario: Environment override
- **WHEN** `FORGEJO_API_KEY` is set
- **THEN** the CLI uses it in place of `forgejo.api_key`

### Requirement: Resolve Forgejo repositories
The CLI SHALL resolve the Forgejo repositories to scan from `forgejo.repositories` and from the Forgejo remotes of paths in `repositories` and `projects.<name>.repositories`.

#### Scenario: Local repository points at the Forgejo server
- **WHEN** `git -C <path> remote get-url origin` resolves to the configured Forgejo host
- **THEN** the CLI includes that `owner/repo` and, when discovered under `projects.<name>`, applies that project's mapping

#### Scenario: Repository has no project mapping
- **WHEN** a resolved repository has no `projects.<name>` mapping
- **THEN** its activity contributes to summary text only and is excluded from time splitting

### Requirement: Collect Forgejo commits
The CLI SHALL collect commits authored by the configured `git.user` identities whose committer timestamp falls within the session window.

#### Scenario: Matching commit exists
- **WHEN** a resolved repository contains a commit in the window whose author name, email, or login matches a `git.user` token
- **THEN** the CLI includes its first message line as a Forgejo commit

### Requirement: Collect Forgejo pull requests and issues
The CLI SHALL collect pull requests and issues the user created, is assigned, was requested to review, or reviewed, updated within the session window.

#### Scenario: Pull request activity
- **WHEN** the search returns a pull request in a resolved repository updated in the window
- **THEN** the CLI includes it as a Forgejo pull request

#### Scenario: Issue activity
- **WHEN** the search returns an issue in a resolved repository updated in the window
- **THEN** the CLI includes it as a Forgejo issue

### Requirement: Tolerate Forgejo failures
The CLI SHALL continue summary generation when Forgejo requests fail.

#### Scenario: API error
- **WHEN** a Forgejo request returns a non-success status or transport error
- **THEN** the CLI writes a warning to stderr and continues with locally collected activity
