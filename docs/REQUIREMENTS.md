# skquad — Requirements

> **Status:** Refined (v1) · **Owner:** Ross · **Last updated:** 2026-08-22
>
> skquad is an **open-source, cloud-native Agentic AI platform**. The goal is a
> simple, easy-to-use platform where a user can create AI agents **in minutes**.
>
> This remains the product target. The current implemented slice and deferred
> requirements are reconciled in
> [`implementation-status.md`](implementation-status.md).

---

## 1. Vision

skquad gives teams a low-barrier way to build, run, and govern **squads of AI
agents** on Kubernetes. It deliberately avoids the complexity of existing
agentic platforms (e.g. Paperclip.ai) and instead optimises for:

- **Time-to-first-agent in minutes** — a new user can go from login to a running
  squad with a single agent and a working LLM provider in a few clicks.
- **Bring Your Own Model (BYOM)** — agents can use any LLM available in the
  platform's LLM provider registry.
- **Enterprise-grade security** — strong identity (OIDC), two-layer RBAC, and
  full audit logging.
- **Multi-user, multi-squad** — many users, each owning isolated squads.
- **Extensibility** — the core stays small; capabilities are added through
  **plugins** (skills, tools, LLM providers, resource connectors).

> **Design constraint:** do **not** copy Paperclip.ai's complexity. Keep the
> initial surface small and easy to start; design for extension, not for
> feature-maximalism.

---

## 2. Guiding Principles

| # | Principle | Meaning |
|---|-----------|---------|
| P1 | **Simplicity first** | The minimal path to a working agent must be trivial. Everything else is opt-in. |
| P2 | **BYOM** | No lock-in to a single model vendor. Any provider in the registry works. |
| P3 | **Security by default** | Identity, RBAC, and audit are not add-ons; they are load-bearing. |
| P4 | **Multi-tenant isolation** | Each squad is isolated (own K8s namespace); agents run in their own pods. |
| P5 | **Extensible via plugins** | New capabilities plug in; the core does not balloon. |
| P6 | **Boring technology** | Prefer well-understood, portable building blocks (Postgres, vanilla K8s, standard operator). |

---

## 3. Core Concepts (Domain Model)

The platform is built around a small set of first-class entities. (Full
relationships and lifecycle in [`domain-model.md`](domain-model.md).)

- **User** — a human, authenticated via OIDC.
- **Squad** — owned by one User. Has a *mission* and an *operating model*.
  Isolated in its own Kubernetes namespace.
- **Agent** — a member of a squad. Runs in its own pod. Has its own identity,
  credentials, and a permission set. Consumes LLMs and resources from the
  registry.
- **Kanban Board** — one per squad. The **primary work surface**. Tasks are the
  unit of work.
- **Task** — a unit of work on a board, assigned to an agent.
- **Operating Model** — the roles of each agent in the squad + how they
  collaborate.
- **Resource Registry** — the catalog of things agents can use: LLM providers,
  skills, tools, APIs, knowledge bases (vector DBs), project workspaces
  (git / Jira / Confluence).
