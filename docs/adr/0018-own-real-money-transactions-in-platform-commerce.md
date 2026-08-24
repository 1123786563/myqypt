---
status: accepted
---

# Own real-money transactions in Platform Commerce

The Platform Commerce context owns Payment Orders, Provider Transactions, Refund Orders, webhook ingestion, and an immutable Payment Journal for WeChat Pay and Alipay. Confirmed `paid` facts drive idempotent OpenMeter fulfillment, while OpenMeter owns subscriptions, entitlements, credit balances, pricing, and billing; separating real-money truth from purchased-value fulfillment preserves reconciliation and future Billing Provider replacement without creating two payment authorities.
