---
status: accepted
---

# Use explicit one-to-one Billing Customer and Tenant ownership

In the initial domain model, each Tenant has exactly one Billing Customer and each Billing Customer owns exactly one Tenant, while a User may belong to multiple Tenants. This separates payment responsibility from human identity and avoids the ambiguous "approximately one-to-one" relationship that would otherwise leak into authorization, usage attribution, and billing reconciliation.
