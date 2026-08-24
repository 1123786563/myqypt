---
status: accepted
---

# Use Temporal for Stage 1 lifecycle orchestration

Temporal is a Day-1 dependency for durable Product Binding and commercial lifecycle orchestration instead of a later migration from a Platform-owned retry engine. Platform PostgreSQL remains the source of truth read by Portal and API, while Temporal owns execution history, retry, timers, signals, and compensation; Workflow history contains stable IDs and classified errors rather than secrets, prompts, documents, or raw payment payloads.

Stage 1 Workflows cover Product enable, suspend, resume, upgrade, Tenant erasure, payment and refund fulfillment, and subscription change. High-frequency requests, Usage Events, OpenFGA checks, model streaming, ordinary page actions, and Payment Provider signature validation remain outside Temporal.
