## ADDED Requirements

### Requirement: Include Forgejo activity in summary prompts
The CLI SHALL include collected Forgejo activity as labelled sections in summary prompts, emitting a section only for each activity kind that has entries.

#### Scenario: Forgejo commit activity is collected
- **WHEN** Forgejo commit activity is collected for the session window
- **THEN** the prompt text contains a `Forgejo commits:` section

#### Scenario: Forgejo pull request activity is collected
- **WHEN** Forgejo pull request activity is collected for the session window
- **THEN** the prompt text contains a `Forgejo pull requests:` section

#### Scenario: Forgejo issue activity is collected
- **WHEN** Forgejo issue activity is collected for the session window
- **THEN** the prompt text contains a `Forgejo issues:` section

#### Scenario: No Forgejo activity is collected
- **WHEN** no Forgejo activity is collected
- **THEN** the prompt text omits Forgejo sections and local-only prompt behavior is unchanged
