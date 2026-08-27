# skquad

> **An open-source, cloud-native Agentic AI platform.**
> Build, run, and govern **squads of AI agents** on Kubernetes — in minutes.

skquad gives teams a low-barrier way to create **squads of AI agents** that
collaborate on tasks via a **Kanban board**. It is **Bring-Your-Own-Model
(BYOM)**, **enterprise-secure** (OIDC, two-layer RBAC, audit), and
**multi-tenant** (each squad isolated in its own Kubernetes namespace).

---

## Why skquad?

- **Minutes to first agent** — a new user can go from login to a running squad
  with a single agent and a working LLM provider in a few clicks.
- **BYOM** — agents can use any LLM in the platform's provider registry.
- **Enterprise-grade security** — OIDC identity, two-layer RBAC, full audit.
- **Multi-user, multi-squad** — many users, each owning isolated squads.
- **Extensible via plugins** — the core stays small; capabilities plug in.

> skquad deliberately avoids the complexity of existing agentic platforms. The
> initial surface is small and easy to start; the architecture is designed for
> extension, not feature-maximalism.

---

## Core Concepts

| Concept | Description |
|---------|-------------|
| **Squad** | A team of agents with a mission + operating model. Isolated in its own K8s namespace. Owned by one user. |
| **Agent** | A member of a squad. Runs in its own pod. Has its own identity, credentials, and permissions. |
| **Kanban Board** | One per squad — the primary work surface. Tasks are the unit of work. |
| **Task** | A unit of work assigned to an agent. An agent's context resets before each new task. |
| **Resource Registry** | The catalog agents can use: LLM providers, skills, tools, APIs, knowledge bases, project workspaces. |
| **Access Grant** | An owner granting another user/agent the right to talk to a squad's agents. |

---

## Architecture (two planes)

- **Control plane** (`skquad-system` namespace) — API server, web app, resource
  registry, identity/RBAC, **LLM gateway** (metering + BYOM), message queue, and
  the **operator**. Always-on; the single enforcement point for security and
  metering.
- **Data plane** (one namespace per squad) — the **agent pods**, each running a
  thin agent runtime. Agents **scale to zero** when idle.

```
Web App → API Server (OIDC, RBAC) → Postgres
                │  enqueues Kubernetes outbox intents
                ▼
             Operator ──► squad namespaces (agent pods, scale-to-zero)
Agent pods ──► LLM Gateway (LiteLLM proxy: metering, cost, BYOM, perms)
```

See [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) for the full design.

Current implementation status: the control-plane API, operator, Helm chart, and
Python agent runtime have working vertical slices for squad/agent/task
lifecycle, generated runtime credentials, task wake-up/scale-down, messaging
inbox APIs, runtime inbox draining, granted resource discovery, dynamic runtime
plugins, task-scoped context with bounded recent memory, operator deletion
finalizers, a durable Kubernetes outbox for Squad/Agent CR convergence, and
LiteLLM gateway bootstrap/key provisioning. The web app is still an early
placeholder, and semantic vector memory search plus gateway metering callbacks
remain planned follow-ups.

---

## Repository Layout

```
skquad/
├── README.md
├── docs/                     # design & architecture documentation
│   ├── REQUIREMENTS.md
│   ├── ARCHITECTURE.md
│   ├── domain-model.md
│   ├── agent-runtime.md
│   ├── llm-gateway.md
│   ├── identity-security.md
│   ├── resource-registry.md
│   ├── kanban-task-lifecycle.md
│   ├── collaboration-messaging.md
│   ├── deployment-operator.md
│   ├── observability-metering.md
│   ├── data-model.md
│   ├── api-design.md
│   ├── web-app-ux.md
│   ├── plugin-architecture.md
│   ├── security-threat-model.md
│   └── adr/                  # architecture decision records
├── control-plane/            # API server (Go)
├── operator/                 # K8s operator (Go, controller-runtime)
├── agent-runtime/            # agent harness (Python, LiteLLM + plugins)
├── llm-gateway/              # LLM gateway (LiteLLM proxy, Python)
├── web/                      # web app (React / Next.js, TypeScript)
├── charts/skquad/            # Helm chart
└── .github/workflows/        # CI/CD
```

---

## Documentation

| Document | Purpose |
|----------|---------|
| [Requirements](docs/REQUIREMENTS.md) | Refined v1 requirements (FR/NFR). |
| [Architecture](docs/ARCHITECTURE.md) | High-level two-plane architecture. |
| [Domain Model](docs/domain-model.md) | Entities, relationships, lifecycle. |
| [Agent Runtime](docs/agent-runtime.md) | The agent harness design. |
| [LLM Gateway](docs/llm-gateway.md) | Metering, cost, BYOM, permissions. |
| [Identity & Security](docs/identity-security.md) | OIDC, RBAC, agent identity, audit. |
| [Resource Registry](docs/resource-registry.md) | The resource catalog. |
| [Kanban & Tasks](docs/kanban-task-lifecycle.md) | Board + task lifecycle. |
| [Collaboration & Messaging](docs/collaboration-messaging.md) | Async agent↔agent. |
| [Deployment & Operator](docs/deployment-operator.md) | K8s, CRDs, scale-to-zero, Helm. |
| [Observability & Metering](docs/observability-metering.md) | Prometheus + token/cost. |
| [Data Model](docs/data-model.md) | Postgres schema. |
| [API Design](docs/api-design.md) | Control-plane REST API. |
| [Web App & UX](docs/web-app-ux.md) | SPA flows + screens. |
| [Plugin Architecture](docs/plugin-architecture.md) | Extensibility via plugins. |
| [Security & Threat Model](docs/security-threat-model.md) | Threats + mitigations. |
| [CI/CD](docs/ci-cd.md) | Validation workflow and delivery follow-ups. |
| [ADRs](docs/adr/) | Key architectural decisions. |

---

## Status

- **Phase 1 — Requirements & Foundation:** ✅ complete.
- **Phase 2 — Architecture Design:** ✅ complete (this repo's `docs/`).
- **Phase 3 — Delivery:** 🚧 in progress (see the [Kanban board](#project-board)).

---

## Project Board

Tasks are tracked on the **skquad** board in Kanbunny.

---

## License

TBD (open-source — license to be chosen).
