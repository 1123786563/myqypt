---
status: accepted
---

# Use Platform-funded models only through Higress

Stage 1 offers only Platform-funded and Platform-managed model access, bills Tenants through Product Offers and canonical Usage, and keeps Provider keys in the managed Secret service. Every Product model call must traverse Higress under enforced network policy so that authorization, reservation, Token facts, routing, limits, audit, and permitted fallback cannot be bypassed; Tenant BYOK is deferred.
