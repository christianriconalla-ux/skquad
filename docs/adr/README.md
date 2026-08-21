# Architecture Decision Records (ADRs)

This directory records skquad's key architectural decisions. Each ADR follows
the format: **Context → Decision → Consequences → Alternatives Considered**.

## Index

| ADR | Title | Status |
|-----|-------|--------|
| [0001](0001-agent-runtime.md) | Agent Runtime — Thin Custom Runtime + LiteLLM + Plugin Interface | Accepted |
| [0002](0002-llm-gateway.md) | Central LLM Gateway — LiteLLM Proxy | Accepted |
| [0003](0003-scale-to-zero.md) | Scale-to-Zero — Operator-Driven | Accepted |
| [0004](0004-message-bus.md) | Async Message Bus — Postgres-Backed Queue (v1) | Accepted |
| [0005](0005-persistence.md) | Data Persistence — Postgres + pgvector | Accepted |
| [0006](0006-deployment.md) | Deployment — Vanilla K8s, Custom Operator + Helm (No OLM) | Accepted |
| [0007](0007-agent-identity.md) | Agent Identity — Owner-Created, Platform-Facilitated | Accepted |
| [0008](0008-languages.md) | Language & Runtime Choices | Accepted |

## How to add an ADR

1. Copy the format from an existing ADR.
2. Number it sequentially (`0009-...`).
3. Set **Status** to `Proposed`, then `Accepted` / `Rejected` / `Superseded`.
4. If a decision changes, mark the old ADR `Superseded by [NNNN]` and add the
   new one.
