---
status: accepted
---

# Verify complete Cell recovery, not backup presence

A recoverable Cell includes its Product database, object manifest, vector snapshot, configuration, Product Version, Product Binding mappings, Secret references, and gateway and identity configuration. Destructive upgrades require a preceding Restore Rehearsal and production performs one complete rehearsal monthly with measured RPO and RTO; Stage 1 promises full-Cell disaster recovery and per-Tenant export, but not unproven single-Tenant restore from shared backups.
