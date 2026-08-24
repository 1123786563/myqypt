---
status: accepted
---

# Use a managed private Registry before Harbor

Stage 1 builds pinned source in isolation, produces License and vulnerability results and an SBOM, signs the image with Cosign, stores it in a managed private Registry, verifies admission, and deploys only by digest. Registry capability remains behind a Provider boundary and self-hosted Harbor is deferred until replication, isolation, scale, or compliance evidence requires it; public Registry images never enter production directly.
