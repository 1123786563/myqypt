---
status: accepted
---

# Separate Platform Users from Casdoor identities

Casdoor owns login identity, credentials, MFA, federation, and a stable subject, while the Platform owns User, Membership, Product Access, and audit relationships through an `identity_provider + subject` Identity Binding. Mutable email, phone, and username values are never cross-system keys, and disabling or deleting a Casdoor identity cannot erase Platform history or Product data by cascade. Casdoor is the Stage 1 identity provider because it is a Go-native IAM that keeps the operator stack on one runtime, replacing the JVM-based Keycloak with lower resource and operational overhead.
