# ADR-0001: Agent Runtime — Thin Custom Runtime + LiteLLM + Plugin Interface

- **Status:** Accepted
- **Date:** 2026-08-22
- **Deciders:** Ross, Sherlock

## Context

skquad is a **platform**, not a single agent application. A platform must own
the agent's **lifecycle** (create, scale-to-zero, destroy), **identity**
(owner-created), **permissions** (RBAC), **metering** (central gateway), and
**isolation** (per-squad namespace, per-agent pod).

Off-the-shelf multi-agent frameworks (CrewAI, AutoGen, LangGraph) are designed
to build **one agent application** (a "crew" of agents in one process). They do
not give the platform-level control skquad needs, and adopting one would mean
fighting the framework for lifecycle, identity, metering, and multi-tenancy.

At the same time, skquad must be **simple to start** and **extensible via
plugins** (per the requirements), and must be **model-agnostic** (BYOM).

## Decision

Build a **thin custom agent runtime** (the "agent harness") in **Python** that:

- Owns the agent's **lifecycle** and **task-scoped context** (reset before each
  new task).
- Uses **LiteLLM** for **model-agnostic** LLM calls, routed through the central
  **LLM gateway**.
- Exposes a **clean plugin interface** for **skills** and **tools** (new
  capabilities are plugins — directly satisfying the extensibility requirement).
- Connects to the **async message queue** for agent↔agent communication.
- Accesses **knowledge bases** via a RAG plugin.
- Enforces **agent permissions** (only use permitted registry resources).
- Persists **long-term memory** in Postgres + pgvector.

The core loop (pick up task → plan → act via plugins → observe → complete) is
small and owned by us; everything else plugs in.

## Consequences

- **(+)** Full control over lifecycle, identity, metering, isolation, and
  multi-tenancy — the things a platform must own.
- **(+)** Model-agnostic (LiteLLM) → BYOM is clean.
- **(+)** Plugin interface → extensibility without core changes.
- **(+)** Simple core → easy to start (the "minutes" flow).
- **(−)** More initial work than adopting CrewAI/AutoGen.
- **(−)** We own the runtime's correctness (context management, tool calling,
  error handling).
- **Mitigation:** keep the core loop minimal; lean on LiteLLM for the hard
  model-agnostic parts; add a focused test suite for the runtime.

## Alternatives Considered

- **CrewAI** — role-based multi-agent "crews"; maps nicely to squads and is
  fast to start. **Rejected** for v1 because it does not expose the
  platform-level control (lifecycle, identity, metering, per-agent pods,
  scale-to-zero) skquad requires. Revisit if the custom runtime proves too
  heavy.
- **AutoGen / LangGraph** — powerful conversation/graph frameworks. **Rejected**
  for the same platform-control reasons; also add conceptual complexity that
  conflicts with "keep it simple."
- **OpenAI Agents SDK** — lightweight but OpenAI-centric; conflicts with BYOM
  and platform control. **Rejected.**
