---
status: accepted
---

# Use managed Secrets before self-hosting OpenBao

Development uses uncommitted local secret files, the controlled beta uses orchestrator-provided secrets with encrypted backup and restricted host access, and paid production uses cloud KMS with a managed Secret service while Platform PostgreSQL stores only `secret_ref`. Self-hosted OpenBao is deferred because operating the root of trust would add unseal, HA, backup, audit, and rotation obligations before they differentiate the product.
