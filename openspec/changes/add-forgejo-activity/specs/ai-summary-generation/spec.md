## ADDED Requirements

### Requirement: Include Forgejo activity in summary prompts
The CLI SHALL include Forgejo commit, pull request, and issue sections in summary prompts when Forgejo activity is collected.

#### Scenario: Forgejo sections are present
- **WHEN** Forgejo activity is collected for the session window
- **THEN** the prompt text contains `Forgejo commits:`, `Forgejo pull requests:`, and `Forgejo issues:` sections between the local commit section and the worklog section

#### Scenario: No Forgejo activity is collected
- **WHEN** no Forgejo activity is collected
- **THEN** the prompt text omits Forgejo sections and local-only prompt behavior is unchanged
