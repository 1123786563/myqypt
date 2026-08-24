---
status: accepted
---

# Require WeKnora Shared-tenancy hardening before paid launch

WeKnora will remain the Lighthouse Product, but its current application-level tenancy is insufficient for a public Shared production instance because database access, vector operations, background work, quota reservation, tenant erasure, and privileged administration lack complete defense in depth. A controlled beta may temporarily isolate no more than ten Tenants in separate instances, while paid launch requires Shared-mode hardening and cross-tenant security acceptance; failure to pass by week 12 delays launch or triggers a Lighthouse Product change rather than a security waiver.
