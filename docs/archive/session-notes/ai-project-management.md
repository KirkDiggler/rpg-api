# AI Project Management Capabilities

Date: 2025-01-13

## GitHub Integration

As Claude Code, I can help manage your GitHub project through the `gh` CLI:

### What I Can Do

**Issues**:
- View issues: `gh issue list`, `gh issue view <number>`
- Create issues: `gh issue create`
- Update issues: `gh issue edit <number>`
- Close issues: `gh issue close <number>`
- Add comments: `gh issue comment <number>`

**Pull Requests**:
- Create PRs: `gh pr create`
- View PRs: `gh pr list`, `gh pr view <number>`
- Check PR status: `gh pr checks`
- View PR comments: `gh api repos/owner/repo/pulls/<number>/comments`
- Cannot merge PRs (requires user action)

**Milestones**:
- View milestones: `gh api repos/owner/repo/milestones`
- Check milestone progress
- Cannot create/edit milestones directly

**Projects**:
- Limited project board access via API
- Can view project status

### Best Practices

1. **Always check PR comments**: Use `gh api` to see inline code review comments
2. **Link PRs to Issues**: Use "Closes #X" in PR descriptions
3. **Update issue status**: Comment on issues when work begins/completes
4. **Check CI status**: Use `gh pr checks` before asking for review

### Example Workflow

```bash
# Start work on issue
gh issue view 4
gh issue comment 4 --body "Starting implementation"

# Create PR when ready
gh pr create --title "feat: Implement feature" \
  --body "Closes #4\n\nDescription..." \
  --milestone "Milestone 1"

# Check PR feedback
gh pr view <number> --comments
gh api repos/owner/repo/pulls/<number>/comments

# After changes
gh pr comment <number> --body "Addressed review feedback"
```

## Project Management Philosophy

- **Transparent Progress**: Use issue comments to track work
- **Link Everything**: PRs should reference issues
- **Check Feedback**: Always look for inline PR comments
- **Document Decisions**: Major choices go in issues/PRs
- **Milestone Tracking**: Keep issues organized by milestone
