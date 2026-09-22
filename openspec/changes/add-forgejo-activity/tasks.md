## 1. Configuration

- [x] 1.1 Add `forgejo` config section with `url`, `api_key`, `repositories`
- [x] 1.2 Support `FORGEJO_API_KEY` override

## 2. Collection

- [x] 2.1 Parse Forgejo git remotes into `owner/repo`
- [x] 2.2 Resolve repositories from config and local remotes with project mapping
- [x] 2.3 Fetch and filter commits by author and window
- [x] 2.4 Fetch and filter pull requests and issues
- [x] 2.5 Assemble split and prompt activity views

## 3. Integration

- [x] 3.1 Include Forgejo sections in `fill-empty-descriptions` and `repair-summaries` prompts
- [x] 3.2 Merge mapped Forgejo activity into `stop` time splitting
- [x] 3.3 Include Forgejo sections in the `stop` non-split prompt

## 4. Verification

- [x] 4.1 Unit tests for parsing, resolution, collection, and integration
- [x] 4.2 `make fmt`, `make test`, `make build`
- [x] 4.3 Update README
