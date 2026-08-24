---
status: accepted
---

# Separate Platform Users from Keycloak identities

Keycloak owns login identity, credentials, MFA, federation, and a stable subject, while the Platform owns User, Membership, Product Access, and audit relationships through an `identity_provider + subject` Identity Binding. Mutable email, phone, and username values are never cross-system keys, and disabling or deleting a Keycloak identity cannot erase Platform history or Product data by cascade.
