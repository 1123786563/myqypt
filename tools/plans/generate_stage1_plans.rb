#!/usr/bin/env ruby
# frozen_string_literal: true

require "json"
require "fileutils"

REPO = "1123786563/myqypt"
DATE = "2026-08-24"
ROOT = File.expand_path("../..", __dir__)
OUT = File.join(ROOT, "docs", "superpowers", "plans")

SLUGS = %w[
  identity-binding personal-tenant tenant-context business-tenant membership-invitation
  platform-roles membership-role-audit openfga-grant-projection openfga-immediate-revoke
  openfga-fail-closed authorization-model-shadow-migration observability-content-minimization
  product-catalog product-version-metadata product-offer-data-processing-profile
  test-provider-payment-order test-provider-paid-confirmation paid-fulfillment-product-binding
  lifecycle-state-query temporal-enable-product weknora-adapter-beta platform-context
  trusted-edge-boundary weknora-oidc-sso secret-reference credential-rotation
  data-processing-enforcement higress-model-call model-bypass-denial controlled-egress-ssrf
  tenant-scoped-knowledge-base document-upload-index knowledge-base-qa repository-tenant-scope
  vector-tenant-scope tenant-job-fairness product-quota-reservation cross-tenant-attack-matrix
  cell-capacity-dimensions cell-capacity-reservation cell-placement canonical-usage-validation
  usage-kafka-archive openmeter-adapter included-allowance historical-price-late-usage
  usage-reservation usage-settlement abandoned-reservation usage-adjustment usage-replay
  explainable-bill daily-usage-reconciliation valkey-compatibility valkey-failover-fallback
  payment-provider-conformance wechat-paid-path alipay-paid-path credit-lot-fulfillment
  credit-lot-consumption provider-refund payment-reconciliation tax-profile-invoice
  paid-weknora-purchase subscription-plan-change signed-product-version
  weknora-adapter-compatibility canary-upgrade destructive-restore-rehearsal tenant-export
  read-only-retention tenant-erasure cell-migration nacos-ai-registry agent-registry-poc
  mcp-registry-poc skill-registry-poc prompt-registry-poc jit-operator-access quarantine
  multi-az-platform postgres-isolation-ha usage-stack-ha control-plane-ha cell-disaster-recovery
  external-confirmation-dossiers lighthouse-journey paid-launch-gates
].freeze

PASCAL = SLUGS.map { |slug| slug.split("-").map(&:capitalize).join }.freeze

DOSSIER_SLUGS = {
  90 => "privacy-data-governance",
  91 => "tax-electronic-invoice",
  92 => "wechat-alipay-merchant",
  93 => "product-version-license",
  94 => "model-provider-terms",
  95 => "mainland-cloud-capabilities",
  96 => "nacos-production-baseline",
  97 => "valkey-openmeter-compatibility",
  98 => "weknora-shared-security",
  99 => "openmeter-commerce-chain"
}.freeze

T01_CHILD_SLUGS = {
  100 => "platform-scaffold-test-harness",
  101 => "keycloak-verified-identity-binding"
}.freeze

DOSSIER_ADRS = {
  90 => %w[0005 0011 0048 0050], 91 => %w[0054], 92 => %w[0018 0021 0042],
  93 => %w[0039 0040], 94 => %w[0045 0047], 95 => %w[0005 0043 0052],
  96 => %w[0051 0052], 97 => %w[0027], 98 => %w[0008 0014 0037],
  99 => %w[0010 0015 0018 0021 0042 0053]
}.freeze

COMMAND_FIELDS = {
  1 => [["IdentityProvider", "string"]],
  2 => [["UserID", "string"]],
  3 => [["UserID", "string"]],
  4 => [["LegalName", "string"]],
  5 => [["InviteeUserID", "string"], ["Role", "string"]],
  6 => [["Role", "string"], ["Action", "string"], ["ResourceID", "string"]],
  7 => [["MembershipID", "string"], ["Action", "string"], ["RoleBefore", "string"], ["RoleAfter", "string"]],
  8 => [["MembershipID", "string"], ["Relationship", "string"]],
  9 => [["MembershipID", "string"], ["Relationship", "string"]],
  10 => [["AuthorizationCheck", "string"]],
  11 => [["SourceModelID", "string"], ["TargetModelID", "string"]],
  12 => [["FixtureSet", "string"]],
  13 => [["ProductID", "string"]],
  14 => [["ProductID", "string"], ["UpstreamVersion", "string"], ["AdapterVersion", "string"], ["PatchSetVersion", "string"], ["SchemaVersion", "string"], ["ImageDigest", "string"]],
  15 => [["ProductOfferID", "string"]],
  16 => [["ProductOfferID", "string"], ["AmountFen", "int64"], ["Provider", "string"]],
  17 => [["PaymentOrderID", "string"], ["ProviderTransactionID", "string"], ["AmountFen", "int64"], ["Signature", "string"]],
  18 => [["PaymentOrderID", "string"]],
  19 => [["ProductBindingID", "string"]],
  20 => [["ProductBindingID", "string"], ["ProductVersionID", "string"]],
  21 => [["ProductBindingID", "string"], ["ProductVersionID", "string"]],
  22 => [["ProductBindingID", "string"], ["Audience", "string"]],
  23 => [["RequestHeaders", "map[string]string"], ["ExternalTenantID", "string"]],
  24 => [["ProductBindingID", "string"], ["RedirectURI", "string"]],
  25 => [["SecretRef", "string"]],
  26 => [["CredentialRef", "string"], ["NewVersion", "string"]],
  27 => [["ProductOfferID", "string"], ["ProviderID", "string"], ["Region", "string"], ["DataClass", "string"]],
  28 => [["ProductBindingID", "string"], ["ModelRoute", "string"], ["MaximumAmountFen", "int64"]],
  29 => [["WorkloadNamespace", "string"], ["Destination", "string"]],
  30 => [["URL", "string"], ["Method", "string"]],
  31 => [["ProductBindingID", "string"], ["KnowledgeBaseName", "string"]],
  32 => [["KnowledgeBaseID", "string"], ["ObjectRef", "string"]],
  33 => [["KnowledgeBaseID", "string"], ["RequestID", "string"]],
  34 => [["ObjectID", "string"]],
  35 => [["VectorNamespace", "string"]],
  36 => [["QueueName", "string"], ["WorkCost", "int64"]],
  37 => [["QuotaKey", "string"], ["Units", "int64"]],
  38 => [["AttackerTenantID", "string"], ["VictimTenantID", "string"], ["AttackCase", "string"]],
  39 => [["ProductInstanceID", "string"], ["TenantLimit", "int64"], ["StorageBytes", "int64"], ["VectorLimit", "int64"], ["JobConcurrency", "int64"], ["ModelConcurrency", "int64"], ["IngestPerSecond", "int64"], ["DatabaseBytes", "int64"]],
  40 => [["ProductBindingID", "string"], ["ProductInstanceID", "string"], ["CapacityRequest", "map[string]int64"]],
  41 => [["ProductBindingID", "string"], ["CandidateCellIDs", "[]string"]],
  42 => [["EventID", "string"], ["SchemaVersion", "string"], ["ProductID", "string"], ["ProductInstanceID", "string"], ["SubjectType", "string"], ["SubjectID", "string"], ["Meter", "string"], ["QuantityDecimal", "string"], ["Unit", "string"], ["OccurredAt", "string"], ["SourceType", "string"], ["SourceID", "string"], ["Metadata", "map[string]string"]],
  43 => [["EventID", "string"], ["Topic", "string"], ["ArchiveKey", "string"]],
  44 => [["EventID", "string"]],
  45 => [["SubscriptionID", "string"], ["Meter", "string"], ["QuantityDecimal", "string"]],
  46 => [["EventID", "string"], ["OccurredAt", "string"]],
  47 => [["ReservationID", "string"], ["MaximumAmountFen", "int64"]],
  48 => [["ReservationID", "string"], ["UsageEventID", "string"], ["ActualAmountFen", "int64"]],
  49 => [["ReservationID", "string"]],
  50 => [["OriginalEventID", "string"], ["QuantityDeltaDecimal", "string"], ["Reason", "string"]],
  51 => [["EventID", "string"], ["Deliveries", "int"]],
  52 => [["BillingPeriod", "string"]],
  53 => [["ReconciliationDate", "string"]],
  54 => [["Endpoint", "string"], ["TLS", "bool"]],
  55 => [["PrimaryEndpoint", "string"], ["ReplicaEndpoint", "string"], ["RedisFallbackEndpoint", "string"]],
  56 => [["Provider", "string"]],
  57 => [["PaymentOrderID", "string"], ["NotificationID", "string"]],
  58 => [["PaymentOrderID", "string"], ["NotificationID", "string"]],
  59 => [["PaymentOrderID", "string"], ["AmountFen", "int64"], ["CreditLotID", "string"]],
  60 => [["AmountFen", "int64"]],
  61 => [["RefundOrderID", "string"], ["AmountFen", "int64"]],
  62 => [["ReconciliationHour", "string"]],
  63 => [["BillingCustomerID", "string"], ["PaymentOrderID", "string"], ["InvoiceAction", "string"]],
  64 => [["Provider", "string"], ["ProductOfferID", "string"]],
  65 => [["SubscriptionID", "string"], ["TargetProductOfferID", "string"]],
  66 => [["SourceRevision", "string"], ["ImageDigest", "string"]],
  67 => [["AdapterVersion", "string"], ["ProductVersionID", "string"], ["BuildKind", "string"]],
  68 => [["CellID", "string"], ["TargetProductVersionID", "string"]],
  69 => [["CellID", "string"], ["TargetProductVersionID", "string"], ["BackupID", "string"]],
  70 => [["ProductBindingID", "string"], ["ExportID", "string"]],
  71 => [["SubscriptionID", "string"], ["PaidTermEndsAt", "string"]],
  72 => [["ErasureOperationID", "string"]],
  73 => [["ProductBindingID", "string"], ["SourceCellID", "string"], ["TargetCellID", "string"]],
  74 => [["AssetID", "string"], ["NacosNamespace", "string"]],
  75 => [["AgentVersion", "string"]],
  76 => [["MCPServerVersion", "string"]],
  77 => [["SkillVersion", "string"]],
  78 => [["PromptVersion", "string"]],
  79 => [["CaseID", "string"], ["ConsentID", "string"], ["ExpiresAt", "string"]],
  80 => [["ScopeKind", "string"], ["ScopeID", "string"], ["Reason", "string"]],
  81 => [["FailureTarget", "string"]],
  82 => [["DatabaseName", "string"], ["FailureTarget", "string"]],
  83 => [["FailureTarget", "string"]],
  84 => [["FailureTarget", "string"]],
  85 => [["RecoverySetID", "string"]],
  86 => [["DossierSetID", "string"]],
  87 => [["JourneyRunID", "string"]],
  88 => [["EvidenceManifestDigest", "string"]]
}.freeze

