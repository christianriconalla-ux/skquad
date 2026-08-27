-- skquad control-plane initial schema.
-- Mirrors docs/data-model.md. Secrets are never stored here — only *_ref
-- (K8s secret references).
CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS vector;

-- ---------------------------------------------------------------------------
-- Users (Layer-1 RBAC principals)
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS users (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    oidc_issuer text,
    oidc_subject text,
    email       text NOT NULL,
    email_verified boolean NOT NULL DEFAULT false,
    name        text NOT NULL DEFAULT '',
    role        text NOT NULL DEFAULT 'user'
                CHECK (role IN ('platform_admin','user')),
    created_at  timestamptz NOT NULL DEFAULT now()
);
ALTER TABLE users ADD COLUMN IF NOT EXISTS oidc_issuer text;
ALTER TABLE users ADD COLUMN IF NOT EXISTS oidc_subject text;
ALTER TABLE users ADD COLUMN IF NOT EXISTS email_verified boolean NOT NULL DEFAULT false;
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_email_key;
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_oidc_subject
    ON users(oidc_issuer, oidc_subject)
    WHERE oidc_issuer IS NOT NULL AND oidc_subject IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);

-- ---------------------------------------------------------------------------
-- Squads (1 : 1 namespace, 1 : 1 board, 1 : * agents)
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS squads (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name            text NOT NULL,
    mission         text NOT NULL DEFAULT '',
    operating_model jsonb NOT NULL DEFAULT '{}',
    owner_id        uuid NOT NULL REFERENCES users(id),
    namespace       text NOT NULL,
    status          text NOT NULL DEFAULT 'active'
                    CHECK (status IN ('active','archived')),
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (owner_id, name)
);
CREATE INDEX IF NOT EXISTS idx_squads_owner ON squads(owner_id);

-- ---------------------------------------------------------------------------
-- Agents (1 pod each, own identity + permissions)
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS agents (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    squad_id           uuid NOT NULL REFERENCES squads(id) ON DELETE CASCADE,
    name               text NOT NULL,
    role               text NOT NULL DEFAULT '',
    identity_id        uuid,
    default_provider   uuid,
    default_model      text NOT NULL DEFAULT '',
    permissions        jsonb NOT NULL DEFAULT '[]',
    idle_timeout_sec   integer NOT NULL DEFAULT 300,
    status             text NOT NULL DEFAULT 'idle'
                       CHECK (status IN ('idle','busy','error')),
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now(),
    UNIQUE (squad_id, name)
);
CREATE INDEX IF NOT EXISTS idx_agents_squad ON agents(squad_id);
ALTER TABLE agents ADD COLUMN IF NOT EXISTS default_model text NOT NULL DEFAULT '';

-- ---------------------------------------------------------------------------
-- Agent identities (owner-created; credential + virtual-key refs)
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS agent_identities (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id       uuid NOT NULL UNIQUE REFERENCES agents(id) ON DELETE CASCADE,
    credential_ref text NOT NULL,
    credential_hash text NOT NULL DEFAULT '',
    virtual_key_ref text,
    created_by     uuid NOT NULL REFERENCES users(id),
    created_at     timestamptz NOT NULL DEFAULT now(),
    rotated_at     timestamptz
);
ALTER TABLE agent_identities ADD COLUMN IF NOT EXISTS credential_hash text NOT NULL DEFAULT '';

