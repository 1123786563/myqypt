---
status: accepted
---

# Model Shared Product Instances separately from Tenant bindings

A Product Version immutably binds upstream, Adapter, Patch Set, schema, and image identity; a Product Instance is a deployed runtime unit that can serve multiple Tenants; and a Product Binding maps one Tenant to that instance and its server-controlled external Tenant identity. Tenant, namespace, and Product Instance are therefore not synonyms, allowing Shared operation without weakening the Platform Tenant boundary.
