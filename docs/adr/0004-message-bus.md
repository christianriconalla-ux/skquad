# ADR-0004: Async Message Bus — Postgres-Backed Queue (v1)

- **Status:** Accepted
- **Date:** 2026-08-22
- **Deciders:** Ross, Sherlock

## Context

Agents communicate **asynchronously** (FR-5): within a squad (delegate,
consult) and across squads (task handoff + ping). A busy agent must **not be
disturbed** — messages are queued and delivered only when the agent is free, to
protect its task context.

We need a message bus that:

- Queues messages per agent (so a busy agent's inbox accumulates without
  interrupting it).
- Supports delivery when the agent becomes free.
- Is **simple** (no new infrastructure for v1) and **swappable** (can move to a
  dedicated broker later).

## Decision

For **v1**, implement the message bus as a **Postgres-backed queue** behind a
small **MessageBus interface**:

- A `messages` table (or a set of tables) stores queued messages with a
  per-agent inbox, status (`pending`, `delivered`, `expired`), and ordering.
- The **control plane** enqueues messages (after enforcing access grants).
- An **agent** polls/claims its next message when it is free (task complete).
- The **MessageBus interface** abstracts enqueue/claim/ack so the backend can be
  swapped (e.g. to **NATS**) without changing agent or control-plane code.

This keeps v1 dependency-light (we already run Postgres) while preserving the
option to move to a purpose-built broker.

## Consequences

- **(+)** No new infrastructure for v1 → simple to deploy and operate.
- **(+)** Durable (Postgres) → messages survive restarts.
- **(+)** Swappable interface → can move to NATS later without rework.
- **(+)** Access-grant enforcement lives in the control plane (single point).
- **(−)** Postgres is not a real-time broker → higher latency, lower throughput
  than NATS/Kafka.
- **(−)** Postgres notification delivery is best-effort around reconnects and
  delayed retries; keep relaxed fallback polling.
- **Mitigation:** keep message volume modest in v1; use `LISTEN/NOTIFY` for
  task/inbox wake-ups; move to NATS if throughput/latency requirements grow.

## Alternatives Considered

- **NATS** — lightweight, purpose-built pub/sub + request/reply; excellent for
  agent messaging. **Rejected** for v1 (adds a dependency) but is the **planned
  v2 backend** behind the same interface.
- **Kafka** — high-throughput, durable, but heavy for v1's scale. **Rejected.**
- **Redis Streams** — good middle ground, but adds Redis as a dependency.
  **Rejected** for v1 (we already have Postgres).
