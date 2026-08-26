# Foundation architecture gates for Issue #105 (F05).
#
# Targets (implementation plan, design ruling 7):
#   generate-check    code generation is deterministic (go generate + clean diff)
#   policy-check      dependency policy gates: Go import/content scan + frontend
#   test-foundation   phased tests (UNIT / CONTRACT / INTEGRATION), reported
#                     separately; INTEGRATION refuses to run green without
#                     TEST_DATABASE_URL
#   verify-foundation unified entry: GENERATE POLICY UNIT CONTRACT INTEGRATION
#                     FRONTEND META, evidence JSON in artifacts/

.PHONY: generate-check policy-check test-foundation verify-foundation

generate-check:
	go generate ./...
	git diff --exit-code -- internal/transport/http/api

policy-check:
	go test ./internal/architecture -run '^TestDependencyPolicy' -count=1 -v
	node scripts/check-frontend-policy.mjs

test-foundation:
	bash scripts/verify-foundation.sh --phases UNIT,CONTRACT,INTEGRATION

verify-foundation:
	bash scripts/verify-foundation.sh
