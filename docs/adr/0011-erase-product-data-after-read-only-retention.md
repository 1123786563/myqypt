---
status: accepted
---

# Erase Product data after Read-only Retention

A cancelled Subscription remains active through its paid term, followed by 30 days of Read-only Retention for inspection, export, or reactivation. After that period the Platform starts Tenant Erasure across Platform and Product databases, files, vectors, queued work, caches, credentials, OpenFGA tuples, bindings, and backup-expiration schedules, while legally required financial and audit records remain subject to a separate compliance retention policy. Completion requires an immutable, data-minimized Erasure Record with per-store Adapter evidence; a Temporal success marker is insufficient, and a Product that cannot provide this evidence cannot enter the Shared production Catalog.
