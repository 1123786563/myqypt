#!/usr/bin/env bash
# Unified foundation verification entry for Issue #105 (F05), design rulings
# 7 and 9.
#
# Runs the verification phases serially and reports each phase separately:
#   GENERATE     make generate-check (generated code is deterministic)
#   POLICY       make policy-check (architecture dependency policy)
#   UNIT         go test ./... -count=1 -skip ^TestContract
#   CONTRACT     go test ./internal/transport/http -run ^TestContract -count=1
#   INTEGRATION  go test ./internal/adapter/postgres -count=1 — TEST_DATABASE_URL
#                must be set; an unset variable FAILS the phase (vacuum
#                acceptance is refused, never skipped green)
#   FRONTEND     pnpm --dir web typecheck+test+build+verify:static — requires
#                web/node_modules (run `pnpm --dir web install` first)
#   META         revision and tool versions
#
# Evidence: artifacts/foundation-verification.json records only phase name,
# static command string, status, duration, the revision, and tool versions
# (design ruling 9). No environment values, DSNs, tokens, or user data ever
# enter the output.
#
# Usage:
#   bash scripts/verify-foundation.sh [--phases GENERATE,POLICY,...] [--json <path>]
#
# Exit 1 if any requested phase fails; the JSON is written either way.
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(dirname "$SCRIPT_DIR")"
cd "$REPO_ROOT"

DEFAULT_PHASES="GENERATE,POLICY,UNIT,CONTRACT,INTEGRATION,FRONTEND,META"
KNOWN_PHASES=" GENERATE POLICY UNIT CONTRACT INTEGRATION FRONTEND META "
PHASES="$DEFAULT_PHASES"
JSON_PATH="artifacts/foundation-verification.json"

REVISION=""
GO_VERSION=""
NODE_VERSION=""
PNPM_VERSION=""
MAKE_VERSION=""

usage() {
  cat >&2 <<EOF
usage: bash scripts/verify-foundation.sh [--phases <csv>] [--json <path>]

  --phases  comma-separated subset of: ${DEFAULT_PHASES}
  --json    evidence JSON path (default: ${JSON_PATH})
EOF
}

while [ $# -gt 0 ]; do
  case "$1" in
    --phases)
      [ $# -ge 2 ] || { usage; exit 2; }
      PHASES="$2"
      shift 2
      ;;
    --json)
      [ $# -ge 2 ] || { usage; exit 2; }
      JSON_PATH="$2"
      shift 2
      ;;
    -h|--help) usage; exit 0 ;;
    *) usage; exit 2 ;;
  esac
done

IFS=','
for _phase in $PHASES; do
  case "$KNOWN_PHASES" in
    *" $_phase "*) ;;
    *) echo "verify-foundation: unknown phase '$_phase'" >&2; usage; exit 2 ;;
  esac
done
unset IFS

ms_now() {
  perl -MTime::HiRes -e 'printf("%d\n", Time::HiRes::time()*1000)'
}

phase_names=()
phase_commands=()
phase_statuses=()
phase_durations=()

