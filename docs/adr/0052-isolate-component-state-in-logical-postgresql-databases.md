---
status: accepted
---

# Isolate component state in logical PostgreSQL databases

Stage 1 uses one managed HA PostgreSQL service with independent databases, roles, migration owners, credentials, monitoring, and backup boundaries for Platform, Keycloak, OpenFGA, Temporal, OpenMeter, Nacos, and every WeKnora Cell. Cross-database joins and shared tables are forbidden, and Billing, Registry, or Product Cell state moves to a separate service when measured connection, storage, load, or failure-isolation thresholds require it.
