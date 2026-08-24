# Task 2 Report: Shared Evidence Harness and Development Stack

## Status

Completed with concerns. The shared `platformtest` harness and Compose stack changes are implemented and committed. Focused Go verification passed. Compose configuration resolved successfully. `docker compose up -d --wait` did not reach a health result during this task because Docker spent the entire runtime window pulling the large `postgres:17` and `quay.io/keycloak/keycloak:26.3` images; I interrupted the still-active pull after collecting blocker evidence.

## Files Changed

- `go.mod`
- `go.sum`
- `deploy/compose/compose.yaml`
- `tests/platformtest/scenario.go`
- `tests/platformtest/run.go`
- `tests/platformtest/run_test.go`
- `tests/platformtest/schema/scenario.schema.json`

## Driver / Report / Redaction Design

- Added a shared registry with `platformtest.Register(seam string, driver Driver)` backed by `sync.Map`, preserving the exact panic contract for missing or duplicate seams.
- Added strict scenario decoding with `yaml.Decoder.KnownFields(true)` so unknown top-level fields are rejected instead of silently accepted.
- Added `platformtest.Run(t *testing.T, scenarioPath string) Report` to:
  - load and validate scenario contracts,
  - select a registered seam driver,
  - enforce a scenario timeout (default `30s`, overrideable per scenario),
  - collect build/runtime dependency metadata,
  - persist JSON evidence under `artifacts/evidence/<scenario-id>/`.
- Added structured `Scenario`, `Assertion`, `Report`, and `AssertionResult` types plus a JSON schema file for later acceptance/conformance/production-gate scenarios.
- Redaction behavior:
  - redacts structured values whose keys match `secret|token|prompt|document|payment_payload`,
  - scrubs those sensitive values back out of free-text `Summary` and assertion `Details` if a driver echoes them,
  - avoids storing raw scenario secrets in persisted evidence metadata.

## Compose Stack Design

- Extended `deploy/compose/compose.yaml` with:
  - `postgres` on `postgres:17`,
  - `keycloak` on `quay.io/keycloak/keycloak:26.3`,
  - health-gated `depends_on` from Keycloak to PostgreSQL,
  - env-variable references with `${VAR:?message}` fail-fast guards so committed Compose content contains no embedded credentials.
- Kept `platform-api` compatible with Task 1 and left production claims out of Compose usage.

## TDD Evidence

### Red 1: Missing harness

Command:

```text
go test ./tests/platformtest -run TestRunRejectsUnknownSeamWithoutLeakingInput -count=1
```

Result:

```text
# github.com/1123786563/myqypt/tests/platformtest [github.com/1123786563/myqypt/tests/platformtest.test]
tests/platformtest/run_test.go:16:12: undefined: Run
FAIL	github.com/1123786563/myqypt/tests/platformtest [build failed]
FAIL
```

### Red 2: Free-text leak from driver output

Command:

```text
go test ./tests/platformtest -run TestRunRedactsSensitiveValuesFromDriverOutputAndEvidence -count=1
```

Result before fix:

```text
--- FAIL: TestRunRedactsSensitiveValuesFromDriverOutputAndEvidence (0.00s)
    run_test.go:54: summary leaked secret: {ScenarioID:t01-2 Seam:test-redaction-driver-output Passed:false Summary:driver saw super-secret-value ...}
FAIL
FAIL	github.com/1123786563/myqypt/tests/platformtest	0.139s
FAIL
```

### Green

Command:

```text
go test ./tests/platformtest -run TestRunRejectsUnknownSeamWithoutLeakingInput -count=1
```

Result:

```text
ok  	github.com/1123786563/myqypt/tests/platformtest	3.203s
```

Command:

```text
go test ./tests/platformtest -run TestRunRedactsSensitiveValuesFromDriverOutputAndEvidence -count=1
```

Result:

```text
ok  	github.com/1123786563/myqypt/tests/platformtest	0.140s
```

## Focused Verification

Command:

```text
gofmt -w tests/platformtest/*.go
go test ./tests/platformtest ./internal/platform -count=1
```

Result:

```text
ok  	github.com/1123786563/myqypt/tests/platformtest	0.132s
ok  	github.com/1123786563/myqypt/internal/platform	0.189s
```

## Compose Validation

Command:

```text
PLATFORM_POSTGRES_DB=platform \
KEYCLOAK_POSTGRES_DB=keycloak \
KEYCLOAK_POSTGRES_USER=keycloak \
PLATFORM_POSTGRES_PASSWORD=platform-pass \
KEYCLOAK_POSTGRES_PASSWORD=keycloak-pass \
KEYCLOAK_ADMIN=admin \
KEYCLOAK_ADMIN_PASSWORD=admin-pass \
docker compose -f deploy/compose/compose.yaml config
```

