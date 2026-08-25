# tools/plans

Planning automation for the GitHub-tracked implementation plans.

## Maintained

- `sync_stage1_plans_to_github.rb` — reworked 2026-08-25 to be series-agnostic. Scans `docs/superpowers/plans/2026-*.md`, binds each file to the issue named in its `**Spec:**` line (`Issue #N`), and replaces the issue body's `## Implementation plan` section with `Source: <path>` plus the plan content. Index/template files are skipped by name. Idempotent: unchanged bodies are not PATCHed.
  - `ruby tools/plans/sync_stage1_plans_to_github.rb --dry-run` prints what would change.
  - Requires an authenticated `gh` CLI. All calls carry `--repo 1123786563/myqypt`.

## Archived one-shot scripts (do not rerun)

- `generate_stage1_plans.rb` — generated the T-series plans on 2026-08-24. Hardcodes the 2026-08-24 date prefix and selects issues by the #2..#101 number range; rerunning would overwrite hand-maintained plans.
- `create_t01_subissues.rb`, `create_t86_subissues.rb` — one-shot sub-issue creators with hardcoded blocker numbers and a `--limit 200` idempotency snapshot. Kept for provenance only.
