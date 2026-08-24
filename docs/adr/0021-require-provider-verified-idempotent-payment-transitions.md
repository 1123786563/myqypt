---
status: accepted
---

# Require Provider-verified idempotent payment transitions

A WeChat Pay or Alipay callback can confirm `paid` only after Provider-specific signature and certificate validation, merchant and application matching, Platform order matching, globally unique Provider Transaction identity, exact CNY amount verification, successful Provider status, and a legal current-state transition. Webhook delivery, active Provider query, and reconciliation converge on the same idempotent transition, while any identity or amount conflict freezes fulfillment for review.
