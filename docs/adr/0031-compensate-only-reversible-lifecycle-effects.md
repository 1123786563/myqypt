---
status: accepted
---

# Compensate only reversible lifecycle effects

Temporal may compensate safely reversible effects such as unused routes, unapplied Product Access, or empty external Tenants, but confirmed payments, occurred usage, irreversible migrations, and completed erasure remain immutable facts handled by forward actions such as refund, adjustment, backup restore, or human intervention. Activities use stable business idempotency keys and persist external identities immediately so replay cannot create duplicate Tenants, credits, or refunds.
