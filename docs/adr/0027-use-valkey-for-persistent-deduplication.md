---
status: accepted
---

# Use Valkey for persistent deduplication

Stage 1 selects Valkey as the Redis-compatible persistent deduplication backend for OpenMeter and other bounded Platform needs. Because the current OpenMeter repository configures and documents Redis rather than certifying Valkey, production readiness requires explicit command, persistence restart, multi-replica deduplication, TLS, failover, and recovery compatibility evidence by week 8. Passing admits Valkey to production; failure makes the paid launch use OpenMeter's verified Redis path and defers Valkey rather than blocking the 24-week release or triggering an unreviewed last-minute replacement.
