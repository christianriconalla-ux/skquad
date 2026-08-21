# skquad — Agent Runtime Design

> **Status:** Draft v1 · **Decision:** [ADR-0001](adr/0001-agent-runtime.md)
>
> The agent runtime is the **thin custom harness** that runs inside each agent
> pod. It owns the agent's lifecycle, task-scoped context, the core work loop,
> and the plugin interface. It is **model-agnostic** (via LiteLLM) and
> **extensible** (via plugins).

---

## 1. Responsibilities

The agent runtime:

1. **Boots** the agent (loads config, identity, permissions, plugins).
2. **Waits for work** — a task assigned to it, or a message in its inbox.
3. **Resets its working context** before starting a new task.
4. **Runs the core loop** — plan → act (via plugins) → observe → complete.
5. **Calls the LLM** through the **LLM gateway** (model-agnostic, metered).
6. **Uses permitted resources** — tools, skills, knowledge bases, workspaces.
7. **Sends/receives async messages** (delegate, consult, cross-squad).
8. **Persists long-term memory** (Postgres + pgvector).
9. **Reports status** (idle/busy) so the operator can scale it to zero.

It does **not** own: identity creation, RBAC, metering storage, or scaling —
those live in the control plane / operator.

---

## 2. Agent Lifecycle (inside the pod)

```mermaid
stateDiagram-v2
    [*] --> Booting
    Booting --> Idle: config + plugins loaded
    Idle --> Working: task assigned OR message pending
    Working --> Idle: task done + inbox drained
    Working --> Error: crash / health failure
    Error --> Idle: recovered (restart)
    Idle --> [*]: scale-to-zero (operator)
```

- **Booting:** load agent config (role, permissions, default provider), the
  agent identity/credential, and the enabled plugins. Connect to the LLM
  gateway, message queue, and Postgres.
- **Idle:** no pending work. The runtime reports `idle` to the control plane.
  After the idle timeout, the operator scales the pod to 0.
- **Working:** a task is active (or a message is being handled). The runtime
  reports `busy`. Incoming messages are **queued** (not delivered) until the
  current task completes — protecting the task context.
- **Error:** crashloop / health failure. Surfaced to the owner; the operator may
  restart the pod.

---

## 3. Task-Scoped Context

A core requirement (FR-3): the agent's **working context / chat history is
reset just before starting a new task** from the Kanban board.

- **Working context** (ephemeral, in-memory): the conversation + scratch state
  for the *current* task. **Cleared** when a new task starts.
- **Long-term memory** (persistent, Postgres + pgvector): facts, decisions, and
  learnings that survive across tasks. The runtime can **write to** and
  **retrieve from** it during a task.

```
new task starts
  → clear working context
  → (optionally) retrieve relevant long-term memory into context
  → run the core loop for the task
  → on completion, distill durable facts into long-term memory
```

This gives each task a clean slate while letting the agent accumulate durable
knowledge over time.

---

## 4. Core Loop

```mermaid
flowchart LR
    A[Pick up task] --> B[Plan]
    B --> C[Act via plugins]
    C --> D[Observe result]
    D --> E{Done?}
    E -->|no| C
    E -->|yes| F[Update task + memory]
    F --> G[Report idle]
```

1. **Pick up task** — load the task (title, description, any attached context).
2. **Plan** — use the LLM (via the gateway) to break the task into steps, given
   the agent's role and the operating model.
3. **Act** — execute steps by calling **plugins** (tools, skills) and the LLM.
4. **Observe** — feed results back; iterate until the task is complete.
5. **Complete** — update the task status, distill durable facts into long-term
   memory, and report idle.

The loop is intentionally small. Complexity is pushed into **plugins** and the
**LLM**, not the harness.

---

## 5. LLM Calls (via LiteLLM + the LLM Gateway)

- The runtime uses **LiteLLM** to call models in a **model-agnostic** way.
- All calls go to the **central LLM gateway** (LiteLLM proxy) using the agent's
  **virtual key** — so calls are metered, attributed, and permission-checked
  centrally (see [llm-gateway.md](llm-gateway.md)).
