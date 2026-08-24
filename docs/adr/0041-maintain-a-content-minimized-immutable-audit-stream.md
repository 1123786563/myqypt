---
status: accepted
---

# Maintain a content-minimized immutable Audit stream

The Platform retains immutable Audit Events for Membership, roles, Product Access, payments, refunds, manual adjustments, Product lifecycle, Secret reference and rotation, OpenFGA model and tuple changes, cross-Tenant administration, exports, and erasure. Audit includes stable authority, Tenant, action, resource, decision, state-hash, request, time, result, and approval references without secrets, prompts, documents, or raw payment payloads; Stage 1 retains it for 12 months while legally required financial records follow a separate policy.