Result:

```text
name: compose
services:
  keycloak:
    image: quay.io/keycloak/keycloak:26.3
    ...
  postgres:
    image: postgres:17
    environment:
      POSTGRES_DB: platform
      POSTGRES_PASSWORD: platform-pass
      POSTGRES_USER: platform
    ...
```

Outcome: configuration resolved successfully with env-only credentials and no embedded secrets in the committed file.

## Compose Up Evidence / Environment Blocker

Command:

```text
PLATFORM_POSTGRES_DB=platform \
KEYCLOAK_POSTGRES_DB=keycloak \
KEYCLOAK_POSTGRES_USER=keycloak \
PLATFORM_POSTGRES_PASSWORD=platform-pass \
KEYCLOAK_POSTGRES_PASSWORD=keycloak-pass \
KEYCLOAK_ADMIN=admin \
KEYCLOAK_ADMIN_PASSWORD=admin-pass \
docker compose -f deploy/compose/compose.yaml up -d --wait
```

Observed behavior:

- Docker daemon was available and began pulling images immediately.
- The command did not reach container creation or health results during the task window.
- Output repeatedly showed ongoing image pulls only, for example:

```text
Image quay.io/keycloak/keycloak:26.3 Pulling
Image golang:1.26.3 Pulling
...
d8d53d81be5b Downloading 31.46MB
adba01be3850 Downloading 30.41MB
```

- After an extended wait with no health result and no code/config error surfaced, I interrupted the still-running pull:

```text
Process exited with code 130
```

Conclusion: Compose runtime evidence is blocked by environment pull time / image acquisition, not by a validated Compose syntax error or a missing local Docker daemon.

## Self-Review

- Verified the new harness does not modify `platform.New(platform.Dependencies) http.Handler` or Task 1 behavior.
- Verified focused tests cover the two highest-risk harness requirements from the brief:
  - unknown seam / strict contract rejection,
  - redaction of structured and echoed sensitive values.
- Verified `git diff --check` returned clean.
- Cleaned generated local evidence artifacts out of the worktree before commit by moving them to a temporary location, so only intended source changes were committed.

## Commit

- `2815e4cd79b8ebff2befc6f322cf831b52bd6611` — `build(platform): add executable scaffold and evidence harness`

## Concerns

- `docker compose up -d --wait` remains unproven in this environment because image pulls never completed during the task window; later verification should retry from an environment with the images already cached or with enough time/bandwidth to finish the pulls.
- The Keycloak healthcheck uses a shell-level HTTP probe on port `9000`; it is Compose-valid, but runtime validation is still pending because the Keycloak image never finished downloading.

## Fix Pass 1: Review Findings From Commit `2815e4c`

### Scope

- Stop persisting scenario customer content into evidence artifacts.
- Stop persisting driver-only free text that could contain prompts, document excerpts, payment payloads, or sensitive personal information.
- Require `PLATFORM_POSTGRES_USER` via env interpolation so PostgreSQL credentials are fully env-only in committed Compose content.
- Add natural coverage for duplicate seam registration, successful execution, and timeout handling.

### Red Evidence

#### Red 1: Scenario snapshot leaked customer content

Command:

```text
go test ./tests/platformtest -run 'TestRunDoesNotPersistScenarioCustomerContent|TestRunRedactsDriverOnlySensitiveTextFromReturnedReportAndEvidence|TestRunRecordsTimeoutWhenDriverExceedsScenarioDeadline' -count=1
```

Result before fix:

```text
--- FAIL: TestRunDoesNotPersistScenarioCustomerContent (0.00s)
    run_test.go:57: evidence leaked "hello from customer": {
          "scenario": {
            "inputs": {
              "email": "alice@example.com",
              "message": "hello from customer"
            },
            "metadata": {
              "customer_name": "Alice Example"
            },
            ...
          }
        }
--- FAIL: TestRunRedactsDriverOnlySensitiveTextFromReturnedReportAndEvidence (0.00s)
    run_test.go:105: summary="prompt: summarize customer email alice@example.com" want "driver summary omitted from evidence"
--- FAIL: TestRunRecordsTimeoutWhenDriverExceedsScenarioDeadline (0.00s)
    run_test.go:186: failure_reason="driver_error" want driver_timeout
FAIL
FAIL	github.com/1123786563/myqypt/tests/platformtest	2.544s
FAIL
```

