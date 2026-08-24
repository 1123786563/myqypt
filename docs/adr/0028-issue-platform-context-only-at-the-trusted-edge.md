---
status: accepted
---

# Issue Platform Context only at the trusted edge

Clients may select a Tenant only at the Platform edge, where active Membership and OpenFGA authorization are verified and all client-supplied internal identity headers are stripped. The Gateway then issues a short-lived, audience-bound Platform Context containing stable User, Tenant, Product, Product Instance, and Product Binding identities; Products are not directly public and never trust a forwarded client Tenant ID or a long-lived role claim.
