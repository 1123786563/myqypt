---
status: proposed
date: 2026-08-25
amends: 0044
---

# Resolve the P0 Production Gate inventory

ADR-0044 and baseline v1.1 section 25 diverge on the non-waivable paid-launch gate list: the baseline adds Tenant Export, Valkey compatibility (or an approved Redis fallback), and a Nacos production PoC that ADR-0044 omits, and the baseline's Nacos entry conflicts with ADR-0051, which forbids Nacos from blocking the WeKnora Lighthouse purchase-to-billing journey. This ADR resolves the inventory into a single authoritative list of thirteen non-waivable gates: Tenant Isolation, Cross-Tenant Security, License, Payment Reconciliation, Usage Replay, Usage Reconciliation, Backup/Restore, Tenant Export, Tenant Erasure, Secret Rotation, Image Signature, Product Adapter Compatibility, and a Verified Deduplication Backend (Valkey compatibility, or the approved Redis fallback of ADR-0027).

Tenant Export joins the non-waivable set because ADR-0050 and baseline section 17 already make verifiable export a condition for entering the Shared production catalog and P0 risk 9 treats incomplete Export and Erasure as one failure mode; its absence from ADR-0044 was an enumeration gap, not a decision. The deduplication backend becomes a named gate because duplicate usage events feed billing directly (P0 risk 3), while remaining satisfiable by either backend so it never forces Valkey adoption. The Nacos production PoC is reclassified out of the paid-launch set: per ADR-0051 and the external-confirmation register, Nacos verification gates only the promotion of AI Registry capabilities from side-path PoCs to production dependencies, and its failure degrades those features without blocking launch.

Consequences: T88 aggregates exactly the thirteen gates above; the side-path Nacos gate runs before any AI Registry feature becomes a production dependency; ADR-0044's non-waivable semantics are unchanged and strengthened only in enumeration; baseline section 25 carries the reconciled list with an annotation. This ADR is proposed pending acceptance by the architecture owner with a security review of the two added gates.
