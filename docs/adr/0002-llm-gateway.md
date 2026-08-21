# ADR-0002: Central LLM Gateway — LiteLLM Proxy

- **Status:** Accepted
- **Date:** 2026-08-22
- **Deciders:** Ross, Sherlock

## Context

skquad is **BYOM**: agents can use any LLM in the provider registry. It must
also **meter tokens** per agent and per squad, and show **monetary cost** when
a provider has per-token pricing. Agent code must stay **model-agnostic**.

If every agent called LLM providers directly, we would have to (a) duplicate
metering logic in every agent, (b) scatter credential handling, and (c) make
agent code model-specific. A **central gateway** that all agent LLM calls flow
through solves all three in one place.

## Decision

Deploy a **central LLM gateway** in the control plane, implemented with the
**LiteLLM proxy**, that:

- Is **model-agnostic** — agents call a single endpoint; the gateway routes to
  the correct provider (OpenAI, Anthropic, Ollama, …) based on the requested
  model.
- **Meters tokens** (input + output) per call and attributes them to the
  **agent** and **squad**.
- Computes **cost** when the provider has **per-token pricing** configured.
- Enforces **agent permissions** — an agent can only use providers/models it is
  permitted to use.
- Handles **BYOM** — routes to the right provider with the right credentials
  (from the registry / secrets).
- Issues **virtual keys** per agent so calls are attributable and rate-limitable.

## Consequences

- **(+)** Single enforcement point for metering, cost, permissions, and BYOM.
- **(+)** Agent code stays model-agnostic (calls one endpoint).
- **(+)** LiteLLM proxy provides virtual keys, budgeting, and usage tracking
  out of the box.
- **(+)** Central place to add caching, fallbacks, and rate limits later.
- **(−)** The gateway is a critical path for all agent LLM calls (availability
  matters).
- **(−)** One more component to operate.
- **Mitigation:** run the gateway as a highly-available Deployment; it is
  stateless (state in Postgres); add health checks + alerts.

## Alternatives Considered

- **Agents call providers directly** — simpler to start, but duplicates metering
  and credential handling and breaks model-agnosticism. **Rejected.**
- **Custom gateway in Go** — full control, but re-implements what LiteLLM proxy
  already does (routing, virtual keys, budgets, usage). **Rejected** for v1;
  revisit if LiteLLM proxy becomes a limitation.
