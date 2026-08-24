---
status: accepted
---

# Adopt OpenFGA in Stage 1

OpenFGA enters Stage 1 as the Platform authorization evaluator and Authorization Projection, while PostgreSQL remains the business source of truth for Membership, Platform Role, and Product Access lifecycles. A single outbox-driven projector writes tuples: grants remain pending until both stores confirm them, while revocations become immediately denying in PostgreSQL before tuple deletion; access requires both active business state and an allowing OpenFGA check.

OpenFGA covers Tenant membership and ownership, Platform Roles, Product Access, Product Instance administration, and subscription or billing visibility, but never Product-internal knowledge bases, sessions, files, or roles. Protected requests fail closed when OpenFGA is unavailable, allow decisions are not cached, and system consumers retry through durable queues. Authorization Models are immutable and versioned, tested through shadow comparison and replayable tuple migration before switching model IDs, with a defined rollback window.
