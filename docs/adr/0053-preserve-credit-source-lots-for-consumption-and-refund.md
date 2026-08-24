---
status: accepted
---

# Preserve Credit source Lots for consumption and refund

Prepaid Usage Balance retains Credit Lots with source type and Payment Order, original and remaining CNY amount, expiry, and refundability, and uses an explicit earliest-expiry-first consumption rule. Unconsumed cash-funded Lots may be reserved and refunded through the original Payment Provider, while Included Allowance, promotional value, and consumed amounts are not refundable; refund concurrency locks specific Lots rather than relying on an aggregate balance.
