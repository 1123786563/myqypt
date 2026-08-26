#!/usr/bin/env bash
# Read-only upstream-update review gate for Issue #105 (F05), design ruling 8.
#
# Prints a diff summary between two commits of a declared upstream (via the
# read-only GitHub compare API) and refuses to bless the update until a review
# record file carries all six review boxes checked:
#   authentication / tenant / network / storage / copied-code / security-advisory
#
# This script NEVER fetches, merges, or syncs anything: `gh api` compare is its
# only network call, and it only reads. Updating the pinned provenance commits
# remains a human-reviewed decision.
#
# Usage:
#   bash scripts/review-upstream-update.sh <source> <old-commit> <new-commit> [--record <file>]
#
# Exit codes: 0 review complete (all boxes checked); 1 review incomplete or
# arguments invalid.
set -euo pipefail

MANIFEST="docs/upstream/provenance.yaml"
REVIEW_CATEGORIES="authentication tenant network storage copied-code security-advisory"

usage() {
  cat >&2 <<EOF
usage: bash scripts/review-upstream-update.sh <source> <old-commit> <new-commit> [--record <file>]

  <source>     upstream name declared in ${MANIFEST}
  <old-commit> current pinned commit (40 lowercase hex)
  <new-commit> proposed upstream commit (40 lowercase hex)
  --record     review record file; must carry all six boxes checked:
               ${REVIEW_CATEGORIES}

Read-only: fetches nothing, merges nothing, syncs nothing.
EOF
}

fail_incomplete() {
  echo "REVIEW-INCOMPLETE: $*" >&2
  exit 1
}

SOURCE=""
OLD_COMMIT=""
NEW_COMMIT=""
RECORD=""
while [ $# -gt 0 ]; do
  case "$1" in
    --record)
      [ $# -ge 2 ] || { usage; exit 1; }
      RECORD="$2"
      shift 2
      ;;
    -h|--help) usage; exit 0 ;;
    *)
      if [ -z "$SOURCE" ]; then SOURCE="$1"
      elif [ -z "$OLD_COMMIT" ]; then OLD_COMMIT="$1"
      elif [ -z "$NEW_COMMIT" ]; then NEW_COMMIT="$1"
      else usage; exit 1
      fi
      shift
      ;;
  esac
done
if [ -z "$SOURCE" ] || [ -z "$OLD_COMMIT" ] || [ -z "$NEW_COMMIT" ]; then
  usage
  exit 1
fi

hex40() {
  echo "$1" | grep -Eq '^[0-9a-f]{40}$'
}

# Manifest validation (PROV-* semantics, design ruling 8): the source must be
# declared, and both commits must be 40 lowercase hex characters.
[ -f "$MANIFEST" ] || fail_incomplete "provenance manifest not found: $MANIFEST"
grep -Eq "^  - name: ${SOURCE}$" "$MANIFEST" || fail_incomplete "source '${SOURCE}' is not declared in ${MANIFEST}"
hex40 "$OLD_COMMIT" || fail_incomplete "old commit is not 40 lowercase hex characters (PROV-COMMIT): ${OLD_COMMIT}"
hex40 "$NEW_COMMIT" || fail_incomplete "new commit is not 40 lowercase hex characters (PROV-COMMIT): ${NEW_COMMIT}"
[ "$OLD_COMMIT" != "$NEW_COMMIT" ] || fail_incomplete "old and new commits are identical; there is nothing to review"

REPOSITORY="$(awk -v src="$SOURCE" '
  /^  - name: / { in_block = ($3 == src) }
  in_block && /^    repository: / { sub(/^    repository: /, ""); print; exit }
' "$MANIFEST")"
[ -n "$REPOSITORY" ] || fail_incomplete "no repository URL found for source '${SOURCE}' in ${MANIFEST}"
SLUG="$(echo "$REPOSITORY" | sed -E 's#^https://github\.com/##')"
echo "$SLUG" | grep -Eq '^[^/]+/[^/]+$' || fail_incomplete "cannot derive owner/repo from repository URL: ${REPOSITORY}"

# Read-only diff summary via the GitHub compare API.
echo "upstream compare (read-only): ${SLUG} ${OLD_COMMIT}...${NEW_COMMIT}"
gh api "repos/${SLUG}/compare/${OLD_COMMIT}...${NEW_COMMIT}" \
  --jq '"compare status: " + .status + ", ahead_by: " + (.ahead_by | tostring) + ", behind_by: " + (.behind_by | tostring) + ", commits: " + (.total_commits | tostring) + ", files changed: " + (.files | length | tostring)' \
  || fail_incomplete "compare API call failed for ${SLUG}"

# Six-box review record gate: every category checked, none unchecked.
if [ -z "$RECORD" ]; then
  fail_incomplete "no --record file provided; the six review boxes must be checked"
fi
[ -f "$RECORD" ] || fail_incomplete "record file not found: ${RECORD}"

INCOMPLETE=0
for category in $REVIEW_CATEGORIES; do
  if ! grep -Eq "^[[:space:]]*[-*+][[:space:]]*\[[xX]\][[:space:]]*${category}[[:space:]]*$" "$RECORD"; then
    echo "REVIEW-INCOMPLETE: record is missing a checked box for '${category}'" >&2
    INCOMPLETE=1
  fi
done
if grep -Eq "^[[:space:]]*[-*+][[:space:]]*\[[[:space:]]*\][[:space:]]*(authentication|tenant|network|storage|copied-code|security-advisory)[[:space:]]*$" "$RECORD"; then
  echo "REVIEW-INCOMPLETE: record still has an unchecked review box" >&2
  INCOMPLETE=1
fi
[ "$INCOMPLETE" -eq 0 ] || exit 1

echo "review record complete: all six boxes checked (${RECORD})"
echo "REVIEW-OK: ${SOURCE} ${OLD_COMMIT} -> ${NEW_COMMIT} (read-only review; sync decisions stay human)"
