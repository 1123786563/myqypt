# Architecture Decision Record Index

> Generated from the accepted decisions of the architecture grilling session on 2026-08-24.

## Index

| ADR | Decision | Status |
| --- | --- | --- |
| 0001 | [Launch as a public multi-tenant SaaS](../adr/0001-launch-as-a-public-multi-tenant-saas.md) | accepted |
| 0002 | [Products retain ownership of product domain objects](../adr/0002-products-own-their-domain-objects.md) | accepted |
| 0003 | [Use an internally curated Product Catalog](../adr/0003-use-an-internally-curated-product-catalog.md) | accepted |
| 0004 | [Use explicit one-to-one Billing Customer and Tenant ownership](../adr/0004-use-explicit-one-to-one-billing-customer-and-tenant-ownership.md) | accepted |
| 0005 | [Launch in one mainland China region](../adr/0005-launch-in-one-mainland-china-region.md) | accepted |
| 0006 | [Use subscriptions with prepaid overage](../adr/0006-use-subscriptions-with-prepaid-overage.md) | accepted |
| 0007 | [Bound the Stage 1 operating envelope](../adr/0007-bound-the-stage-1-operating-envelope.md) | accepted |
| 0008 | [Require WeKnora Shared-tenancy hardening before paid launch](../adr/0008-require-weknora-shared-tenancy-hardening-before-paid-launch.md) | accepted |
| 0009 | [Forbid cross-Tenant Product Domain Object sharing](../adr/0009-forbid-cross-tenant-product-object-sharing.md) | accepted |
| 0010 | [Separate payment confirmation from fulfillment](../adr/0010-separate-payment-confirmation-from-fulfillment.md) | accepted |
| 0011 | [Erase Product data after Read-only Retention](../adr/0011-erase-product-data-after-read-only-retention.md) | accepted |
| 0012 | [Accept usage only from approved server authorities](../adr/0012-accept-usage-only-from-approved-server-authorities.md) | accepted |
| 0013 | [Separate global User and Tenant lifecycles](../adr/0013-separate-global-user-and-tenant-lifecycles.md) | accepted |
| 0014 | [Maintain WeKnora hardening as an upstream-first Patch Queue](../adr/0014-maintain-weknora-hardening-as-an-upstream-first-patch-queue.md) | accepted |
| 0015 | [Run Kafka and ClickHouse as OpenMeter core dependencies](../adr/0015-run-kafka-and-clickhouse-as-openmeter-core-dependencies.md) | accepted |
| 0016 | [Own a canonical Usage stream outside OpenMeter](../adr/0016-own-a-canonical-usage-stream-outside-openmeter.md) | accepted |
| 0017 | [Use append-only Usage Adjustments and pre-execution Reservations](../adr/0017-use-append-only-adjustments-and-pre-execution-reservations.md) | accepted |
| 0018 | [Own real-money transactions in Platform Commerce](../adr/0018-own-real-money-transactions-in-platform-commerce.md) | accepted |
| 0019 | [Fix Canonical Usage identity and privacy boundaries](../adr/0019-fix-canonical-usage-identity-and-privacy-boundaries.md) | accepted |
| 0020 | [Use fixed-precision rating at occurrence time](../adr/0020-use-fixed-precision-rating-at-occurrence-time.md) | accepted |
| 0021 | [Require Provider-verified idempotent payment transitions](../adr/0021-require-provider-verified-idempotent-payment-transitions.md) | accepted |
| 0022 | [Adopt OpenFGA in Stage 1](../adr/0022-adopt-openfga-in-stage-1.md) | accepted |
| 0023 | [Model Shared Product Instances separately from Tenant bindings](../adr/0023-model-shared-product-instances-separately-from-tenant-bindings.md) | accepted |
| 0024 | [Separate Platform Users from Casdoor identities](../adr/0024-separate-platform-users-from-casdoor-identities.md) | accepted |
| 0025 | [Use Temporal for Stage 1 lifecycle orchestration](../adr/0025-use-temporal-for-stage-1-lifecycle-orchestration.md) | accepted |
| 0026 | [Use managed Secrets before self-hosting OpenBao](../adr/0026-use-managed-secrets-before-self-hosting-openbao.md) | accepted |
| 0027 | [Use Valkey for persistent deduplication](../adr/0027-use-valkey-for-persistent-deduplication.md) | accepted |
| 0028 | [Issue Platform Context only at the trusted edge](../adr/0028-issue-platform-context-only-at-the-trusted-edge.md) | accepted |
| 0029 | [Separate Product User and Membership bindings](../adr/0029-separate-product-user-and-membership-bindings.md) | accepted |
| 0030 | [Separate Desired State, Observed State, and Lifecycle Operation](../adr/0030-separate-desired-observed-and-operation-state.md) | accepted |
| 0031 | [Compensate only reversible lifecycle effects](../adr/0031-compensate-only-reversible-lifecycle-effects.md) | accepted |
| 0032 | [Run Shared Products in capacity-bounded Cells](../adr/0032-run-shared-products-in-capacity-bounded-cells.md) | accepted |
| 0033 | [Require versioned migration and rollback contracts](../adr/0033-require-versioned-migration-and-rollback-contracts.md) | accepted |
| 0034 | [Reserve multidimensional Cell capacity before placement](../adr/0034-reserve-multidimensional-cell-capacity-before-placement.md) | accepted |
| 0035 | [Retain native Product UIs behind the Platform edge](../adr/0035-retain-native-product-uis-behind-the-platform-edge.md) | accepted |
| 0036 | [Separate Platform APIs from Product APIs](../adr/0036-separate-platform-apis-from-product-apis.md) | accepted |
| 0037 | [Verify complete Cell recovery, not backup presence](../adr/0037-verify-complete-cell-recovery-not-backup-presence.md) | accepted |
| 0038 | [Separate content-free observability from Audit and Usage](../adr/0038-separate-content-free-observability-from-audit-and-usage.md) | accepted |
| 0039 | [Run License Gates for every Product Version](../adr/0039-run-license-gates-for-every-product-version.md) | accepted |
| 0040 | [Use a managed private Registry before Harbor](../adr/0040-use-a-managed-private-registry-before-harbor.md) | accepted |
| 0041 | [Maintain a content-minimized immutable Audit stream](../adr/0041-maintain-a-content-minimized-immutable-audit-stream.md) | accepted |
| 0042 | [Reconcile Payment hourly and Usage daily](../adr/0042-reconcile-payment-hourly-and-usage-daily.md) | accepted |
| 0043 | [Use one mainland Region across multiple Availability Zones](../adr/0043-use-one-mainland-region-across-multiple-availability-zones.md) | accepted |
| 0044 | [Make P0 Production Gates non-waivable](../adr/0044-make-p0-production-gates-non-waivable.md) | accepted |
| 0045 | [Use Platform-funded models only through Higress](../adr/0045-use-platform-funded-models-only-through-higress.md) | accepted |
| 0046 | [Default-deny Product egress](../adr/0046-default-deny-product-egress.md) | accepted |
| 0047 | [Bind model routing to Data Processing Profiles](../adr/0047-bind-model-routing-to-data-processing-profiles.md) | accepted |
| 0048 | [Use consented JIT operator access](../adr/0048-use-consented-jit-operator-access.md) | accepted |
| 0049 | [Quarantine without deleting evidence or customer data](../adr/0049-quarantine-without-deleting-evidence-or-customer-data.md) | accepted |
| 0050 | [Require verifiable Tenant Export from Shared Products](../adr/0050-require-verifiable-tenant-export-from-shared-products.md) | accepted |
| 0051 | [Include Nacos in the Day-1 Platform](../adr/0051-include-nacos-in-the-day-1-platform.md) | accepted |
| 0052 | [Isolate component state in logical PostgreSQL databases](../adr/0052-isolate-component-state-in-logical-postgresql-databases.md) | accepted |
| 0053 | [Preserve Credit source Lots for consumption and refund](../adr/0053-preserve-credit-source-lots-for-consumption-and-refund.md) | accepted |
| 0054 | [Support electronic invoice requests in Stage 1](../adr/0054-support-electronic-invoice-requests-in-stage-1.md) | accepted |
| 0055 | [Use evidence triggers for deferred Platform components](../adr/0055-use-evidence-triggers-for-deferred-platform-components.md) | accepted |
| 0056 | [Use a Platform-owned notification gateway for outbound communication](../adr/0056-use-a-notification-gateway-for-outbound-communication.md) | accepted |
| 0057 | [Resolve the P0 Production Gate inventory](../adr/0057-resolve-the-p0-production-gate-inventory.md) | accepted (amends 0044) |

## Reading order

- ADR 0001-0014: Platform scope, tenancy, catalog, payment lifecycle, usage authority, and WeKnora upstream strategy.
- ADR 0015-0021: OpenMeter dependencies, canonical usage, financial integrity, pricing, and Provider-verified payment.
- ADR 0022-0038: OpenFGA, Product identity, Temporal, Secrets, Valkey, Platform Context, Cells, upgrades, recovery, APIs, and observability.
- ADR 0039-0055: License, supply chain, audit, reconciliation, availability, model and egress policy, operator access, Nacos, PostgreSQL isolation, Credit Lots, invoicing, and deferred-component triggers.

## Governance

ADRs record hard-to-reverse choices and their reasons. Implementation plans, task checklists, test evidence, version matrices, and operational runbooks must live in separate documents and reference the relevant ADR rather than rewriting it.