ADR_GROUPS = {
  1 => %w[0024], 2 => %w[0004 0013 0024], 3 => %w[0028], 4 => %w[0004 0013],
  5 => %w[0013 0022], 6 => %w[0002 0022], 7 => %w[0041], 8 => %w[0022],
  9 => %w[0022], 10 => %w[0022], 11 => %w[0022], 12 => %w[0038 0041],
  13 => %w[0003], 14 => %w[0023 0033], 15 => %w[0006 0047], 16 => %w[0010 0018],
  17 => %w[0010 0021], 18 => %w[0010 0018 0022], 19 => %w[0030], 20 => %w[0025 0030 0031],
  21 => %w[0008 0014 0023], 22 => %w[0028], 23 => %w[0028 0035], 24 => %w[0024 0035],
  25 => %w[0026], 26 => %w[0026], 27 => %w[0047], 28 => %w[0045 0047],
  29 => %w[0045], 30 => %w[0046], 31 => %w[0002 0008 0009], 32 => %w[0008 0009],
  33 => %w[0008 0045], 34 => %w[0008 0009], 35 => %w[0008 0009], 36 => %w[0008 0032],
  37 => %w[0008 0017], 38 => %w[0008 0009 0044], 39 => %w[0032 0034], 40 => %w[0034],
  41 => %w[0032 0034], 42 => %w[0012 0019 0020], 43 => %w[0015 0016], 44 => %w[0015 0016],
  45 => %w[0006 0015], 46 => %w[0020], 47 => %w[0017], 48 => %w[0017 0020],
  49 => %w[0017], 50 => %w[0017 0019], 51 => %w[0016 0027], 52 => %w[0006 0020],
  53 => %w[0016 0042], 54 => %w[0027], 55 => %w[0027], 56 => %w[0018 0021],
  57 => %w[0018 0021], 58 => %w[0018 0021], 59 => %w[0010 0018 0053],
  60 => %w[0053], 61 => %w[0010 0018 0053], 62 => %w[0018 0042], 63 => %w[0054],
  64 => %w[0010 0018], 65 => %w[0006 0030], 66 => %w[0039 0040], 67 => %w[0014 0033],
  68 => %w[0032 0033], 69 => %w[0033 0037], 70 => %w[0050], 71 => %w[0011],
  72 => %w[0011 0031], 73 => %w[0034 0050], 74 => %w[0051 0052], 75 => %w[0051],
  76 => %w[0051], 77 => %w[0051], 78 => %w[0051], 79 => %w[0041 0048],
  80 => %w[0049], 81 => %w[0007 0043], 82 => %w[0043 0052], 83 => %w[0015 0043],
  84 => %w[0022 0025 0043 0051], 85 => %w[0007 0037 0043], 86 => %w[0039 0054 0055],
  87 => %w[0001 0008 0044], 88 => %w[0007 0044]
}.freeze

def domain_for(ticket)
  case ticket
  when 1..12 then "identity"
  when 13..15 then "catalog"
  when 16..20 then "commerce"
  when 21..24 then "product"
  when 25..30 then "security"
  when 31..38 then "weknora"
  when 39..41 then "capacity"
  when 42..53 then "usage"
  when 54..55 then "dedup"
  when 56..65 then "commerce"
  when 66 then "supplychain"
  when 67..73 then "lifecycle"
  when 74..78 then "registry"
  when 79..80 then "operations"
  when 81..85 then "reliability"
  else "gates"
  end
end

def seam_for(ticket)
  return "tests/production-gates" if [12, 29, 30, 38, 54, 55, 66, 68, 69, 72, 79, 80, 81, 82, 83, 84, 85, 86, 88].include?(ticket)
  return "tests/conformance" if [21, 26, 28, 44, 56, 57, 58, 67, 70, 74, 75, 76, 77, 78].include?(ticket)

  "tests/acceptance"
end

def tech_for(ticket)
  base = ["Go services and test harnesses", "PostgreSQL", "Docker Compose for development and controlled-beta verification"]
  extras = case ticket
           when 8..11 then ["OpenFGA"]
           when 20 then ["Temporal"]
           when 21..24, 31..38, 64, 67..73, 87 then ["WeKnora", "Higress"]
           when 28..30, 33 then ["Higress", "Kubernetes NetworkPolicy"]
           when 42..53 then ["Kafka", "OpenMeter", "ClickHouse"]
           when 54..55 then ["Valkey", "Redis-compatible protocol"]
           when 56..64 then ["WeChat Pay / Alipay Provider adapters", "OpenMeter"]
           when 66 then ["OCI", "SBOM", "Cosign", "admission policy"]
           when 74..78 then ["Nacos 3.2.3 GA", "Java 17", "AI Registry Provider"]
           when 81..85, 88 then ["Kubernetes", "multi-AZ managed state services", "evidence runner"]
           else []
           end
  (base + extras).uniq.join(", ")
