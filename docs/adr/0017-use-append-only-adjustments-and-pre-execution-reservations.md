---
status: accepted
---

# Use append-only Usage Adjustments and pre-execution Reservations

Incorrect usage is corrected by a new immutable Usage Adjustment referencing the original event, never by updating events or aggregates. Variable-cost operations authorize a conservative maximum through a Usage Reservation before execution, settle against a trusted final Usage Event, and release the remainder; abandoned Reservations require recovery rather than remaining occupied indefinitely.