#### Red 2: Compose did not fail fast when `PLATFORM_POSTGRES_USER` was absent

Command:

```text
PLATFORM_POSTGRES_DB=platform \
PLATFORM_POSTGRES_PASSWORD=platform-pass \
KEYCLOAK_POSTGRES_DB=keycloak \
KEYCLOAK_POSTGRES_USER=keycloak \
KEYCLOAK_POSTGRES_PASSWORD=keycloak-pass \
KEYCLOAK_ADMIN=admin \
KEYCLOAK_ADMIN_PASSWORD=admin-pass \
docker compose -f deploy/compose/compose.yaml config
```

Result before fix:

```text
name: compose
services:
  postgres:
    environment:
      POSTGRES_DB: platform
      POSTGRES_PASSWORD: platform-pass
      POSTGRES_USER: platform
    healthcheck:
      test:
        - CMD-SHELL
        - pg_isready -U platform
```

Outcome: the command resolved successfully even though `PLATFORM_POSTGRES_USER` was missing, proving the username was still committed instead of env-only.

### Fix Design

- Replaced persisted scenario snapshots with minimal evidence metadata only: `id`, `seam`, optional `timeout`, and optional `assertion_count`.
- Replaced driver-supplied `Summary` and assertion `Details` with fixed safe placeholders unless the text is one of the harness’s own status messages.
- Added explicit timeout classification so context deadline failures are recorded as `driver_timeout` with summary `scenario execution timed out`.
- Switched PostgreSQL Compose username wiring to `${PLATFORM_POSTGRES_USER:?PLATFORM_POSTGRES_USER is required}` in both environment and healthcheck interpolation.

### Green Evidence

#### Focused harness regression suite

Command:

```text
go test ./tests/platformtest -run 'TestRunDoesNotPersistScenarioCustomerContent|TestRunRedactsDriverOnlySensitiveTextFromReturnedReportAndEvidence|TestRunRecordsTimeoutWhenDriverExceedsScenarioDeadline|TestRegisterPanicsOnDuplicateSeam|TestRunRecordsSuccessfulDriverExecution' -count=1
```

Result:

```text
ok  	github.com/1123786563/myqypt/tests/platformtest	3.171s
```

#### Full covering Go verification

Command:

```text
go test ./tests/platformtest ./internal/platform -count=1
```

Result:

```text
ok  	github.com/1123786563/myqypt/tests/platformtest	0.127s
ok  	github.com/1123786563/myqypt/internal/platform	0.188s
```

#### Compose fail-fast verification

Command:

```text
PLATFORM_POSTGRES_DB=platform \
PLATFORM_POSTGRES_PASSWORD=platform-pass \
KEYCLOAK_POSTGRES_DB=keycloak \
KEYCLOAK_POSTGRES_USER=keycloak \
KEYCLOAK_POSTGRES_PASSWORD=keycloak-pass \
KEYCLOAK_ADMIN=admin \
KEYCLOAK_ADMIN_PASSWORD=admin-pass \
docker compose -f deploy/compose/compose.yaml config
```

Result after fix:

```text
error while interpolating services.postgres.environment.POSTGRES_USER: required variable PLATFORM_POSTGRES_USER is missing a value: PLATFORM_POSTGRES_USER is required
```

#### Compose success verification with full env

Command:

```text
PLATFORM_POSTGRES_DB=platform \
PLATFORM_POSTGRES_USER=platform \
PLATFORM_POSTGRES_PASSWORD=platform-pass \
KEYCLOAK_POSTGRES_DB=keycloak \
KEYCLOAK_POSTGRES_USER=keycloak \
KEYCLOAK_POSTGRES_PASSWORD=keycloak-pass \
KEYCLOAK_ADMIN=admin \
KEYCLOAK_ADMIN_PASSWORD=admin-pass \
docker compose -f deploy/compose/compose.yaml config
```

Result:

```text
name: compose
services:
  postgres:
    environment:
      POSTGRES_DB: platform
      POSTGRES_PASSWORD: platform-pass
      POSTGRES_USER: platform
    healthcheck:
      test:
        - CMD-SHELL
        - pg_isready -U platform
```

Outcome: committed Compose credentials now fail fast when incomplete and resolve cleanly when all required env vars are present.

### Fix Concerns

- I did not rerun `docker compose up -d --wait` in this fix pass because the reviewed defect was the env-only interpolation contract, and the earlier runtime blocker was unrelated image-pull latency rather than a validated configuration error. The prior blocker evidence remains accurate until a later environment with completed image pulls can rerun the stack bring-up.
