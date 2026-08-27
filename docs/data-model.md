# skquad — Data Model (Postgres)

> **Status:** Draft v1 · **Decision:** [ADR-0005](adr/0005-persistence.md)
>
> Single **Postgres** store (with **pgvector**) for domain data, agent
> long-term memory, audit log, metering, and the v1 message queue. This is the
> physical schema behind the [domain model](domain-model.md).

---

## 1. Conventions

- Primary keys: `uuid` (default `gen_random_uuid()`).
- Timestamps: `timestamptz`.
- Soft deletes where noted (`status`); hard delete for cascaded squad resources.
- Credential values are **never** stored here — only **secret references**.
- `jsonb` for flexible/structured documents (operating model, permissions,
  metadata).

---

## 2. Identity & Users

```sql
CREATE TABLE users (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email         text NOT NULL UNIQUE,
    display_name  text,
    role          text NOT NULL DEFAULT 'user'
                  CHECK (role IN ('platform_admin', 'user')),
    oidc_subject  text UNIQUE,          -- IdP subject
    status        text NOT NULL DEFAULT 'active'
                  CHECK (status IN ('active', 'deactivated')),
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE agent_identities (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id      uuid NOT NULL,        -- set when attached to an agent
    subject       text NOT NULL UNIQUE, -- stable agent principal
    credential_ref text,                -- K8s secret ref
    credential_hash text,               -- one-way runtime auth verifier
    virtual_key_ref text,               -- LLM gateway virtual key ref
    created_by    uuid NOT NULL REFERENCES users(id),
    status        text NOT NULL DEFAULT 'active'
                  CHECK (status IN ('active', 'rotated', 'revoked')),
    created_at    timestamptz NOT NULL DEFAULT now(),
    rotated_at    timestamptz
);
```

---

## 3. Squads, Agents, Boards, Tasks

```sql
CREATE TABLE squads (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name          text NOT NULL,
    mission       text,
    operating_model jsonb,              -- roles + collaboration rules
    owner_id      uuid NOT NULL REFERENCES users(id),
    k8s_namespace text NOT NULL UNIQUE, -- squad-<id>
    status        text NOT NULL DEFAULT 'active'
                  CHECK (status IN ('active', 'archived', 'deleted')),
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE agents (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name          text NOT NULL,
    squad_id      uuid NOT NULL REFERENCES squads(id) ON DELETE CASCADE,
    role          text,                 -- from operating model
    identity_id   uuid REFERENCES agent_identities(id),
    credentials_ref text,               -- K8s secret ref
    default_provider_id uuid REFERENCES llm_providers(id),
    default_model text NOT NULL DEFAULT '', -- LiteLLM/gateway model alias
    status        text NOT NULL DEFAULT 'idle'
                  CHECK (status IN ('idle', 'busy', 'error', 'deleted')),
    idle_timeout  interval NOT NULL DEFAULT '300s',
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    UNIQUE (squad_id, name)
);

CREATE TABLE kanban_boards (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    squad_id      uuid NOT NULL UNIQUE REFERENCES squads(id) ON DELETE CASCADE,
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE tasks (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    board_id      uuid NOT NULL REFERENCES kanban_boards(id) ON DELETE CASCADE,
    title         text NOT NULL,
    description   text NOT NULL DEFAULT '',
    status        text NOT NULL DEFAULT 'todo'
                  CHECK (status IN ('todo','in-progress','in-review','done','blocked')),
    assignee_agent_id uuid REFERENCES agents(id) ON DELETE SET NULL,
    created_by_type text NOT NULL CHECK (created_by_type IN ('user','agent')),
    created_by_id uuid NOT NULL,        -- user id or agent id (polymorphic)
    position      integer NOT NULL DEFAULT 0,
    metadata      jsonb NOT NULL DEFAULT '{}',
    version       integer NOT NULL DEFAULT 0,  -- optimistic concurrency
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_tasks_board_status ON tasks(board_id, status, position);
CREATE INDEX idx_tasks_assignee ON tasks(assignee_agent_id) WHERE status IN ('todo','in-progress');
```

---

## 4. Resource Registry

