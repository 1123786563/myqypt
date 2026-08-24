---
status: accepted
---

# Fix Canonical Usage identity and privacy boundaries

Every Canonical Usage Event carries a globally unique event and schema version, Tenant, Product, Product Instance, Usage Subject, Meter, fixed-precision quantity and unit, occurrence and ingestion times, Usage Authority, request and trace references, and optional actor, resource, correction, and allow-listed metadata. Organization and Billing Customer IDs are resolved from versioned Tenant mappings rather than duplicated, the actor User never becomes the billing boundary, and metadata cannot contain secrets, prompts, document contents, or personal sensitive information.
