---
status: accepted
---

# Separate Product User and Membership bindings

Within a Shared Product Instance, one Platform User maps to one external Product user through a Product User Binding, while every Platform Membership and Product Binding pair maps independently to an external membership and Product role. This lets one person participate in multiple Tenants without duplicate Product identities, email-based merging, or leakage of Product-internal roles back into Platform authorization.
