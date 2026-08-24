---
status: accepted
---

# Own a canonical Usage stream outside OpenMeter

Approved Usage Authorities publish validated Canonical Usage Events to a Platform-owned Kafka topic, whose independent consumers write an immutable archive and forward events through the OpenMeter Adapter into OpenMeter's own ingestion pipeline. The streams may share a Kafka cluster but not topic ownership or retention contracts, preserving replay and billing-engine replacement without coupling the Platform to OpenMeter internal topics or ClickHouse tables.
