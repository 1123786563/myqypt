# Issue tracker: GitHub

Issues and specifications for this repository live in GitHub repository `1123786563/myqypt`. Use the `gh` CLI for all issue operations.

Because the Git remote uses a proxy URL, every `gh issue`, `gh pr`, and related `gh api` command must explicitly include `--repo 1123786563/myqypt` or target the repository in the API path.

## Conventions

- Create an issue with `gh issue create --repo 1123786563/myqypt`.
- Read an issue with `gh issue view <number> --repo 1123786563/myqypt`.
- Update an issue with `gh issue edit <number> --repo 1123786563/myqypt`.
- Close an issue with `gh issue close <number> --repo 1123786563/myqypt` only after its acceptance criteria are satisfied.
- Reopen an issue with `gh issue reopen <number> --repo 1123786563/myqypt` when the tracked work is not actually complete.
- Search and list issues with `gh issue list --repo 1123786563/myqypt`.
- Use issue comments for concise progress, decisions, blockers, and verification evidence.
- Preserve issue descriptions as the durable specification; edit them when the agreed scope or acceptance criteria change.

## Pull requests as a triage surface

Pull requests are not a request or triage surface. Track requests, bugs, and planned work as issues. A pull request may reference and close an issue, but it does not replace the issue.

## Publishing from a skill

When a skill says to publish, create or update the corresponding GitHub issue in `1123786563/myqypt`. Do not leave the result only in chat or in an untracked scratch file. Include the relevant labels, relationships, acceptance criteria, and verification evidence.

## Wayfinding operations

- Map the work: list open issues and inspect relevant parents, children, labels, milestones, and dependencies.
- Represent parent-child relationships with GitHub sub-issues when available. Otherwise, maintain explicit task lists and reciprocal links in issue bodies.
- Represent blocking dependencies with GitHub's native dependency support when available. If the CLI has no direct command, use `gh api` against `repos/1123786563/myqypt/...`; otherwise, add explicit `Blocked by` and `Blocks` links to both issues.
- Find the executable frontier: issues that are open, sufficiently specified, not blocked, and labeled `ready-for-agent`.
- Claim work by leaving a comment before implementation and avoiding duplicate claims.
- Resolve work by attaching verification evidence, updating relationships, and closing the issue only when its acceptance criteria are met.
