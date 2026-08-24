---
status: accepted
---

# Run Shared Products in capacity-bounded Cells

Every Shared Product Instance is a Cell with explicit Tenant, data, and concurrency capacity instead of one global instance. Upgrades progress from internal to canary and then batched production Cells, while each Product Binding remains fixed to one Cell unless an explicit migration moves it, limiting the blast radius of software defects, cross-tenant mistakes, resource contention, and failed upgrades.
