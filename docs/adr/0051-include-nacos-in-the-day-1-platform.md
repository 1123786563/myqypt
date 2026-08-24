---
status: accepted
---

# Include Nacos in the Day-1 Platform

Nacos enters Day 1 as an internal runtime registry and distribution foundation behind an AI Registry Provider, while Platform PostgreSQL remains the Product-level AI Asset metadata source of truth. The verified 2026-08-24 baseline is Nacos 3.2.3 GA on Java 17 with at least three Server nodes behind an internal load balancer, external PostgreSQL, managed shared authentication secrets, and a management-only Console; 3.3.0-BETA is excluded.

MCP, Agent, Skill, and Prompt Registry capabilities run as side-path PoCs and become production dependencies only after their individual version, authorization, visibility, client, cache, rollback, and failure-mode gates pass. None may block the WeKnora Lighthouse purchase-to-billing journey merely because Nacos itself is present on Day 1.