```sql
CREATE TABLE llm_providers (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name          text NOT NULL UNIQUE,
    type          text NOT NULL,        -- openai | anthropic | ollama | ...
    base_url      text NOT NULL,
    api_key_ref   text,                 -- secret ref
    default_model text NOT NULL DEFAULT '', -- default LiteLLM/gateway model alias
    models        jsonb NOT NULL DEFAULT '[]',  -- list of model ids
    pricing       jsonb,                -- { input_per_token, output_per_token, currency }
    status        text NOT NULL DEFAULT 'active'
                  CHECK (status IN ('active', 'deprecated')),
    registered_by uuid NOT NULL REFERENCES users(id),
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE skills (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name          text NOT NULL UNIQUE,
    description   text,
    package_ref   text NOT NULL,        -- image / repo / path
    version       text NOT NULL DEFAULT '1',
    status        text NOT NULL DEFAULT 'active' CHECK (status IN ('active','deprecated')),
    registered_by uuid NOT NULL REFERENCES users(id),
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE tools (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name          text NOT NULL UNIQUE,
    description   text,
    schema        jsonb NOT NULL DEFAULT '{}',  -- parameter JSON schema
    endpoint_ref  text,
    status        text NOT NULL DEFAULT 'active' CHECK (status IN ('active','deprecated')),
    registered_by uuid NOT NULL REFERENCES users(id),
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE apis (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name          text NOT NULL UNIQUE,
    description   text,
    base_url      text NOT NULL,
    auth_ref      text,
    spec_ref      text,                 -- OpenAPI spec ref
    status        text NOT NULL DEFAULT 'active' CHECK (status IN ('active','deprecated')),
    registered_by uuid NOT NULL REFERENCES users(id),
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE knowledge_bases (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name          text NOT NULL UNIQUE,
    description   text,
    vector_db_ref text NOT NULL,        -- connection secret ref
    collection    text NOT NULL,
    embedding_model text,
    status        text NOT NULL DEFAULT 'active' CHECK (status IN ('active','deprecated')),
    registered_by uuid NOT NULL REFERENCES users(id),
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE project_workspaces (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name          text NOT NULL UNIQUE,
    description   text,
    type          text NOT NULL CHECK (type IN ('git','jira','confluence')),
    endpoint      text NOT NULL,
    auth_ref      text,
    status        text NOT NULL DEFAULT 'active' CHECK (status IN ('active','deprecated')),
    registered_by uuid NOT NULL REFERENCES users(id),
    created_at    timestamptz NOT NULL DEFAULT now()
);
```

---

## 5. Permissions & Access Grants

```sql
-- Agent → registry resource grants (Layer 2 RBAC, squad-owner managed)
CREATE TABLE agent_permissions (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id      uuid NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    resource_type text NOT NULL
                  CHECK (resource_type IN
                    ('llm_provider','skill','tool','api','knowledge_base','project_workspace')),
    resource_id   uuid NOT NULL,
    granted_by    uuid NOT NULL REFERENCES users(id),
    created_at    timestamptz NOT NULL DEFAULT now(),
    UNIQUE (agent_id, resource_type, resource_id)
);
CREATE INDEX idx_agent_permissions_agent ON agent_permissions(agent_id);

-- Cross-user / cross-squad access grants (owner-issued)
CREATE TABLE access_grants (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    squad_id      uuid NOT NULL REFERENCES squads(id) ON DELETE CASCADE,
    grantee_type  text NOT NULL CHECK (grantee_type IN ('user','agent')),
    grantee_id    uuid NOT NULL,        -- user id or agent id
    permissions   jsonb NOT NULL DEFAULT '["talk_to_agents"]',
    granted_by    uuid NOT NULL REFERENCES users(id),
    status        text NOT NULL DEFAULT 'active' CHECK (status IN ('active','revoked')),
    created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_access_grants_squad ON access_grants(squad_id);
CREATE INDEX idx_access_grants_grantee ON access_grants(grantee_type, grantee_id);
```

---

## 6. Messaging (v1 queue)

```sql
CREATE TABLE messages (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    from_type     text NOT NULL CHECK (from_type IN ('agent','user')),
    from_id       uuid NOT NULL,
    to_agent_id   uuid NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    squad_id      uuid NOT NULL REFERENCES squads(id) ON DELETE CASCADE,
    type          text NOT NULL
                  CHECK (type IN ('consult','delegate','handoff','ping','reply')),
    payload       jsonb NOT NULL DEFAULT '{}',
    status        text NOT NULL DEFAULT 'pending'
                  CHECK (status IN ('pending','delivered','expired','dead')),
    correlation_id uuid,                -- links a reply to its consult
    ttl           interval,
    created_at    timestamptz NOT NULL DEFAULT now(),
    delivered_at  timestamptz
);
CREATE INDEX idx_messages_inbox ON messages(to_agent_id, status, created_at)
    WHERE status = 'pending';
CREATE INDEX idx_messages_squad ON messages(squad_id, created_at);
```

Current implementation note: the embedded migration creates this durable inbox
schema, but message enqueue/claim/ack APIs are still tracked as a follow-up
implementation slice.

---

## 7. Audit Log

