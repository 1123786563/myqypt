---
status: accepted
---

# Forbid cross-Tenant Product Domain Object sharing

The initial Platform forbids access to a Product Domain Object across the hard Tenant boundary, even when an upstream Product exposes organization or workspace sharing. A collaborator must become a member of the owning Tenant and act under that Tenant Context, preventing product-specific sharing semantics from silently weakening the Platform's security, billing, and erasure boundaries.
