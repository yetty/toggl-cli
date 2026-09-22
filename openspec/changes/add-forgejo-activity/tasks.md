## 1. Configuration

- [ ] 1.1 Add `forgejo` config section with `url`, `api_key`, `repositories`
- [ ] 1.2 Support `FORGEJO_API_KEY` override

## 2. Collection

- [ ] 2.1 Parse Forgejo git remotes into `owner/repo`
- [ ] 2.2 Resolve repositories from config and local remotes with project mapping
- [ ] 2.3 Fetch and filter commits by author and window
- [ ] 2.4 Fetch and filter pull requests and issues
- [ ] 2.5 Assemble split and prompt activity views

## 3. Integration

- [ ] 3.1 Include Forgejo sections in `fill-empty-descriptions` and `repair-summaries` prompts
- [ ] 3.2 Merge mapped Forgejo activity into `stop` time splitting
- [ ] 3.3 Include Forgejo sections in the `stop` non-split prompt

## 4. Verification

- [ ] 4.1 Unit tests for parsing, resolution, collection, and integration
- [ ] 4.2 `make fmt`, `make test`, `make build`
- [ ] 4.3 Update README
