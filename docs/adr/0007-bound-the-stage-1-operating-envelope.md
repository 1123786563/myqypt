---
status: accepted
---

# Bound the Stage 1 operating envelope

Stage 1 targets a 16-week closed beta and 24-week paid launch in one region with one Lighthouse Product, 100 paid Tenants, 1,000 monthly active Users, 100 concurrent AI requests, and 50 control-plane requests per second. It targets 99.9% monthly control-plane and gateway availability, platform-metadata and billing-fact RPO of 15 minutes, Product-data RPO of one hour, and overall RTO of four hours, while keeping fixed infrastructure below CNY 30,000 per month and excluding private deployment, dedicated clusters, multiple formal Product-isolation modes, and automatic Day-1 adoption of OpenChoreo. Docker Compose is limited to development, CI, integration, and the controlled beta; paid production runs OCI images on a single-region multi-node Kubernetes environment with managed stateful data and secret services where available. Temporal and Nacos are explicit Day-1 inclusions with bounded responsibilities.
