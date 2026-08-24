---
status: accepted
---

# Reserve multidimensional Cell capacity before placement

Product Binding placement reserves Tenant count, storage, vectors, background-job concurrency, model-request concurrency, ingestion rate, and database size rather than relying on Tenant count alone. Stage 1 stops new placement when capacity is unavailable and moves existing bindings only through an explicit export, verification, switch, and rollback-aware Cell Migration Workflow instead of automatic live rebalancing.
