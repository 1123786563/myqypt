---
status: accepted
---

# Reconcile Payment hourly and Usage daily

Hourly Payment Reconciliation compares WeChat Pay and Alipay transactions with the Platform Payment Journal and OpenMeter fulfillment and credit, while daily Usage Reconciliation compares the Platform Kafka and immutable archive with OpenMeter ingestion, ClickHouse rating, and credit or billing ledgers. Every discrepancy becomes a Reconciliation Case; unexplained amount, duplicate fulfillment, or negative-balance conflicts freeze affected money movement and are corrected only through immutable adjustment, refund, or fulfillment events.