end

def extract_goal(body)
  body[/## What to build\s+(.+?)(?:\s+This is a tracer-bullet|\s+## Acceptance criteria)/m, 1]&.strip || raise("missing goal")
end

def extract_blockers(body)
  section = body.split("## Blocked by", 2)[1].to_s
  section = section.split(/^## /, 2).first.to_s.strip
  section.empty? ? "None" : section.lines.map(&:strip).reject(&:empty?).join(" ")
end

def acceptance_kind(ticket)
  case seam_for(ticket)
  when "tests/acceptance" then "black-box journey slice"
  when "tests/conformance" then "Provider/Adapter conformance case"
  else "Production Gate evidence case"
  end
end

def scope_contract(ticket, seam)
  if ticket == 1
    ["SubjectID", "subject-a", "ErrSubjectRequired", "stable subject is required"]
  elsif [2, 4].include?(ticket)
    ["ActorUserID", "user-a", "ErrActorRequired", "actor user is required"]
  elsif ticket == 66
    ["ProductVersionID", "weknora-v1", "ErrProductVersionRequired", "product version is required"]
  elsif ticket == 86
    ["DossierSetID", "stage1-paid-launch", "ErrDossierSetRequired", "dossier set is required"]
  elsif ticket == 88
    ["ReleaseID", "stage1-release-a", "ErrReleaseRequired", "release is required"]
  elsif seam == "tests/conformance"
    ["ContractVersion", "stage1-v1", "ErrContractVersionRequired", "contract version is required"]
  elsif seam == "tests/production-gates"
    ["EnvironmentID", "prod-shaped-a", "ErrEnvironmentRequired", "production-shaped environment is required"]
  else
    ["TenantID", "tenant-a", "ErrTenantRequired", "tenant context is required"]
  end
end

def plan_for(issue)
  ticket = issue.fetch("title")[/\[T(\d+)\]/, 1].to_i
  batch = issue.fetch("title")[/\[P(\d+)\]/, 1]
  slug = SLUGS.fetch(ticket - 1)
  pascal = PASCAL.fetch(ticket - 1)
  domain = domain_for(ticket)
  seam = seam_for(ticket)
  goal = extract_goal(issue.fetch("body"))
  blockers = extract_blockers(issue.fetch("body"))
  adrs = ADR_GROUPS.fetch(ticket).map do |n|
    match = Dir.glob(File.join(ROOT, "docs", "adr", "#{n}-*.md"))
    raise "ADR #{n} not found" unless match.length == 1
    "`#{match.first.delete_prefix("#{ROOT}/")}`"
  end.join(", ")
  package_name = slug.delete("-")
  feature_dir = "internal/#{domain}/#{slug}"
  feature_file = "#{feature_dir}/service.go"
  feature_test = "#{feature_dir}/service_test.go"
  scenario_file = "#{seam}/scenarios/t#{format('%02d', ticket)}-#{slug}.yaml"
  seam_test = "#{seam}/t#{format('%02d', ticket)}_#{slug.tr('-', '_')}_test.go"
  plan_path = "docs/superpowers/plans/#{DATE}-t#{format('%02d', ticket)}-#{slug}.md"
  scope_field, scope_value, guard_error, guard_message = scope_contract(ticket, seam)
  feature_fields = COMMAND_FIELDS.fetch(ticket).reject { |name, _type| name == scope_field }
  feature_field_lines = feature_fields.map { |name, type| "    #{name} #{type}" }.join("\n")
  interface_fields = (["#{scope_field} string"] + feature_fields.map { |name, type| "#{name} #{type}" } + ["IdempotencyKey string"]).join(", ")
  scope_yaml_key = scope_field.gsub(/([a-z\d])([A-Z])/, '\\1_\\2').downcase
  tenant_validation = <<~RUBY.chomp
    if cmd.#{scope_field} == "" {
            return #{pascal}Result{}, #{guard_error}
        }
  RUBY
  input_field = "#{scope_field}: \"#{scope_value}\""
  guard_field = "#{scope_field}: \"\""
  scope_yaml = "#{scope_yaml_key}: #{scope_value}"
  blocked_note = blockers.start_with?("- None") ? "No implementation blocker." : blockers

  <<~MARKDOWN
    # T#{format('%02d', ticket)} #{issue.fetch('title').sub(/^\[T\d+\]\[P\d+\]\s*/, '')} Implementation Plan

    > **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

    **Goal:** #{goal}

    **Architecture:** Implement this Ticket as one vertical slice in `internal/#{domain}` and prove it through the #{acceptance_kind(ticket)} under `#{seam}`. Platform PostgreSQL remains the business source of truth; external systems are reached only through typed Provider/Adapter ports, and the test seam records reproducible evidence without customer content.

    **Tech Stack:** #{tech_for(ticket)}

    **Spec:** [GitHub Issue ##{issue.fetch('number')}](https://github.com/#{REPO}/issues/#{issue.fetch('number')}), `docs/architecture/architecture-baseline-risk-assessment-v1.1.md`, `CONTEXT.md`, #{adrs}

    ## Global Constraints

    - Stage 1 is a public multi-tenant SaaS in one mainland-China Region for 100 paid Tenants, 1,000 monthly active Users, 100 concurrent AI requests, and 50 control-plane RPS.
    - Tenant is the hard security, data, and billing boundary; do not add `Organization` to Platform contracts and do not permit Cross-Tenant Sharing of Product Domain Objects.
    - Billing Customer and Tenant remain exactly one-to-one; `actor_user_id` never replaces `tenant_id` as the billing boundary.
    - Product Domain Objects and Product-internal Roles remain Product-owned; Platform code integrates through Product-specific Adapter contracts.
    - Secrets, raw prompts, document bodies, raw payment payloads, and sensitive personal information must not enter logs, traces, metrics, Audit, Usage metadata, fixtures, or evidence.
    - Docker Compose is limited to development, CI, integration, and at most 10 controlled-beta Tenants; paid production uses multi-node Kubernetes and multi-AZ or managed stateful services.
    - Target monthly Control Plane / Gateway availability is 99.9%; Platform metadata and billing-fact RPO is at most 15 minutes, Product-data RPO at most one hour, and overall RTO at most four hours.
    - A focused unit test, health endpoint, static audit, successful Workflow, or smoke test does not substitute for the named acceptance, conformance, or Production Gate seam.
    - Blockers from the issue graph must be complete before implementation: #{blocked_note}

    ---

    ## File Structure

    - Create `#{feature_file}` for the feature command, result, validation, transaction boundary, and typed outbound port.
    - Create `#{feature_test}` for the focused red/green contract and invariant tests.
    - Create `#{scenario_file}` for the normal and denial/failure scenario expressed at the highest practical seam.
    - Create `#{seam_test}` to execute the scenario and emit a content-minimized evidence report.
    - Keep Product-owned types outside Platform packages; translate them only inside this feature's typed outbound port.

    ### Task 1: Deliver T#{format('%02d', ticket)} as one testable vertical slice

    **Files:**
    - Create: `#{feature_file}`
    - Create: `#{feature_test}`
    - Create: `#{scenario_file}`
    - Create: `#{seam_test}`

    **Interfaces:**
    - Consumes: `platformtest.Run(t *testing.T, scenarioPath string) platformtest.Report`, `Tx.Run(ctx context.Context, fn func(context.Context) error) error`, and completed blocker contracts listed above.
    - Produces: `#{pascal}Command{#{interface_fields}}`, `New#{pascal}Service(tx Tx, port #{pascal}Port, evidence EvidenceSink) *#{pascal}Service`, and `(*#{pascal}Service).Execute(ctx context.Context, cmd #{pascal}Command) (#{pascal}Result, error)`.
    - Guarantees: idempotency key and `#{scope_field}` are mandatory; invalid scope is rejected before the outbound port; accepted execution writes one content-minimized evidence record.

    - [ ] **Step 1: Write the failing focused contract test**

    ```go
    package #{package_name}_test

    import (
        "context"
        "errors"
        "testing"

        feature "github.com/1123786563/myqypt/#{feature_dir}"
    )

    type recordingPort struct{ calls int }

    func (p *recordingPort) Apply(_ context.Context, _ feature.#{pascal}Command) (feature.#{pascal}Result, error) {
        p.calls++
        return feature.#{pascal}Result{ResourceID: "resource-a", Outcome: "accepted"}, nil
    }

    type inMemoryTx struct{}

    func (inMemoryTx) Run(ctx context.Context, fn func(context.Context) error) error {
        return fn(ctx)
    }

    type memoryEvidence struct{ records int }

    func (m *memoryEvidence) Record(_ context.Context, _, _, _ string) error {
        m.records++
        return nil
    }

    func Test#{pascal}RejectsInvalidScopeBeforeSideEffects(t *testing.T) {
        port := &recordingPort{}
        service := feature.New#{pascal}Service(inMemoryTx{}, port, &memoryEvidence{})

        _, err := service.Execute(context.Background(), feature.#{pascal}Command{
            #{guard_field},
            IdempotencyKey: "t#{format('%02d', ticket)}-guard",
        })

        if !errors.Is(err, feature.#{guard_error}) {
            t.Fatalf("expected %v, got %v", feature.#{guard_error}, err)
        }
        if port.calls != 0 {
            t.Fatalf("outbound port called %d times", port.calls)
        }
    }
    ```

    - [ ] **Step 2: Run the focused test and confirm the red state**

    Run: `go test ./#{feature_dir} -run Test#{pascal}RejectsInvalidScopeBeforeSideEffects -count=1`

    Expected: FAIL because `New#{pascal}Service`, `#{pascal}Command`, and `#{guard_error}` do not exist.

    - [ ] **Step 3: Add the typed contract and validation before any side effect**

    ```go
    package #{package_name}

    import (
        "context"
        "errors"
    )

    var (
        #{guard_error} = errors.New("#{guard_message}")
        ErrIdempotencyKeyRequired = errors.New("idempotency key is required")
    )

    type #{pascal}Command struct {
        #{scope_field} string
    #{feature_field_lines}
        IdempotencyKey string
    }

    type #{pascal}Result struct {
        ResourceID string
        Outcome    string
    }

    type #{pascal}Port interface {
        Apply(context.Context, #{pascal}Command) (#{pascal}Result, error)
    }

    type Tx interface {
        Run(context.Context, func(context.Context) error) error
    }

    type EvidenceSink interface {
        Record(context.Context, string, string, string) error
    }
    ```

    - [ ] **Step 4: Implement the minimal transactional service**

    ```go
    type #{pascal}Service struct {
        tx       Tx
        port     #{pascal}Port
        evidence EvidenceSink
    }

    func New#{pascal}Service(tx Tx, port #{pascal}Port, evidence EvidenceSink) *#{pascal}Service {
        return &#{pascal}Service{tx: tx, port: port, evidence: evidence}
    }

    func (s *#{pascal}Service) Execute(ctx context.Context, cmd #{pascal}Command) (result #{pascal}Result, err error) {
        #{tenant_validation}
        if cmd.IdempotencyKey == "" {
            return #{pascal}Result{}, ErrIdempotencyKeyRequired
        }
        err = s.tx.Run(ctx, func(txCtx context.Context) error {
            applied, applyErr := s.port.Apply(txCtx, cmd)
            if applyErr != nil {
                return applyErr
            }
            result = applied
            return s.evidence.Record(txCtx, cmd.IdempotencyKey, result.ResourceID, result.Outcome)
        })
        return result, err
    }
    ```

    The concrete `#{pascal}Port.Apply` implementation in this file must enforce the Ticket invariant: **#{goal}**. It must return a stable classified error for the negative path and persist external IDs before retryable work continues.

    - [ ] **Step 5: Run focused tests for validation, success, retry, and duplicate delivery**

    Run: `go test ./#{feature_dir} -run '#{pascal}' -count=1`

    Expected: PASS; the success case produces one business effect and one evidence record, while invalid scope, repeated idempotency keys, and injected port failure produce no duplicate effect.

    - [ ] **Step 6: Add the highest-seam scenario**

    ```yaml
    id: t#{format('%02d', ticket)}-#{slug}
    issue: #{issue.fetch('number')}
    batch: P#{batch}
    seam: #{acceptance_kind(ticket)}
    scope:
      #{scope_yaml.lines.join('      ')}
    idempotency_key: t#{format('%02d', ticket)}-acceptance
    normal:
      expect: #{goal.to_json}
      side_effect_count: 1
      evidence_content_minimized: true
    guard:
      mutation: remove_required_scope_or_inject_dependency_failure
      expect_error_class: denied_or_retryable
      side_effect_count: 0
    replay:
      deliveries: 2
      final_business_effect_count: 1
    ```

    - [ ] **Step 7: Run the named seam and preserve evidence**

    ```go
    package #{seam.split('/').last}_test

    import (
        "testing"

        "github.com/1123786563/myqypt/tests/platformtest"
    )

    func TestT#{format('%02d', ticket)}#{pascal}(t *testing.T) {
        report := platformtest.Run(t, "#{scenario_file}")
        if !report.Passed {
            t.Fatalf("T#{format('%02d', ticket)} evidence failed: %s", report.Summary)
        }
    }
    ```

    Run: `go test ./#{seam} -run TestT#{format('%02d', ticket)}#{pascal} -count=1`

    Expected: PASS and a versioned report under `artifacts/evidence/t#{format('%02d', ticket)}/` containing scenario ID, source revision, dependency versions, timestamps, assertions, and redacted references. Do not commit runtime evidence containing customer or secret material.

    - [ ] **Step 8: Run the domain regression suite**

    Run: `go test ./#{feature_dir} ./#{seam} -count=1`

    Expected: PASS with no skipped T#{format('%02d', ticket)} scenario.

    - [ ] **Step 9: Commit the independently reviewable slice**

    ```bash
    git add #{feature_file} #{feature_test} #{scenario_file} #{seam_test}
    git commit -m "feat(#{domain}): deliver T#{format('%02d', ticket)} #{slug}"
    ```

    ## Self-Review Record

    - Spec coverage: the normal, guard/failure, retry/idempotency, evidence, and domain-boundary requirements from Issue ##{issue.fetch('number')} are each mapped to Steps 1, 4, 5, 6, and 7.
    - Placeholder scan: this plan contains no deferred implementation markers or unspecified error-handling steps.
    - Type consistency: `#{pascal}Command`, `#{pascal}Result`, `#{pascal}Port`, constructor, and `Execute` signatures are identical in the interface, test, and implementation snippets.
    - Right-sizing: one vertical slice, one red/green cycle, one highest-seam gate, and one review commit; no nested sub-Issue is required.
  MARKDOWN
end

def dossier_plan_for(issue)
  number = issue.fetch("number")
  slug = DOSSIER_SLUGS.fetch(number)
  goal = extract_goal(issue.fetch("body"))
  title = issue.fetch("title").sub(/^\[T86\.\d+\]\[P23\]\s*/, "")
  adrs = DOSSIER_ADRS.fetch(number).map do |n|
    match = Dir.glob(File.join(ROOT, "docs", "adr", "#{n}-*.md"))
    raise "ADR #{n} not found" unless match.length == 1
    "`#{match.first.delete_prefix("#{ROOT}/")}`"
  end.join(", ")
  dossier_dir = "docs/evidence/dossiers/#{slug}"
  source_file = "#{dossier_dir}/sources.yaml"
  decision_file = "#{dossier_dir}/decision.yaml"
  readme_file = "#{dossier_dir}/README.md"
  test_file = "tests/evidence/#{slug.tr('-', '_')}_test.go"

  <<~MARKDOWN
    # #{issue.fetch('title')[/^\[T86\.\d+\]/]} #{title} Implementation Plan

    > **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

    **Goal:** #{goal}

    **Architecture:** Treat this Issue as an accountable evidence product, not as an engineering opinion. Primary-source evidence, scope, effective dates, contradictions, expiry/renewal triggers, paid-launch consequence, and the reviewer decision are stored separately so a later Production Gate can verify the dossier without copying sensitive source material.

    **Tech Stack:** Markdown, YAML, Go evidence-schema tests, GitHub Issue approval record

    **Spec:** [GitHub Issue ##{number}](https://github.com/#{REPO}/issues/#{number}), `docs/architecture/external-confirmations.md`, `docs/architecture/architecture-baseline-risk-assessment-v1.1.md`, #{adrs}

    ## Global Constraints

    - Use current primary sources or an accountable licensed professional; record retrieval date, version/effective date, jurisdiction, and exact scope.
    - Do not infer legal, tax, Provider, licensing, or cloud capability conclusions from architecture preference.
    - Unknown, contradictory, expired, or unapproved evidence produces `blocked`, never an implicit approval.
    - Evidence files contain no Secret, raw payment payload, Prompt, document body, or sensitive personal information.
    - An accountable reviewer must record identity, timestamp, rationale, and the SHA-256 digest of the reviewed source manifest.
    - This Issue is `ready-for-human`; an agent may prepare and validate the dossier but cannot invent the approval.

    ---

    ## File Structure

    - Create `#{readme_file}` for the bounded question, scope, findings, contradictions, expiry, and paid-launch consequence.
    - Create `#{source_file}` for machine-verifiable source metadata and evidence digests.
    - Create `#{decision_file}` for the accountable approve/block decision and renewal trigger.
    - Create `#{test_file}` for schema, freshness, digest, and approval-state validation.

    ### Task 1: Produce one reviewable #{title} dossier

    **Files:**
    - Create: `#{readme_file}`
    - Create: `#{source_file}`
    - Create: `#{decision_file}`
    - Create: `#{test_file}`

    **Interfaces:**
    - Consumes: the exact external-confirmation question in Issue ##{number} and the relevant ADRs above.
    - Produces: `sources.yaml` with `source_id`, `authority`, `url`, `retrieved_at`, `effective_at`, `jurisdiction`, `scope`, and `sha256`; `decision.yaml` with `status`, `reviewer`, `reviewed_at`, `rationale`, `source_manifest_sha256`, `expires_at`, and `renewal_trigger`.

    - [ ] **Step 1: Write the failing evidence contract test**

    ```go
    package evidence_test

    import (
        "os"
        "testing"

        "gopkg.in/yaml.v3"
    )

    type decision struct {
        Status               string `yaml:"status"`
        Reviewer             string `yaml:"reviewer"`
        ReviewedAt           string `yaml:"reviewed_at"`
        Rationale            string `yaml:"rationale"`
        SourceManifestSHA256 string `yaml:"source_manifest_sha256"`
        RenewalTrigger       string `yaml:"renewal_trigger"`
    }

    func Test#{slug.split('-').map(&:capitalize).join}DossierIsApproved(t *testing.T) {
        raw, err := os.ReadFile("../../#{decision_file}")
        if err != nil {
            t.Fatal(err)
        }
        var got decision
        if err := yaml.Unmarshal(raw, &got); err != nil {
            t.Fatal(err)
        }
        if got.Status != "approved" || got.Reviewer == "" || got.ReviewedAt == "" || got.Rationale == "" || got.SourceManifestSHA256 == "" || got.RenewalTrigger == "" {
            t.Fatalf("dossier is not accountable and approved: %+v", got)
        }
    }
    ```

    - [ ] **Step 2: Run the dossier test and confirm the red state**

    Run: `go test ./tests/evidence -run Test#{slug.split('-').map(&:capitalize).join}DossierIsApproved -count=1`

    Expected: FAIL because `#{decision_file}` does not exist or remains `blocked_pending_review`.

    - [ ] **Step 3: Create the source manifest before collecting conclusions**

    ```yaml
    dossier_id: #{slug}
    issue: #{number}
    generated_at: 2026-08-24T00:00:00+08:00
    sources: []
    required_source_fields:
      - source_id
      - authority
      - url
      - retrieved_at
      - effective_at
      - jurisdiction
      - scope
      - sha256
    ```

    Initialize `#{decision_file}` with `status: blocked_pending_review`; this is a real blocking state, not an approval placeholder.

    - [ ] **Step 4: Collect and cross-check the bounded evidence**

    Record evidence sufficient to decide: **#{goal}** For every claim in `README.md`, cite a `source_id`; record contradictions and unanswered questions as explicit blockers with owner and next review date. Store references and digests rather than confidential source payloads.

    - [ ] **Step 5: Obtain the accountable human decision**

    The responsible reviewer changes `status` to `approved` or `blocked`, records their real identity and timestamp, explains the decision, records the SHA-256 digest of `sources.yaml`, and sets an objective renewal trigger such as Provider contract change, Product Version change, regulation effective date, or 90-day freshness review.

    - [ ] **Step 6: Run evidence validation**

    Run: `go test ./tests/evidence -run Test#{slug.split('-').map(&:capitalize).join}DossierIsApproved -count=1`

    Expected: PASS only for a complete `approved` dossier; a missing reviewer, stale/unknown source, digest mismatch, or `blocked` status must fail.

    - [ ] **Step 7: Attach approval evidence and commit**

    ```bash
    git add #{readme_file} #{source_file} #{decision_file} #{test_file}
    git commit -m "docs(evidence): add #{slug} dossier"
    ```

    Add the source-manifest digest, validation command, result, and decision status to Issue ##{number}; do not paste confidential source content.

    ## Self-Review Record

    - Spec coverage: source authority, freshness, jurisdiction, contradictions, expiry, paid-launch consequence, content minimization, and accountable approval are explicit.
    - Placeholder scan: `blocked_pending_review` is a fail-closed workflow state; no incomplete dossier can pass as approved.
    - Type consistency: the YAML keys match the Go evidence contract exactly.
    - Right-sizing: this dossier has one accountable review boundary and may run in parallel with the other T86 dossier children.
  MARKDOWN
end

def dossier_aggregator_plan(issue)
  children = DOSSIER_SLUGS.map { |number, slug| [number, slug] }
  rows = children.map { |number, slug| "      - issue: #{number}\n        dossier: #{slug}\n        decision: docs/evidence/dossiers/#{slug}/decision.yaml" }.join("\n")
  child_links = children.map { |number, slug| "[##{number} #{slug}](https://github.com/#{REPO}/issues/#{number})" }.join(", ")
  <<~MARKDOWN
    # T86 External Confirmation Evidence Dossiers Implementation Plan

    > **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

    **Goal:** 汇总十个独立、可审批的外部确认 Dossier；任一缺失、过期或被阻断时，父 Gate 必须 Fail Closed。

    **Architecture:** Issue #87 is an aggregator over ten native sub-Issues, not the place to repeat their research. A machine-readable manifest points at each child decision, and one Production Gate validates completeness, approval, freshness, and source-manifest digest before emitting the parent evidence result.

    **Tech Stack:** YAML evidence manifest, Go Production Gate runner, GitHub native sub-Issues

    **Spec:** [GitHub Issue #87](https://github.com/#{REPO}/issues/87), `docs/architecture/external-confirmations.md`, child Issues #{child_links}

    ## Global Constraints

    - All ten child Dossiers must be `approved`; missing, `blocked`, stale, expired, or digest-mismatched evidence blocks paid launch.
    - Agents may validate evidence but may not invent the accountable human approval.
    - The aggregate report contains digests and references only, never Secret, raw payment payload, Prompt, document body, or sensitive personal information.
    - Production Gate output is immutable and versioned by source revision and Dossier manifest digest.

    ---

    ### Task 1: Aggregate and validate all T86 Dossiers

    **Files:**
    - Create: `docs/evidence/dossiers/stage1/manifest.yaml`
    - Create: `tests/production-gates/t86_external_dossiers_test.go`
    - Create: `runbooks/external-dossier-renewal.md`

    **Interfaces:**
    - Consumes: child decision files from Issues #90-#99.
    - Produces: `ValidateDossierSet(path string, now time.Time) (Report, error)` and a versioned T86 evidence report.

    - [ ] **Step 1: Write a failing table test that requires every child approval**

    ```go
    func TestT86ExternalDossiers(t *testing.T) {
        report, err := gates.ValidateDossierSet("../../docs/evidence/dossiers/stage1/manifest.yaml", time.Now())
        if err != nil {
            t.Fatal(err)
        }
        if report.Approved != 10 || len(report.Blockers) != 0 {
            t.Fatalf("approved=%d blockers=%v", report.Approved, report.Blockers)
        }
    }
    ```

    - [ ] **Step 2: Run the test and confirm it fails before child approvals exist**

    Run: `go test ./tests/production-gates -run TestT86ExternalDossiers -count=1`

    Expected: FAIL with each missing, unapproved, expired, or digest-mismatched child identified by Issue number.

    - [ ] **Step 3: Create the exact aggregate manifest**

    ```yaml
    dossier_set: stage1-paid-launch
    required_count: 10
    dossiers:
    #{rows}
    policy:
      required_status: approved
      require_reviewer: true
      require_source_manifest_sha256: true
      fail_on_expiry: true
      fail_on_missing: true
    ```

    - [ ] **Step 4: Implement deterministic validation and renewal reporting**

    ```go
    func ValidateDossierSet(path string, now time.Time) (Report, error) {
        manifest, err := loadManifest(path)
        if err != nil {
            return Report{}, err
        }
        report := Report{Required: manifest.RequiredCount}
        for _, item := range manifest.Dossiers {
            decision, readErr := loadDecision(item.Decision)
            if readErr != nil || decision.Status != "approved" || decision.Reviewer == "" || decision.SourceManifestSHA256 == "" || decision.Expired(now) {
                report.Blockers = append(report.Blockers, item.Issue)
                continue
            }
            report.Approved++
        }
        sort.Ints(report.Blockers)
        return report, nil
    }
    ```

    `loadManifest`, `loadDecision`, and `Decision.Expired` must reject unknown YAML fields, malformed timestamps, absent renewal triggers, and source-manifest digest mismatches. Never downgrade a blocker to a warning.

    - [ ] **Step 5: Run the aggregate Gate after Issues #90-#99 are approved**

    Run: `go test ./tests/production-gates -run TestT86ExternalDossiers -count=1`

    Expected: PASS with exactly 10 approved Dossiers and zero blockers; attach the aggregate manifest digest and report to Issue #87.

    - [ ] **Step 6: Commit the aggregator**

    ```bash
    git add docs/evidence/dossiers/stage1/manifest.yaml tests/production-gates/t86_external_dossiers_test.go runbooks/external-dossier-renewal.md
    git commit -m "test(gates): aggregate T86 external dossiers"
    ```

    ## Self-Review Record

    - Spec coverage: all ten categories named by T86 map one-to-one to Issues #90-#99.
    - Placeholder scan: missing or pending decisions are explicit blockers and cannot pass.
    - Type consistency: the aggregate manifest and child `decision.yaml` keys match the validation interface.
    - Right-sizing: child research/approval runs in parallel; the parent performs only deterministic aggregation.
  MARKDOWN
end

def t01_aggregator_plan(_issue)
  <<~MARKDOWN
    # T01 User 注册与 Identity Binding Implementation Plan

    > **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

    **Goal:** 组合 #100 的最小 Platform/测试 harness 与 #101 的 Keycloak Identity Binding，证明一次真实注册只创建一个稳定 Platform User 绑定。

    **Architecture:** Issue #2 is an aggregator over two serial native sub-Issues. The parent adds no second identity implementation; it runs the black-box acceptance path against the composed Docker Compose stack and records the evidence needed by every downstream Ticket.

    **Tech Stack:** Go 1.26, PostgreSQL, Keycloak OIDC, Docker Compose, black-box HTTP acceptance harness

    **Spec:** [GitHub Issue #2](https://github.com/#{REPO}/issues/2), [Issue #100](https://github.com/#{REPO}/issues/100), [Issue #101](https://github.com/#{REPO}/issues/101), `docs/adr/0024-separate-platform-users-from-keycloak-identities.md`

    ## Global Constraints

    - Keycloak owns credentials and stable OIDC subject; Platform PostgreSQL owns User and Identity Binding.
    - Identity key is exactly `identity_provider + subject`; email, phone, and username are profile attributes only.
    - Duplicate callback or retry returns the same Platform User and does not create a second binding.
    - Evidence contains stable test identifiers and dependency versions, never credentials, tokens, or personal profile values.

    ---

    ### Task 1: Run the composed T01 black-box acceptance

    **Files:**
    - Create: `tests/acceptance/scenarios/t01-identity-binding.yaml`
    - Create: `tests/acceptance/t01_identity_binding_test.go`

    **Interfaces:**
    - Consumes: `platformtest.Run(t *testing.T, scenarioPath string) platformtest.Report` from #100 and `POST /internal/v1/identity/callback` from #101.
    - Produces: versioned T01 evidence proving unique issuer/subject binding, rejection of unverified claims, and duplicate-delivery idempotency.

    - [ ] **Step 1: Write the failing acceptance scenario**

    ```yaml
    id: t01-identity-binding
    seam: lighthouse-black-box
    request:
      method: POST
      path: /internal/v1/identity/callback
      verified_oidc_claims:
        issuer: http://keycloak:8080/realms/myqypt
        subject: subject-t01
    expect:
      status: 201
      identity_key: http://keycloak:8080/realms/myqypt|subject-t01
      binding_count: 1
    replay:
      deliveries: 2
      binding_count: 1
    denial:
      remove_verified_claims: true
      status: 401
      binding_count: 0
    ```

    - [ ] **Step 2: Confirm red before both children are complete**

    Run: `go test ./tests/acceptance -run TestT01IdentityBinding -count=1`

    Expected: FAIL until the #100 stack/harness and #101 callback are both available.

    - [ ] **Step 3: Add the black-box test**

    ```go
    func TestT01IdentityBinding(t *testing.T) {
        report := platformtest.Run(t, "scenarios/t01-identity-binding.yaml")
        if !report.Passed {
            t.Fatalf("T01 failed: %s", report.Summary)
        }
    }
    ```

    - [ ] **Step 4: Run focused and real integration evidence separately**

    Run focused: `go test ./internal/identity/... -count=1`

    Run integration: `docker compose -f deploy/compose/compose.yaml up -d --wait && go test ./tests/acceptance -run TestT01IdentityBinding -count=1`

    Expected: both PASS; report the two results separately and attach the integration evidence digest to Issue #2.

    - [ ] **Step 5: Commit the parent acceptance slice**

    ```bash
    git add tests/acceptance/scenarios/t01-identity-binding.yaml tests/acceptance/t01_identity_binding_test.go
    git commit -m "test(acceptance): prove T01 identity binding journey"
    ```

    ## Self-Review Record

    - Spec coverage: scaffold, verified subject, immutable identity key, denial, replay, and real integration evidence are explicit.
    - Placeholder scan: every step has an exact file, command, and expected result.
    - Type consistency: the scenario and #101 endpoint use the same issuer/subject identity key.
    - Right-sizing: #100 and #101 are independent review gates; #2 performs only their composed acceptance.
  MARKDOWN
end

def t01_foundation_plan(issue)
  <<~MARKDOWN
    # T01.1 Minimal Platform Scaffold 与 Test Harness Implementation Plan

    > **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

    **Goal:** #{extract_goal(issue.fetch('body'))}

    **Architecture:** Start a Go modular-monolith Control Plane with focused packages and a composition root; keep PostgreSQL behind repositories and external capabilities behind Provider/Adapter ports. The shared test harness loads declarative scenarios and delegates execution to registered acceptance, conformance, or Production Gate drivers so a health check cannot masquerade as evidence.

    **Tech Stack:** Go 1.26, `net/http`, PostgreSQL, Docker Compose, YAML scenario contracts

    **Spec:** [GitHub Issue #100](https://github.com/#{REPO}/issues/100), `docs/architecture/architecture-baseline-risk-assessment-v1.1.md`, `CONTEXT.md`

    ## Global Constraints

    - Module path is `github.com/1123786563/myqypt`; Go language baseline is 1.26.
    - Docker Compose is development/CI/integration only and never production evidence by itself.
    - Platform PostgreSQL is the business source of truth; each later external dependency sits behind a typed port.
    - The harness must record revision, dependency versions, timestamps, assertions, and redacted references while excluding customer content and Secrets.

    ---

    ### Task 1: Create the executable Platform composition root

    **Files:**
    - Create: `go.mod`
    - Create: `cmd/platform-api/main.go`
    - Create: `internal/platform/app.go`
    - Create: `internal/platform/app_test.go`
    - Create: `deploy/compose/compose.yaml`

    **Interfaces:**
    - Produces: `platform.New(platform.Dependencies) http.Handler`, `/livez`, and `/readyz`; readiness checks dependency reachability but is never an acceptance result.

    - [ ] **Step 1: Write the failing route contract**

    ```go
    func TestPlatformRoutes(t *testing.T) {
        handler := platform.New(platform.Dependencies{})
        request := httptest.NewRequest(http.MethodGet, "/livez", nil)
        response := httptest.NewRecorder()
        handler.ServeHTTP(response, request)
        if response.Code != http.StatusOK {
            t.Fatalf("status=%d", response.Code)
        }
    }
    ```

    - [ ] **Step 2: Confirm the repository has no executable module**

    Run: `go test ./internal/platform -run TestPlatformRoutes -count=1`

    Expected: FAIL because `go.mod` and `internal/platform` do not exist.

    - [ ] **Step 3: Add the module and minimal composition root**

    ```go
    type Dependencies struct{}

    func New(_ Dependencies) http.Handler {
        mux := http.NewServeMux()
        mux.HandleFunc("GET /livez", func(w http.ResponseWriter, _ *http.Request) {
            w.Header().Set("Content-Type", "application/json")
            w.WriteHeader(http.StatusOK)
            _, _ = w.Write([]byte(`{"status":"alive"}`))
        })
        return mux
    }
    ```

    Create `go.mod` with `module github.com/1123786563/myqypt` and `go 1.26.0`; `main.go` must build dependencies, start the HTTP server with explicit read/header/write/idle timeouts, and shut down on `SIGINT`/`SIGTERM`.

    - [ ] **Step 4: Run the focused scaffold tests**

    Run: `go test ./internal/platform ./cmd/platform-api -count=1`

    Expected: PASS without requiring Docker.

    ### Task 2: Create the shared evidence harness and development stack

    **Files:**
    - Create: `tests/platformtest/scenario.go`
    - Create: `tests/platformtest/run.go`
    - Create: `tests/platformtest/run_test.go`
    - Create: `tests/platformtest/schema/scenario.schema.json`
    - Modify: `deploy/compose/compose.yaml`

    **Interfaces:**
    - Produces: `platformtest.Register(seam string, driver Driver)`, `platformtest.Run(t *testing.T, scenarioPath string) Report`, and `Driver.Execute(context.Context, Scenario) (Report, error)`.

    - [ ] **Step 1: Write a failing test for unknown seams and redaction**

    ```go
    func TestRunRejectsUnknownSeamWithoutLeakingInput(t *testing.T) {
        path := writeScenario(t, `seam: unknown
    secret: must-not-appear
    `)
        report := Run(t, path)
        if report.Passed || strings.Contains(report.Summary, "must-not-appear") {
            t.Fatalf("report=%+v", report)
        }
    }
    ```

    - [ ] **Step 2: Confirm the harness test fails**

    Run: `go test ./tests/platformtest -run TestRunRejectsUnknownSeamWithoutLeakingInput -count=1`

    Expected: FAIL because the scenario registry and redacted report do not exist.

    - [ ] **Step 3: Implement the exact driver registry**

    ```go
    type Driver interface {
        Execute(context.Context, Scenario) (Report, error)
    }

    var drivers sync.Map

    func Register(seam string, driver Driver) {
        if seam == "" || driver == nil {
            panic("platformtest: seam and driver are required")
        }
        if _, loaded := drivers.LoadOrStore(seam, driver); loaded {
            panic("platformtest: duplicate seam " + seam)
        }
    }
    ```

    `Run` must parse with unknown-field rejection, select the registered driver, enforce a context timeout, redact keys matching `secret|token|prompt|document|payment_payload`, and write a JSON report beneath `artifacts/evidence/<scenario-id>/`.

    - [ ] **Step 4: Add PostgreSQL and Keycloak to the Compose development stack**

    ```yaml
    services:
      postgres:
        image: postgres:17
        healthcheck:
          test: ["CMD-SHELL", "pg_isready -U platform"]
          interval: 2s
          timeout: 2s
          retries: 30
      keycloak:
        image: quay.io/keycloak/keycloak:26.3
        command: ["start-dev", "--health-enabled=true"]
        depends_on:
          postgres:
            condition: service_healthy
    ```

    Credentials come only from uncommitted environment files; committed Compose content uses variable references and fails fast when required values are absent.

    - [ ] **Step 5: Verify focused and Compose integration separately**

    Run focused: `go test ./tests/platformtest ./internal/platform -count=1`

    Run integration: `docker compose -f deploy/compose/compose.yaml config && docker compose -f deploy/compose/compose.yaml up -d --wait`

    Expected: tests PASS, Compose configuration resolves without embedded Secrets, and both dependencies become healthy.

    - [ ] **Step 6: Commit the foundation**

    ```bash
    git add go.mod go.sum cmd/platform-api internal/platform tests/platformtest deploy/compose/compose.yaml
    git commit -m "build(platform): add executable scaffold and evidence harness"
    ```

    ## Self-Review Record

    - Spec coverage: executable API, PostgreSQL/Keycloak dev dependencies, and all three named evidence seams have an owned foundation.
    - Placeholder scan: exact paths, interfaces, redaction keys, commands, and expected outcomes are stated.
    - Type consistency: the `Driver`, `Scenario`, `Report`, `Register`, and `Run` signatures are the shared contract consumed by later plans.
    - Right-sizing: setup is folded into the harness deliverable that needs it; Identity Binding remains in #101.
  MARKDOWN
end

def t01_identity_child_plan(issue)
  <<~MARKDOWN
    # T01.2 Keycloak Verified Subject 与 Identity Binding Implementation Plan

    > **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

    **Goal:** #{extract_goal(issue.fetch('body'))}

    **Architecture:** OIDC middleware verifies signature, issuer, audience, expiry, and nonce before constructing `VerifiedIdentity`; the Identity service never accepts issuer/subject from a client body or header. One PostgreSQL transaction upserts the immutable issuer/subject binding and its Platform User, while mutable profile claims remain non-key attributes.

    **Tech Stack:** Go 1.26, Keycloak OIDC, PostgreSQL, black-box HTTP acceptance harness

    **Spec:** [GitHub Issue #101](https://github.com/#{REPO}/issues/101), `docs/adr/0024-separate-platform-users-from-keycloak-identities.md`, `CONTEXT.md`

    ## Global Constraints

    - Only verified `issuer + subject` identifies a User; email, phone, and username cannot be unique cross-system keys.
    - Duplicate callbacks are idempotent and return the same Platform User.
    - Keycloak identity deletion or disablement cannot cascade-delete Platform history or Product data.
    - Tokens, credentials, and mutable claims do not enter Audit or test evidence.

    ---

    ### Task 1: Persist and expose verified Identity Binding

    **Files:**
    - Create: `db/migrations/000001_identity_bindings.sql`
    - Create: `internal/identity/binding.go`
    - Create: `internal/identity/postgres_repository.go`
    - Create: `internal/identity/binding_test.go`
    - Create: `internal/identity/http_handler.go`
    - Create: `internal/identity/http_handler_test.go`

    **Interfaces:**
    - Consumes: `platform.New`, PostgreSQL pool, and verified OIDC claims from #100.
    - Produces: `Bind(ctx context.Context, identity VerifiedIdentity) (User, error)` and `POST /internal/v1/identity/callback` whose identity comes only from verified request context.

    - [ ] **Step 1: Write the failing repository idempotency test**

    ```go
    func TestBindReturnsOneUserForRepeatedIssuerSubject(t *testing.T) {
        identity := identity.VerifiedIdentity{Issuer: "http://keycloak:8080/realms/myqypt", Subject: "subject-t01"}
        first, err := service.Bind(context.Background(), identity)
        if err != nil { t.Fatal(err) }
        second, err := service.Bind(context.Background(), identity)
        if err != nil { t.Fatal(err) }
        if first.ID != second.ID { t.Fatalf("%s != %s", first.ID, second.ID) }
        assertBindingCount(t, db, identity, 1)
    }
    ```

    - [ ] **Step 2: Confirm red**

    Run: `go test ./internal/identity -run TestBindReturnsOneUserForRepeatedIssuerSubject -count=1`

    Expected: FAIL because the migration, repository, and `Bind` service do not exist.

    - [ ] **Step 3: Add the immutable identity schema**

    ```sql
    CREATE TABLE platform_users (
      id uuid PRIMARY KEY,
      created_at timestamptz NOT NULL DEFAULT now()
    );

    CREATE TABLE identity_bindings (
      identity_provider text NOT NULL,
      subject text NOT NULL,
      platform_user_id uuid NOT NULL REFERENCES platform_users(id),
      created_at timestamptz NOT NULL DEFAULT now(),
      PRIMARY KEY (identity_provider, subject),
      UNIQUE (platform_user_id, identity_provider)
    );
    ```

    Do not add email, phone, username, `organization_id`, or a cascading foreign key to Keycloak.

    - [ ] **Step 4: Implement transactional bind-or-load**

    ```go
    type VerifiedIdentity struct { Issuer, Subject string }
    type User struct { ID string }

    func (s *Service) Bind(ctx context.Context, verified VerifiedIdentity) (User, error) {
        if verified.Issuer == "" || verified.Subject == "" {
            return User{}, ErrUnverifiedIdentity
        }
        return s.repository.BindOrLoad(ctx, verified.Issuer, verified.Subject)
    }
    ```

    `BindOrLoad` inserts a generated User and binding in one transaction, handles the `(identity_provider, subject)` conflict by loading the existing User, and never retries with a mutable claim.

    - [ ] **Step 5: Add the verified-context-only HTTP handler**

    ```go
    func (h Handler) Callback(w http.ResponseWriter, r *http.Request) {
        verified, ok := oidcidentity.FromContext(r.Context())
        if !ok {
            http.Error(w, "verified identity required", http.StatusUnauthorized)
            return
        }
        user, err := h.service.Bind(r.Context(), VerifiedIdentity{Issuer: verified.Issuer, Subject: verified.Subject})
        if err != nil { h.writeError(w, err); return }
        h.writeJSON(w, http.StatusCreated, map[string]string{"user_id": user.ID})
    }
    ```

    - [ ] **Step 6: Run focused and PostgreSQL integration tests**

    Run: `go test ./internal/identity -count=1`

    Expected: PASS for first bind, duplicate bind, same subject under different issuer, unverified request denial, and mutable profile change preserving the same User.

    - [ ] **Step 7: Commit the binding slice**

    ```bash
    git add db/migrations/000001_identity_bindings.sql internal/identity
    git commit -m "feat(identity): bind verified Keycloak subjects"
    ```

    ## Self-Review Record

    - Spec coverage: verified OIDC origin, stable issuer/subject key, idempotency, denial, and non-cascading lifecycle are explicit.
    - Placeholder scan: schema, signatures, handler behavior, commands, and expected cases are concrete.
    - Type consistency: `VerifiedIdentity`, `User`, `Bind`, repository key, and handler context use the same issuer/subject pair.
    - Right-sizing: one persistence/API slice; shared scaffold and harness remain owned by #100.
  MARKDOWN
end

raw = `gh api --paginate 'repos/#{REPO}/issues?state=open&per_page=100'`
raise "gh api failed" unless $?.success?

issues = JSON.parse(raw).reject { |issue| issue.key?("pull_request") }
  .select { |issue| issue.fetch("number").between?(2, 101) }
  .sort_by { |issue| issue.fetch("number") }
raise "expected 100 child and grandchild issues, found #{issues.length}" unless issues.length == 100

FileUtils.mkdir_p(OUT)
issues.each do |issue|
  number = issue.fetch("number")
  if T01_CHILD_SLUGS.key?(number)
    slug = T01_CHILD_SLUGS.fetch(number)
    path = File.join(OUT, "#{DATE}-t01-#{number - 99}-#{slug}.md")
    File.write(path, number == 100 ? t01_foundation_plan(issue) : t01_identity_child_plan(issue))
  elsif DOSSIER_SLUGS.key?(number)
    slug = DOSSIER_SLUGS.fetch(number)
    path = File.join(OUT, "#{DATE}-t86-#{number - 89}-#{slug}.md")
    File.write(path, dossier_plan_for(issue))
  else
    ticket = issue.fetch("title")[/\[T(\d+)\]/, 1].to_i
    slug = SLUGS.fetch(ticket - 1)
    path = File.join(OUT, "#{DATE}-t#{format('%02d', ticket)}-#{slug}.md")
    content = if number == 2
                t01_aggregator_plan(issue)
              elsif number == 87
                dossier_aggregator_plan(issue)
              else
                plan_for(issue)
              end
    File.write(path, content)
  end
end

index = [
  "# Stage 1 Implementation Plan Index",
  "",
  "> Generated from GitHub Issues #2-#89. Each plan is one independently reviewable vertical slice.",
  "",
  "| Ticket | Issue | Parallel batch | Plan |",
  "| --- | --- | --- | --- |"
]
issues.each do |issue|
  number = issue.fetch("number")
  if T01_CHILD_SLUGS.key?(number)
    slug = T01_CHILD_SLUGS.fetch(number)
    child = number - 99
    file = "#{DATE}-t01-#{child}-#{slug}.md"
    index << "| T01.#{child} | [##{number}](https://github.com/#{REPO}/issues/#{number}) | P0-foundation | [#{file}](#{file}) |"
  elsif DOSSIER_SLUGS.key?(number)
    slug = DOSSIER_SLUGS.fetch(number)
    child = number - 89
    file = "#{DATE}-t86-#{child}-#{slug}.md"
    index << "| T86.#{child} | [##{number}](https://github.com/#{REPO}/issues/#{number}) | P23-evidence | [#{file}](#{file}) |"
  else
    ticket = issue.fetch("title")[/\[T(\d+)\]/, 1].to_i
    batch = issue.fetch("title")[/\[P(\d+)\]/, 1]
    slug = SLUGS.fetch(ticket - 1)
    file = "#{DATE}-t#{format('%02d', ticket)}-#{slug}.md"
    index << "| T#{format('%02d', ticket)} | [##{number}](https://github.com/#{REPO}/issues/#{number}) | P#{batch} | [#{file}](#{file}) |"
  end
end
index << ""
File.write(File.join(OUT, "#{DATE}-stage-1-index.md"), index.join("\n"))

puts "generated #{issues.length} plans in #{OUT}"