record_phase() {
  # $1 name  $2 static command string  $3 status  $4 duration_ms
  phase_names[${#phase_names[@]}]="$1"
  phase_commands[${#phase_commands[@]}]="$2"
  phase_statuses[${#phase_statuses[@]}]="$3"
  phase_durations[${#phase_durations[@]}]="$4"
  printf 'PHASE %-12s %-4s %7d ms  %s\n' "$1" "$3" "$4" "$2"
}

collect_meta() {
  [ -n "$REVISION" ] || REVISION="$(git rev-parse --short HEAD 2>/dev/null || echo unknown)"
  [ -n "$GO_VERSION" ] || GO_VERSION="$(go version 2>/dev/null | awk '{print $3}' || echo unknown)"
  [ -n "$NODE_VERSION" ] || NODE_VERSION="$(node --version 2>/dev/null || echo unknown)"
  [ -n "$PNPM_VERSION" ] || PNPM_VERSION="$(pnpm --version 2>/dev/null || echo unknown)"
  [ -n "$MAKE_VERSION" ] || MAKE_VERSION="$(make --version 2>/dev/null | head -1 || echo unknown)"
}

run_one_phase() {
  phase="$1"
  start="$(ms_now)"
  case "$phase" in
    GENERATE)
      make generate-check
      rc=$?
      ;;
    POLICY)
      make policy-check
      rc=$?
      ;;
    UNIT)
      go test ./... -count=1 -skip '^TestContract'
      rc=$?
      ;;
    CONTRACT)
      go test ./internal/transport/http -run '^TestContract' -count=1
      rc=$?
      ;;
    INTEGRATION)
      if [ -z "${TEST_DATABASE_URL:-}" ]; then
        echo "INTEGRATION: TEST_DATABASE_URL is not set; refusing vacuum acceptance — phase FAILS (no silent skip)" >&2
        rc=1
      else
        go test ./internal/adapter/postgres -count=1
        rc=$?
      fi
      ;;
    FRONTEND)
      if [ ! -d web/node_modules ]; then
        echo "FRONTEND: web/node_modules is missing; run 'pnpm --dir web install' first (not installed automatically)" >&2
        rc=1
      else
        rc=0
        if pnpm --dir web run typecheck; then
          if pnpm --dir web run test; then
            if pnpm --dir web run build; then
              if pnpm --dir web run verify:static; then
                :
              else
                rc=1
              fi
            else
              rc=1
            fi
          else
            rc=1
          fi
        else
          rc=1
        fi
      fi
      ;;
    META)
      collect_meta
      rc=$?
      ;;
  esac
  end="$(ms_now)"
  if [ "$rc" -eq 0 ]; then status="PASS"; else status="FAIL"; fi
  record_phase "$phase" "$(phase_command_string "$phase")" "$status" "$((end - start))"
}

# Static command labels recorded in the evidence JSON (design ruling 9): make
# target names and go/pnpm invocations, never environment-expanded values.
phase_command_string() {
  case "$1" in
    GENERATE) echo "make generate-check" ;;
    POLICY) echo "make policy-check" ;;
    UNIT) echo "go test ./... -count=1 -skip ^TestContract" ;;
    CONTRACT) echo "go test ./internal/transport/http -run ^TestContract -count=1" ;;
    INTEGRATION) echo "go test ./internal/adapter/postgres -count=1" ;;
    FRONTEND) echo "pnpm --dir web run typecheck && pnpm --dir web run test && pnpm --dir web run build && pnpm --dir web run verify:static" ;;
    META) echo "meta:revision-and-tool-versions" ;;
  esac
}

write_evidence() {
  collect_meta
  local generated_at n i
  generated_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  mkdir -p "$(dirname "$JSON_PATH")"
  {
    printf '{\n'
    printf '  "schema_version": 1,\n'
    printf '  "generated_at": "%s",\n' "$generated_at"
    printf '  "revision": "%s",\n' "$REVISION"
    printf '  "tools": {\n'
    printf '    "go": "%s",\n' "$GO_VERSION"
    printf '    "node": "%s",\n' "$NODE_VERSION"
    printf '    "pnpm": "%s",\n' "$PNPM_VERSION"
    printf '    "make": "%s"\n' "$MAKE_VERSION"
    printf '  },\n'
    printf '  "phases": [\n'
    n=${#phase_names[@]}
    i=0
    while [ "$i" -lt "$n" ]; do
      if [ "$((i + 1))" -lt "$n" ]; then
        printf '    {"name": "%s", "command": "%s", "status": "%s", "duration_ms": %s},\n' \
          "${phase_names[$i]}" "${phase_commands[$i]}" "${phase_statuses[$i]}" "${phase_durations[$i]}"
      else
        printf '    {"name": "%s", "command": "%s", "status": "%s", "duration_ms": %s}\n' \
          "${phase_names[$i]}" "${phase_commands[$i]}" "${phase_statuses[$i]}" "${phase_durations[$i]}"
      fi
      i=$((i + 1))
    done
    printf '  ]\n'
    printf '}\n'
  } > "$JSON_PATH"
  echo "evidence JSON written: ${JSON_PATH}"
}

FAILED=0
IFS=','
for phase in $PHASES; do
  run_one_phase "$phase"
done
unset IFS

for phase_status in "${phase_statuses[@]}"; do
  if [ "$phase_status" != "PASS" ]; then
    FAILED=1
  fi
done

write_evidence

if [ "$FAILED" -ne 0 ]; then
  echo "verify-foundation: FAILED" >&2
  exit 1
fi
echo "verify-foundation: PASS"