```sql
CREATE TABLE audit_log (
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    actor_type    text NOT NULL CHECK (actor_type IN ('user','agent','system')),
    actor_id      uuid NOT NULL,
    action        text NOT NULL,        -- e.g. 'squad.create', 'task.assign'
    resource_type text,
    resource_id   uuid,
    squad_id      uuid,
    metadata      jsonb NOT NULL DEFAULT '{}',
    timestamp     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_audit_squad_time ON audit_log(squad_id, timestamp);
CREATE INDEX idx_audit_actor_time ON audit_log(actor_type, actor_id, timestamp);
-- Partition by month for retention (see below).
```

---

## 8. Metering

```sql
CREATE TABLE metering (
    id            bigint GENERATED ALWAYS AS IDENTITY,
    agent_id      uuid NOT NULL,
    squad_id      uuid NOT NULL,
    model         text NOT NULL,
    provider      text NOT NULL,
    input_tokens  bigint NOT NULL DEFAULT 0,
    output_tokens bigint NOT NULL DEFAULT 0,
    cost          numeric(18,8),        -- null if no pricing configured
    currency      text,
    timestamp     timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (id, timestamp)
) PARTITION BY RANGE (timestamp);
-- Create monthly partitions; retain raw for N months, then aggregate + drop.
CREATE INDEX idx_metering_agent_time ON metering(agent_id, timestamp);
CREATE INDEX idx_metering_squad_time ON metering(squad_id, timestamp);
```

---

## 9. Agent Long-Term Memory (pgvector)

```sql
CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE agent_memory (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id      uuid NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    squad_id      uuid REFERENCES squads(id) ON DELETE CASCADE,
    content       text NOT NULL,        -- the durable fact / decision
    embedding     vector(1536),         -- dimension matches embedding model
    source_task_id uuid REFERENCES tasks(id) ON DELETE SET NULL,
    metadata      jsonb NOT NULL DEFAULT '{}',
    created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_memory_agent ON agent_memory(agent_id);
CREATE INDEX idx_memory_squad ON agent_memory(squad_id) WHERE squad_id IS NOT NULL;
CREATE INDEX idx_memory_embedding ON agent_memory
    USING ivfflat (embedding vector_cosine_ops);  -- tune lists for data size
```

- **Per-agent** memory by default; runtime reads always constrain by `agent_id`.
- `squad_id` scopes memories to a squad/task context. Shared squad memory is a
  later policy decision, not implied by setting `squad_id` alone.
- Semantic search via cosine similarity on `embedding`.

Current implementation note: the embedded migration creates the pgvector-backed
memory table and indexes. The control plane now supports bounded recent-memory
reads for task context and writes task completion summaries as memory rows when
the runtime explicitly requests persistence. Semantic embedding writes/search
and artifact storage are still follow-up implementation slices.

---

## 10. Relationships (summary)

```
users 1—* squads (owner)
squads 1—1 kanban_boards
squads 1—* agents
agents *—1 agent_identities
agents *—1 llm_providers (default provider)
agents default_model —> gateway model alias
agents *—* {registry resources} via agent_permissions
squads 1—* access_grants
kanban_boards 1—* tasks
tasks *—1 agents (assignee)
agents 1—* messages (inbox)
agents 1—* agent_memory
agents 1—* metering
squads 1—* metering
```

---

## 11. Partitioning & Retention

| Table | Strategy |
|-------|----------|
| `metering` | Range-partition by `timestamp` (monthly). Retain raw N months; aggregate + drop older. |
| `audit_log` | Range-partition by `timestamp` (monthly). Retain longer (compliance). |
| `messages` | Prune `delivered`/`expired`/`dead` older than a retention window. |
| `agent_memory` | Retain per agent; optional pruning of low-value rows (later). |

---

## 12. Security Notes

- **No raw secrets** in this schema — only `*_ref` (K8s secret references).
- **Least-privilege DB roles** — the API server, gateway, and operator each get
  a role scoped to the tables they need.
- **Row-level isolation** — squad-scoped queries always filter by `squad_id`
  (enforced in the API layer + RLS optionally).
- **Audit** — all significant writes are mirrored to `audit_log`.

---

## 13. Open Points

- **Embedding dimension** — set to match the chosen embedding model (1536 for
  OpenAI `text-embedding-3-small`; adjust as needed).
- **RLS** — whether to enable Postgres Row-Level Security for squad isolation
  (defense-in-depth; start with app-level enforcement).
- **Metering aggregation** — materialized views / rollups for fast dashboards
  (later).
- **Soft vs hard delete** — confirm per table (squads cascade hard; users/agents
  soft where noted).
