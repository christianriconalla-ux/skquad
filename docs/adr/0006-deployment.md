# ADR-0006: Deployment — Vanilla K8s, Custom Operator + Helm (No OLM)

- **Status:** Accepted
- **Date:** 2026-08-22
- **Deciders:** Ross, Sherlock

## Context

skquad must run on **Kubernetes** and be deployed via an **operator** (NFR-2).
Ross specified **vanilla Kubernetes — no OLM** (Operator Lifecycle Manager).
OLM is OpenShift's operator packaging/distribution framework; excluding it
means we target portable, vanilla K8s clusters.

The operator must manage:

- **Squad lifecycle** — create/delete the squad **namespace** + base resources.
- **Agent lifecycle** — create/delete the agent **Deployment** (0↔1 replicas).
- **Scale-to-zero** — see ADR-0003.
- **Isolation** — network policies, resource quotas per squad namespace.

## Decision

- Target **vanilla Kubernetes** (no OpenShift/OLM dependency).
- Implement a **custom operator** in **Go** using **controller-runtime /
  kubebuilder**, defining CRDs:
  - `Squad` — reconciles the squad namespace + base resources.
  - `Agent` — reconciles the agent Deployment, secrets, and replica count
    (scale-to-zero).
- Package and deploy with **Helm** (one chart for the control plane; the
  operator is installed as part of it). Squad/agent resources are created via
  the API, not by hand.
- No OLM/CSV packaging. Distribution is via Helm chart + (optionally) a
  container image for the operator.

## Consequences

- **(+)** Portable across any vanilla K8s cluster (no OpenShift lock-in).
- **(+)** Standard, well-understood tooling (controller-runtime, Helm).
- **(+)** Single operator owns squad/agent lifecycle + scaling → coherent.
- **(+)** CRDs give a clean, declarative model for squads and agents.
- **(−)** We own the operator's correctness (reconciliation, error handling).
- **(−)** No OLM means no built-in operator marketplace/distribution; we
  distribute via Helm.
- **Mitigation:** follow kubebuilder conventions; add operator tests (envtest);
  document installation clearly.

## Alternatives Considered

- **OLM (OpenShift)** — provides operator packaging + distribution, but ties us
  to OpenShift and adds complexity. **Rejected** per Ross's requirement
  (vanilla K8s, no OLM).
- **Kustomize only (no operator)** — simpler, but no lifecycle management, no
  scale-to-zero, no reconciliation. **Rejected** (we need an operator).
- **KEDA for scaling** — see ADR-0003; rejected for v1 in favour of
  operator-driven scaling.
