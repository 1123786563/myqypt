---
status: accepted
---

# Use fixed-precision rating at occurrence time

Payment amounts use integer CNY fen and Meter quantities and intermediate pricing use fixed-precision decimals rather than floating point. Rating selects the Product Offer and Price version effective at `occurred_at`, records that version and its explicit ledger-rounding rule, automatically accepts events up to 24 hours late, and sends older or materially clock-skewed events to reconciliation instead of silently applying the current price.
