---
status: accepted
---

# Run Kafka and ClickHouse as OpenMeter core dependencies

Stage 1 includes PostgreSQL, Kafka, ClickHouse, and persistent Redis-compatible deduplication for OpenMeter. Its minimum runtime consists of the API, sink worker, balance worker, billing worker, and scheduled subscription sync, invoice collection, invoice advancement, and charge advancement; the Platform deployment supplies the missing `advance-charges` schedule when prepaid credits require it. Svix and the independent notification service remain disabled because payment webhook ingestion belongs to Platform Commerce. Kafka and ClickHouse are core because the selected OpenMeter production units use them for ingestion, asynchronous domain flows, usage storage and aggregation, metered entitlements, balance, and usage-based billing; disabling bundled charts means supplying external services, not disabling these dependencies.