- **Access Grant** — an owner granting another user (or another squad's agents)
  the right to talk to the squad's agents.

Two planes:

- **Control plane** — API server, web app, resource registry, identity/RBAC,
  LLM gateway, operator. Always-on, in a dedicated namespace.
- **Data plane** — the agent pods, living in squad namespaces.

---

## 4. Functional Requirements

### 4.1 Onboarding — the "minutes" flow (FR-1)

A brand-new user, after logging in, can:

1. Create a **squad**.
2. Add a **single agent** to the squad.
3. Configure the squad to use a **predefined LLM provider**.
4. **Done** — the agent is running and can pick up tasks.

This is the north-star UX. Every other capability is an extension of this path.

### 4.2 Squads (FR-2)

- A user can create **squads** of **one or more agents**.
- Each squad has a **mission** (what it is for) and an **operating model**
  (the roles of each agent + how they collaborate).
- A squad is **owned by exactly one user**.
- The owner can **grant other users access** to talk to the squad's agents.
- Each squad is **isolated in its own Kubernetes namespace**.

### 4.3 Agents (FR-3)

- Each agent has its **own identity and credentials**, and a **set of
  permissions**.
- The **squad owner creates the agent's identity** and configures the agent to
  use its credential.
- Each agent **runs in its own pod**.
- An agent's **working context is task-scoped**: its context / chat history is
  **reset just before starting a new task** from the Kanban board.
- An agent has **long-term memory** that persists across tasks (Postgres +
  pgvector).
- Agents can be **configured to talk to agents in other squads** (task handoff,
  questions) — subject to access grants.

### 4.4 Kanban Board (FR-4)

- Each squad has a **Kanban board**.
- The board is the **primary way a user interacts with the squad** (create /
  assign tasks, watch progress).
- **Tasks** are the unit of work. An agent **picks up tasks assigned to it**
  from the board.
- A user may **also talk to an agent directly** through a chat interface
  (secondary interaction).
- The agent's context / chat history **resets before each new task** from the
  board.

### 4.5 Collaboration (FR-5)

- **Within a squad:** agents pull tasks from the shared Kanban board. Agents
  can **message each other** to **delegate tasks** or **consult** (e.g. code
  review).
- **Across squads:** the **human user defines which agents may talk to other
  squads**. An agent from one squad may **add a task to another squad's board**
  and **ping an agent** in the other squad to pick it up.
- **All agent-to-agent communication is asynchronous** (queued), so a busy
  agent is **not disturbed** (to protect its task context).

### 4.6 Resource Registry (FR-6)

The platform maintains a **registry of resources** that agents can use:

- **LLM providers**
- **Skills**
- **Tools**
- **APIs**
- **Knowledge bases** (vector databases)
- **Project workspaces** (git repos, Jira, Confluence)

- The **platform administrator registers** resources (e.g. a vector database).
- The **squad owner grants agents access** to specific registered resources.

### 4.7 Bring Your Own Model (FR-7)

- Agents can use **any LLM model** made available in the platform's **LLM
  provider registry**.
- Users can **bring their own** LLM provider + API key.
- **Predefined LLM providers** (registered by the platform admin) power the
  fast onboarding path.

### 4.8 Metering (FR-8)

- The platform **meters token usage** per **agent** and per **squad**.
- If a **price per token** is configured on a provider, metering shows the
  **monetary cost**.
- Metering is captured centrally (via the **LLM gateway**).

### 4.9 Observability (FR-9)

- An **observability feature** can be **enabled** to emit **Prometheus
  metrics**.

---

## 5. Non-Functional Requirements

### 5.1 Security (NFR-1)

- **OIDC** for human users.
- **Two-layer RBAC:**
  - **User RBAC** — managed by the **platform administrator** (roles such as
    platform admin, squad owner, squad member).
  - **Agent permissions** — defined by the **squad owner** (which registry
    resources an agent may access).
- **Audit logging** — a who-did-what trail for both user and agent actions.
- **Multi-tenant isolation** — squads isolated by namespace; agents in their
  own pods.

### 5.2 Deployment & Operations (NFR-2)

- Runs on **vanilla Kubernetes** — **no OLM**.
- Deployed via a **custom operator** (e.g. controller-runtime / kubebuilder) +
  **Helm**.
- **Squad = namespace**, **agent = pod**.
- **Agent pods scale to zero** when idle.

### 5.3 Data & Persistence (NFR-3)

- **Postgres** for data persistence.
- **Postgres + pgvector** for agent long-term memory.
- **Audit log** stored in Postgres.

### 5.4 Extensibility (NFR-4)

- The architecture must be **extensible through plugins** (skills, tools, LLM
  providers, resource connectors) without modifying the core.

### 5.5 Performance & Scale (NFR-5)

- v1 targets a maximum supported deployment size of about **100 squads** with
  about **7 agents per squad** (roughly 700 agents total).
- Control-plane HTTP API calls must complete in **less than 1 second** under
  the supported v1 scale target, excluding external LLM/provider latency.
- Postgres-backed task, message, audit, memory, and metering paths must be
  designed and tested against this v1 scale before introducing a dedicated
  message bus.
- Higher-volume task throughput and metering-rate targets remain benchmark
  items, but v1 architecture decisions should assume the scale envelope above.

---

## 6. Out of Scope for v1

- Fine-grained per-token budgeting/enforcement beyond accurate metering.
- Agent-to-agent SLA guarantees.
- Multi-cluster federation.
- Hosted multi-tenant control planes.
- On-prem single-node/non-Kubernetes mode.

v1 is a **single-tenant, Helm-installed Kubernetes platform** managed by the
Skquad operator. The internal squad/agent isolation model remains load-bearing,
but the first release is not a shared hosted service.

---

## 7. Open Decisions & Tensions

These are resolved in the ADRs ([`adr/`](adr/)) and the architecture
([`ARCHITECTURE.md`](ARCHITECTURE.md)).

| # | Decision / Tension | Working recommendation |
|---|--------------------|------------------------|
| D1 | **Agent "brain" / framework** | Thin **custom agent runtime** + **LiteLLM** (model-agnostic) + a **plugin interface** for skills/tools. (Alternative: CrewAI — faster, but less control over lifecycle/identity/metering.) |
| D2 | **Scale-to-zero mechanism** | **Operator-driven** — the operator scales each agent's deployment 0↔1 based on task assignment / ping / idle timeout. (Alternative: KEDA.) |
| D3 | **Async message bus** | **Postgres-backed queue** for v1 (no new infra, simplest) behind a swappable interface (NATS later if needed). |
| D4 | **Agent identity creation vs "minutes" flow** | The owner must create the agent identity + credential. Streamline with a **one-click "create agent identity"** the platform facilitates but the owner owns, so the fast path stays fast. |
| D5 | **Predefined LLM provider vs BYOM** | Fast path uses a **predefined provider** (admin-registered, works out of the box). BYOM is the "bring your own key" path. |
| D6 | **Buyer / deployment model** | v1 targets a **single organization operating its own Kubernetes install**, installed through the Helm chart and managed by the Skquad operator. Hosted multi-tenant service posture is future scope. |
| D7 | **License posture** | Skquad remains **Apache License 2.0** for v1. |

---

## 8. Traceability

| Requirement | Document |
|-------------|----------|
| Domain model & lifecycle | [`domain-model.md`](domain-model.md) |
| High-level architecture | [`ARCHITECTURE.md`](ARCHITECTURE.md) |
| Agent runtime | [`agent-runtime.md`](agent-runtime.md) |
| LLM gateway | [`llm-gateway.md`](llm-gateway.md) |
| Identity & security | [`identity-security.md`](identity-security.md) |
| Resource registry | [`resource-registry.md`](resource-registry.md) |
| Kanban & task lifecycle | [`kanban-task-lifecycle.md`](kanban-task-lifecycle.md) |
| Collaboration & messaging | [`collaboration-messaging.md`](collaboration-messaging.md) |
| Deployment & operator | [`deployment-operator.md`](deployment-operator.md) |
| Observability & metering | [`observability-metering.md`](observability-metering.md) |
| Data model | [`data-model.md`](data-model.md) |
| API design | [`api-design.md`](api-design.md) |
| Web app & UX | [`web-app-ux.md`](web-app-ux.md) |
| Plugin architecture | [`plugin-architecture.md`](plugin-architecture.md) |
| Security & threat model | [`security-threat-model.md`](security-threat-model.md) |
| Key decisions | [`adr/`](adr/) |
