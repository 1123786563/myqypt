---
status: accepted
---

# Separate content-free observability from Audit and Usage

Restricted logs and traces may carry Tenant, Product Binding, request, and trace identities but never prompts, document bodies, secrets, or raw payment payloads; Prometheus metrics exclude high-cardinality Tenant and User labels, while per-Tenant usage remains in the Canonical Usage and ClickHouse paths. Security and business Audit is an independent immutable stream rather than a retention mode for mutable debugging logs.
