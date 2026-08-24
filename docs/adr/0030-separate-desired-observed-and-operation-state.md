---
status: accepted
---

# Separate Desired State, Observed State, and Lifecycle Operation

A Product Binding records the requested `active`, `suspended`, or `erased` Desired State separately from its latest confirmed absent, provisioning, active, degraded, suspended, erasing, or erased Observed State. Every attempt to converge them is a separately identified Lifecycle Operation with queued, running, compensating, succeeded, failed, or needs-attention progress, preventing one generic failure value from destroying the last known business and runtime truth.
