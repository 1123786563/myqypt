---
status: accepted
---

# Separate payment confirmation from fulfillment

External payment confirmation and delivery of purchased value are separate, durable states: a Payment Order progresses from `created` to `awaiting_payment`, `paid`, and then `fulfilled`. A failure after `paid` is retried idempotently without asking the Billing Customer to pay again or reverting the confirmed payment. Refunds reserve the eligible amount against concurrent consumption before calling the original Payment Provider, then write an immutable balance reversal only after Provider confirmation; failure retries or releases the reservation instead of directly editing a balance.
