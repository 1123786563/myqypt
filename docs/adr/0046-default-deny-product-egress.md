---
status: accepted
---

# Default-deny Product egress

Product workloads have no unrestricted Internet access and reach declared connector and search targets through a controlled Egress Proxy that enforces domain and protocol policy, repeated DNS and address validation, private and metadata network denial, and request, response, and time limits. Tenant connector credentials are injected by Secret reference, preventing Product functionality from becoming an SSRF path or an unmetered route around approved Providers.