- The runtime requests a **model** (from the agent's permitted providers); the
  gateway routes to the correct upstream provider.
- The runtime never holds upstream provider credentials — only its own virtual
  key.

```python
# Pseudocode
resp = litellm.completion(
    model=agent.default_model,          # e.g. "gpt-4o", "claude-...", "llama3"
    messages=working_context.messages,
    api_base=LLM_GATEWAY_URL,           # central gateway
    api_key=agent.virtual_key,          # per-agent virtual key
    tools=enabled_tool_schemas,         # from plugins
)
```

---

## 6. Plugin Interface

Capabilities are **plugins** (see [plugin-architecture.md](plugin-architecture.md)).
The runtime exposes a small interface that plugins implement:

```python
class Plugin(Protocol):
    name: str
    def manifest(self) -> Manifest: ...        # name, version, capabilities
    def tools(self) -> list[ToolSchema]: ...   # callable functions exposed to the LLM
    def skills(self) -> list[Skill]: ...       # packaged capabilities
    async def invoke(self, call: ToolCall) -> Result: ...
```

- **Tools** — callable functions the LLM can invoke (e.g. "run query", "read
  file", "call API").
- **Skills** — packaged, reusable capabilities (prompt + logic) the agent can
  apply.
- **Resource connectors** — plugins that connect to registry resources
  (knowledge bases, git/Jira/Confluence workspaces). They use the agent's
  permissions + credentials.

The runtime loads the plugins the agent is **permitted** to use (from its
permission set) and exposes their tools/skills to the LLM.

---

## 7. Knowledge Base Access (RAG)

- A **RAG plugin** connects to the agent's permitted **knowledge bases**
  (registered vector DBs).
- During a task, the runtime can **retrieve** relevant chunks (semantic search)
  and inject them into the working context.
- The agent's **own long-term memory** (Postgres + pgvector) is a separate,
  built-in store (see §3).

---

## 8. Messaging (async)

- The runtime has an **inbox** (per-agent queue, see
  [collaboration-messaging.md](collaboration-messaging.md)).
- While **working**, new messages are **queued** (not delivered) — protecting
  the task context.
- When the task completes and the runtime is **idle**, it drains the inbox:
  - **Consultation** → answer using the LLM + context, send the reply.
  - **Delegation / task handoff** → create/assign a task (possibly on another
    squad's board, if permitted).
- All messaging is **asynchronous** and goes through the control plane (which
  enforces access grants).

---

## 9. Long-Term Memory (Postgres + pgvector)

- A built-in **memory store** in Postgres with a `vector` column.
- **Write:** on task completion (or on explicit "remember"), distill durable
  facts/decisions into memory rows (with embeddings).
- **Read:** at task start (or on demand), retrieve relevant memory rows by
  semantic similarity and inject into the working context.
- Scoped **per agent** (and optionally per squad for shared memory).

---

## 10. Configuration

An agent's runtime config (managed by the squad owner via the API):

- **Role** (from the operating model) — e.g. "planner", "coder", "reviewer".
- **Default LLM provider / model** (from the registry).
- **Permissions** — which registry resources (providers, tools, skills, KBs,
  workspaces) the agent may use.
- **Enabled plugins** — derived from permissions.
- **Idle timeout** — for scale-to-zero.
- **Operating model excerpt** — the collaboration rules relevant to this agent.

---

## 11. Error Handling & Health

- **Health endpoint** (`/healthz`, `/readyz`) for the operator's probes.
- **LLM errors** — retry with backoff; surface persistent failures to the task
  (mark `blocked`) and to the owner.
- **Plugin errors** — caught per-invocation; reported to the LLM as a tool
  result (so it can adapt) and logged.
- **Crashloop** — the operator restarts the pod; the task is re-queued (idempotent
  pickup).
- **Audit** — significant actions (task start/complete, memory writes, messages)
  are reported to the control plane for the audit log.

---

## 12. Pod Layout

```
agent pod
├── agent-runtime (Python)          # the harness (this component)
│   ├── core loop
│   ├── context manager (task-scoped)
│   ├── memory store client (pgvector)
│   ├── message client (inbox)
│   └── plugin loader
├── plugins/                         # enabled plugins (tools, skills, connectors)
└── config (mounted)                 # agent config + virtual key (secret)
```

The pod is **stateless** with respect to working context (it is reset per task
and the pod can scale to zero). Durable state lives in Postgres.

---

## 13. Open Points

- **Planning strategy** — single-shot plan vs. iterative re-planning; decide
  during implementation (start simple: one plan, iterate on observations).
- **Shared squad memory** — whether agents in a squad share a memory space or
  keep per-agent memory (start per-agent; add shared later).
- **Context window management** — summarisation/eviction policy for long tasks.
