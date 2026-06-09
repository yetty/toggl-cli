## ADDED Requirements

### Requirement: Build summary prompt text from commits and worklog
The CLI SHALL build prompt text from non-empty git commit sections and non-empty worklog content.

#### Scenario: Commits and worklog are both present
- **WHEN** collected commits and worklog content are both non-empty
- **THEN** the prompt text contains a `Git commits:` section and a `Work log:` section separated by a blank line

#### Scenario: Only worklog is present
- **WHEN** no commits are collected and the worklog is non-empty
- **THEN** the prompt text omits the `Git commits:` section and includes a `Work log:` section

#### Scenario: No commits or worklog are present
- **WHEN** collected commits are empty and worklog content is blank
- **THEN** the prompt text is empty

### Requirement: Request one-line OpenAI summaries
The CLI SHALL use OpenAI chat completions to derive a Toggl description when prompt text is non-empty.

#### Scenario: OpenAI summary succeeds
- **WHEN** prompt text is non-empty and the OpenAI chat completion request returns at least one choice
- **THEN** the CLI uses the first choice message content as the description

#### Scenario: OpenAI returns non-200 status
- **WHEN** the OpenAI chat completion request returns a status other than 200
- **THEN** the CLI treats summary generation as failed and offers manual entry

#### Scenario: OpenAI returns no choices
- **WHEN** the OpenAI response decodes successfully but contains no choices
- **THEN** the CLI treats summary generation as failed and offers manual entry

### Requirement: Use constrained summary instructions
The OpenAI request SHALL ask for exactly one brief Toggl description sentence of at most 140 characters without Markdown, bullets, headings, repository lists, or commit hashes.

#### Scenario: OpenAI request is sent
- **WHEN** the CLI sends an OpenAI chat completion request
- **THEN** the request includes the configured model and a system message containing the one-sentence 140-character no-Markdown constraints

### Requirement: Fall back to manual descriptions
The CLI SHALL prompt the user for a manual description when no prompt text is available or OpenAI summarization fails.

#### Scenario: No prompt text exists
- **WHEN** no commits and no worklog entries are found
- **THEN** the CLI does not call OpenAI
- **THEN** the CLI prompts `No commits or work log entries found. Enter description manually (or press Enter to skip): `

#### Scenario: AI summarization fails
- **WHEN** prompt text exists but OpenAI summarization returns an error
- **THEN** the CLI prints the error and collected data
- **THEN** the CLI prompts `AI summarization failed. Enter description manually (or press Enter to skip): `

#### Scenario: Manual description is empty
- **WHEN** the user presses Enter without typing a manual description
- **THEN** the CLI prints `Skipped description.` and treats the description as empty
