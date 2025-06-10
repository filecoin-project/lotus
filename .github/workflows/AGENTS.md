# PR Validation Workflows

## Important Considerations

### Synchronization Requirements
- **CRITICAL**: Changes to validation logic MUST be reflected in CONTRIBUTING.md](../../CONTRIBUTING.md).  CONTRIBUTING.md and enforcement workflows should be kept in sync.
- Update both enforcement (workflows) AND documentation (CONTRIBUTING.md) together
- Consider impact on existing open PRs when changing validation rules

### GitHub API Limitations  
- `pull_request_target` required for write permissions to post reviews
- 5-second delay before dismissing reviews to avoid race conditions
- Bot review detection relies on `user.type === 'Bot'`

### Regex Pattern Gotchas
- Case sensitivity matters: `Revert` vs `revert`
- Escaping required for special characters in descriptions
- JavaScript regex syntax differs from other tools

### Workflow Permissions
- `pr-title-check.yml` needs `pull-requests: write` for reviews
- `changelog.yml` uses default permissions (read-only)
