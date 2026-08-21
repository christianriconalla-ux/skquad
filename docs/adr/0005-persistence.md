# ADR-0005: Data Persistence — Postgres + pgvector

- **Status:** Accepted
- **Date:** 2026-08-22
- **Deciders:** Ross, Sherlock

## Context

skquad needs persistent storage for:

- **Domain data** — users, squads, agents, boards, tasks, registry, access
  grants.
- **Agent long-term memory** — semantic memory that persists across tasks
  (FR-3).
- **Audit log** — who-did-what trail (NFR-1).
- **Metering** — token usage + cost per agent/squad (FR-8).
- **Async messages** — the v1 message queue (ADR-0004).

The requirements call for **Postgres** for data persistence and **Postgres +
pgvector** for agent long-term memory.

## Decision

Use a **single Postgres** instance (with the **pgvector** extension) as the
primary store for all of the above:

- **Domain data** in relational tables.
- **Agent long-term memory** in a table with a `vector` column (pgvector) for
  semantic search.
- **Audit log** in an append-only table.
- **Metering** in a time-series-friendly table (partitioned by time if needed).
- **Messages** in the queue tables (ADR-0004).

A single store keeps v1 simple (one dependency, one backup/restore story, one
set of credentials) while pgvector covers the semantic-memory requirement
without a separate vector database.

## Consequences

- **(+)** One dependency → simple to deploy, operate, back up.
- **(+)** pgvector covers long-term memory without a separate vector DB.
- **(+)** Transactional consistency across domain data, audit, and metering.
- **(+)** The registry's *knowledge bases* (external vector DBs) are still
  supported as **resources** agents can use — pgvector is for the agent's own
  memory, not a replacement for registered KBs.
- **(−)** Postgres is not a dedicated vector DB → very large semantic-memory
  workloads may need tuning or a dedicated KB.
- **(−)** Metering volume can grow; mitigate with time-based partitioning and
  retention policies.
- **Mitigation:** partition metering + audit by time; add retention; move
  heavy vector workloads to a registered KB if needed.

## Alternatives Considered

- **Separate vector DB (Qdrant, etc.) for memory** — better for very large
  semantic workloads, but adds a dependency. **Rejected** for v1; pgvector is
  sufficient and the registry already supports external KBs.
- **Multiple stores (Postgres + Redis + vector DB)** — more moving parts.
  **Rejected** for v1 simplicity.