-- ---------------------------------------------------------------------------
-- Kanban boards (one per squad)
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS kanban_boards (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    squad_id   uuid NOT NULL UNIQUE REFERENCES squads(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now()
);

-- ---------------------------------------------------------------------------
-- Tasks (unit of work)
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS tasks (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    board_id          uuid NOT NULL REFERENCES kanban_boards(id) ON DELETE CASCADE,
    squad_id          uuid NOT NULL REFERENCES squads(id) ON DELETE CASCADE,
    title             text NOT NULL,
    description       text NOT NULL DEFAULT '',
    status            text NOT NULL DEFAULT 'todo'
                      CHECK (status IN ('todo','in-progress','in-review','done','blocked')),
    assignee_agent_id uuid REFERENCES agents(id) ON DELETE SET NULL,
    created_by_type   text NOT NULL CHECK (created_by_type IN ('user','agent')),
    created_by_id     uuid NOT NULL,
    position          integer NOT NULL DEFAULT 0,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_tasks_board ON tasks(board_id, position);
CREATE INDEX IF NOT EXISTS idx_tasks_squad ON tasks(squad_id);
CREATE INDEX IF NOT EXISTS idx_tasks_assignee ON tasks(assignee_agent_id);

-- ---------------------------------------------------------------------------
-- Task executions (runtime attempts with leases and fencing)
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS task_executions (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id          uuid NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    agent_id         uuid NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    worker_id        text NOT NULL,
    fencing_token    text NOT NULL UNIQUE DEFAULT gen_random_uuid()::text,
    status           text NOT NULL DEFAULT 'active'
                     CHECK (status IN ('active','completed','blocked','expired')),
    lease_expires_at timestamptz NOT NULL,
    result_status    text CHECK (result_status IN ('in-review','done','blocked')),
    result_summary   text NOT NULL DEFAULT '',
    started_at       timestamptz NOT NULL DEFAULT now(),
    completed_at     timestamptz,
    updated_at       timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_task_executions_task
    ON task_executions(task_id, status, lease_expires_at);
CREATE INDEX IF NOT EXISTS idx_task_executions_agent_active
    ON task_executions(agent_id, lease_expires_at)
    WHERE status = 'active';

-- ---------------------------------------------------------------------------
-- Registry: LLM providers (BYOM)
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS llm_providers (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name          text NOT NULL UNIQUE,
    kind          text NOT NULL,
    base_url      text NOT NULL,
    api_key_ref   text NOT NULL,
    default_model text NOT NULL DEFAULT '',
    models        jsonb NOT NULL DEFAULT '[]',
    pricing       jsonb NOT NULL DEFAULT '{}',
    status        text NOT NULL DEFAULT 'active'
                  CHECK (status IN ('active','deprecated')),
    registered_by uuid NOT NULL REFERENCES users(id),
    created_at    timestamptz NOT NULL DEFAULT now()
);
ALTER TABLE llm_providers ADD COLUMN IF NOT EXISTS default_model text NOT NULL DEFAULT '';

-- ---------------------------------------------------------------------------
-- Registry: generic resources (skills, tools, apis, knowledge bases, workspaces)
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS registry_resources (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    type          text NOT NULL
                  CHECK (type IN ('skill','tool','api','knowledge_base','project_workspace')),
    name          text NOT NULL,
    description   text NOT NULL DEFAULT '',
    endpoint      text NOT NULL DEFAULT '',
    auth_ref      text NOT NULL DEFAULT '',
    manifest      jsonb NOT NULL DEFAULT '{}',
    status        text NOT NULL DEFAULT 'active'
                  CHECK (status IN ('active','deprecated')),
    registered_by uuid NOT NULL REFERENCES users(id),
    created_at    timestamptz NOT NULL DEFAULT now(),
    UNIQUE (type, name)
);
CREATE INDEX IF NOT EXISTS idx_registry_type ON registry_resources(type);

-- ---------------------------------------------------------------------------
-- Agent permissions (Layer-2 RBAC: agent -> registry resource)
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS agent_permissions (
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
CREATE INDEX IF NOT EXISTS idx_agent_permissions_agent ON agent_permissions(agent_id);

-- ---------------------------------------------------------------------------
-- Access grants (owner-issued: user or cross-squad agent -> squad)
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS access_grants (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    squad_id     uuid NOT NULL REFERENCES squads(id) ON DELETE CASCADE,
    grantee_type text NOT NULL CHECK (grantee_type IN ('user','agent')),
    grantee_id   uuid NOT NULL,
    permissions  text NOT NULL DEFAULT 'talk',
    granted_by   uuid NOT NULL REFERENCES users(id),
    created_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (squad_id, grantee_type, grantee_id)
);
CREATE INDEX IF NOT EXISTS idx_access_grants_squad ON access_grants(squad_id);

-- ---------------------------------------------------------------------------
-- Messages (v1 Postgres-backed per-agent inbox)
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS messages (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    from_type      text NOT NULL CHECK (from_type IN ('agent','user')),
    from_id        uuid NOT NULL,
    to_agent_id    uuid NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    squad_id       uuid NOT NULL REFERENCES squads(id) ON DELETE CASCADE,
    type           text NOT NULL
                   CHECK (type IN ('consult','delegate','handoff','ping','reply')),
    payload        jsonb NOT NULL DEFAULT '{}',
    status         text NOT NULL DEFAULT 'pending'
                   CHECK (status IN ('pending','delivered','expired','dead')),
    correlation_id uuid,
    attempts       integer NOT NULL DEFAULT 0,
    max_attempts   integer NOT NULL DEFAULT 3,
    next_retry_at  timestamptz NOT NULL DEFAULT now(),
    ttl            interval,
    expires_at     timestamptz NOT NULL DEFAULT now() + interval '24 hours',
    terminal_reason text NOT NULL DEFAULT '',
    created_at     timestamptz NOT NULL DEFAULT now(),
    delivered_at   timestamptz
);
ALTER TABLE messages ADD COLUMN IF NOT EXISTS attempts integer NOT NULL DEFAULT 0;
ALTER TABLE messages ADD COLUMN IF NOT EXISTS max_attempts integer NOT NULL DEFAULT 3;
ALTER TABLE messages ADD COLUMN IF NOT EXISTS next_retry_at timestamptz NOT NULL DEFAULT now();
ALTER TABLE messages ADD COLUMN IF NOT EXISTS expires_at timestamptz NOT NULL DEFAULT now() + interval '24 hours';
ALTER TABLE messages ADD COLUMN IF NOT EXISTS terminal_reason text NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_messages_inbox ON messages(to_agent_id, status, created_at)
    WHERE status = 'pending';
CREATE INDEX IF NOT EXISTS idx_messages_retry ON messages(to_agent_id, status, next_retry_at)
    WHERE status = 'pending';
CREATE INDEX IF NOT EXISTS idx_messages_squad ON messages(squad_id, created_at);

-- ---------------------------------------------------------------------------
-- Metering (token usage; partition by month in production)
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS metering (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id      uuid NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    squad_id      uuid NOT NULL REFERENCES squads(id) ON DELETE CASCADE,
    task_id       uuid REFERENCES tasks(id) ON DELETE SET NULL,
    provider_id   uuid,
    model         text NOT NULL DEFAULT '',
    input_tokens  integer NOT NULL DEFAULT 0,
    output_tokens integer NOT NULL DEFAULT 0,
    cost          double precision NOT NULL DEFAULT 0,
    currency      text NOT NULL DEFAULT 'USD',
    timestamp     timestamptz NOT NULL DEFAULT now()
);
ALTER TABLE IF EXISTS metering ADD COLUMN IF NOT EXISTS task_id uuid REFERENCES tasks(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS idx_metering_squad ON metering(squad_id, timestamp);
CREATE INDEX IF NOT EXISTS idx_metering_agent ON metering(agent_id, timestamp);
CREATE INDEX IF NOT EXISTS idx_metering_task ON metering(task_id, timestamp);

-- ---------------------------------------------------------------------------
-- Audit log (append-only)
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS audit_log (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    actor_type    text NOT NULL CHECK (actor_type IN ('user','agent','system')),
    actor_id      uuid NOT NULL,
    action        text NOT NULL,
    resource_type text NOT NULL DEFAULT '',
    resource_id   uuid,
    squad_id      uuid,
    metadata      jsonb NOT NULL DEFAULT '{}',
    timestamp     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_audit_squad ON audit_log(squad_id, timestamp);
CREATE INDEX IF NOT EXISTS idx_audit_actor ON audit_log(actor_id, timestamp);

-- ---------------------------------------------------------------------------
-- Kubernetes outbox (durable control-plane -> operator CR intents)
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS kubernetes_outbox (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    aggregate_type text NOT NULL CHECK (aggregate_type IN ('squad','agent')),
    aggregate_id   uuid NOT NULL,
    operation      text NOT NULL CHECK (operation IN
                    ('upsert_squad','delete_squad','upsert_agent','delete_agent')),
    payload        jsonb NOT NULL DEFAULT '{}',
    status         text NOT NULL DEFAULT 'pending'
                   CHECK (status IN ('pending','applied','failed')),
    attempts       integer NOT NULL DEFAULT 0,
    last_error     text NOT NULL DEFAULT '',
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    locked_until   timestamptz,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_kubernetes_outbox_ready
    ON kubernetes_outbox(status, next_attempt_at, created_at)
    WHERE status IN ('pending','failed');
CREATE INDEX IF NOT EXISTS idx_kubernetes_outbox_aggregate
    ON kubernetes_outbox(aggregate_type, aggregate_id, created_at);

-- ---------------------------------------------------------------------------
-- Agent long-term memory (semantic memory via pgvector)
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS agent_memory (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id       uuid NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    squad_id       uuid REFERENCES squads(id) ON DELETE CASCADE,
    content        text NOT NULL,
    embedding      vector(1536),
    source_task_id uuid REFERENCES tasks(id) ON DELETE SET NULL,
    metadata       jsonb NOT NULL DEFAULT '{}',
    created_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_memory_agent ON agent_memory(agent_id);
CREATE INDEX IF NOT EXISTS idx_memory_squad ON agent_memory(squad_id) WHERE squad_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_memory_embedding ON agent_memory
    USING ivfflat (embedding vector_cosine_ops);
