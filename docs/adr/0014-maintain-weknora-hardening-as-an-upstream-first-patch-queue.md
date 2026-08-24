---
status: accepted
---

# Maintain WeKnora hardening as an upstream-first Patch Queue

Every WeKnora Shared-tenancy hardening change will first be proposed upstream and retained locally as a minimal, replayable Patch Queue tied to `upstream_version`, `adapter_version`, and `patchset_version`. Compatibility testing covers both upstream-only and patched builds, and a formal Fork is declared only if upstream persistently rejects security boundaries that cannot be enforced by the Platform Gateway or Product Adapter.
