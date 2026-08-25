# External Confirmations Register

> Date: 2026-08-24  
> Purpose: facts and approvals that architecture decisions cannot manufacture internally.

## Required before closed beta

| Area | Confirmation | Evidence owner | Blocking result |
| --- | --- | --- | --- |
| WeKnora | Shared-tenancy hardening plan covers database scope, vectors, task fairness, quota reservation, erasure, RBAC and privileged access | Platform Security + WeKnora owners | No uncontrolled Shared beta |
| Nacos | 3.2.3 cluster, Java 17, external PostgreSQL, auth, backup/restore and internal-only networking operate in the selected environment | Platform/SRE | AI Registry features remain side-path PoCs; Lighthouse stays independent of them |
| Valkey | OpenMeter command, persistence, TLS, multi-replica dedupe, failover and recovery compatibility | Billing Platform | Use approved Redis fallback if not passed by week 8 |
| Model Providers | Processing region, content retention, training use, subprocessors and commercial API rights | Product + Legal/Privacy | Provider excluded from Data Processing Profile |
| Payments | Merchant onboarding, sandbox, signatures, certificates, callback acknowledgement, active query, refund and reconciliation behavior for WeChat Pay and Alipay | Commerce + Finance | Provider excluded from paid flow |
| Cloud | Mainland-China availability of managed PostgreSQL, Kafka, ClickHouse, Secret/KMS, Registry, Object Storage and multi-AZ networking | Platform/SRE | Rework Day-1 topology and cost envelope |

## Required before paid launch

| Area | Confirmation | Evidence owner | Blocking result |
| --- | --- | --- | --- |
| Privacy | Mainland-China privacy notice, consent, deletion, data export, minor protection, operator access and incident notification | Legal/Privacy | Paid launch blocked |
| Tax | Electronic invoice type, tax rate, issuance timing, red-letter reversal, voiding and statutory retention | Finance/Tax counsel | Business invoice feature blocked |
| License | ProductVersion-specific License Report covering source, assets, plugins, models, datasets, dependencies and images | Legal + Supply Chain | ProductVersion blocked |
| WeKnora | Real cross-Tenant attack matrix, Shared Cell load/noisy-neighbor test, Tenant Export, Erasure evidence and Restore Rehearsal | Security + QA + Product team | Paid Shared launch blocked |
| OpenMeter | Real Kafka/ClickHouse/Valkey topology, Usage replay, reservations, credits, billing, workers and scheduled jobs | Billing Platform | Usage billing blocked |
| Payment chain | Provider sandbox and production-like test proves `created → awaiting_payment → paid → fulfilled`, duplicate callback, recovery, partial refund and reconciliation | Commerce + Finance + QA | Paid launch blocked |
| Disaster recovery | Measured Platform metadata RPO ≤15m, Product RPO ≤1h and RTO ≤4h in isolated restore | SRE + Product owners | Paid launch blocked |
| Authorization | PostgreSQL/OpenFGA grant and revoke consistency, fail-closed behavior, model migration and rollback | Security + Platform | Paid launch blocked |
| Notification channel | Outbound SMS/email providers: mainland SMS compliance (real-name, template review), email deliverability, fraud-control boundaries per recipient identity. Design accepted via ADR-0056 (ruling on #131, 2026-08-25); provider facts still gate production enablement (T86.11 dossier) | Platform + Legal/Privacy | Verification-code and notification features blocked |
| Regulatory prerequisites | ICP filing, public-security filing, MLPS (等级保护), GenAI service filing and AI-content labeling obligations for a public paid SaaS in mainland China | Platform/SRE + Legal/Privacy | Paid launch blocked |

## Nacos fact snapshot

As of 2026-08-24, the reviewed production baseline is Nacos 3.2.3 GA on Java 17, at least three Server nodes behind an internal load balancer, an external supported database, explicit auth, and a management-only Console. Nacos 3.3.0-BETA is excluded.

MCP, Agent, Skill and Prompt Registry capabilities remain separate side-path PoCs. Their presence in Nacos does not prove client compatibility, Platform authorization, visibility isolation, safe distribution, rollback or runtime behavior.

Primary references:

- [Nacos Quick Start](https://nacos.io/en/docs/latest/quickstart/quick-start/)
- [Nacos 3.2.3 Release](https://github.com/alibaba/nacos/releases/tag/3.2.3)
- [Deployment Best Practices](https://nacos.io/docs/latest/manual/admin/deployment/deployment-best-practices/)
- [Cluster Deployment](https://nacos.io/en/docs/latest/manual/admin/deployment/deployment-cluster/)
- [Authentication](https://nacos.io/en/docs/latest/manual/admin/auth/)
- [AI Registry Overview](https://nacos.io/en/docs/latest/manual/user/ai/ai-registry-overview/)

## Evidence rule

Configuration success, a healthy endpoint, static code inspection, unit tests, focused component tests, Workflow completion and a one-off smoke test are partial evidence. None alone satisfies a paid-launch Production Gate.
