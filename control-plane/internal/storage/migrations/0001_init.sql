-- skquad control-plane initial schema.
-- Mirrors docs/data-model.md. Secrets are never stored here — only *_ref
-- (K8s secret references).
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- ---------------------------------------------------------------------------
-- Users (Layer-1 RBAC principals)
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS users (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email       text NOT NULL UNIQUE,
    name        text NOT NULL DEFAULT '',
    role        text NOT NULL DEFAULT 'user'
                CHECK (role IN ('platform_admin','user')),
    created_at  timestamptz NOT NULL DEFAULT now()
);

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
    permissions        jsonb NOT NULL DEFAULT '[]',
    idle_timeout_sec   integer NOT NULL DEFAULT 300,
    status             text NOT NULL DEFAULT 'idle'
                       CHECK (status IN ('idle','busy','error')),
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now(),
    UNIQUE (squad_id, name)
);
CREATE INDEX IF NOT EXISTS idx_agents_squad ON agents(squad_id);

-- ---------------------------------------------------------------------------
-- Agent identities (owner-created; credential + virtual-key refs)
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS agent_identities (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id       uuid NOT NULL UNIQUE REFERENCES agents(id) ON DELETE CASCADE,
    credential_ref text NOT NULL,
    virtual_key_ref text,
    created_by     uuid NOT NULL REFERENCES users(id),
    created_at     timestamptz NOT NULL DEFAULT now(),
    rotated_at     timestamptz
);

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
-- Registry: LLM providers (BYOM)
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS llm_providers (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name          text NOT NULL UNIQUE,
    kind          text NOT NULL,
    base_url      text NOT NULL,
    api_key_ref   text NOT NULL,
    models        jsonb NOT NULL DEFAULT '[]',
    pricing       jsonb NOT NULL DEFAULT '{}',
    status        text NOT NULL DEFAULT 'active'
                  CHECK (status IN ('active','deprecated')),
    registered_by uuid NOT NULL REFERENCES users(id),
    created_at    timestamptz NOT NULL DEFAULT now()
);

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
-- Metering (token usage; partition by month in production)
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS metering (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id      uuid NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    squad_id      uuid NOT NULL REFERENCES squads(id) ON DELETE CASCADE,
    provider_id   uuid,
    model         text NOT NULL DEFAULT '',
    input_tokens  integer NOT NULL DEFAULT 0,
    output_tokens integer NOT NULL DEFAULT 0,
    cost          double precision NOT NULL DEFAULT 0,
    currency      text NOT NULL DEFAULT 'USD',
    timestamp     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_metering_squad ON metering(squad_id, timestamp);
CREATE INDEX IF NOT EXISTS idx_metering_agent ON metering(agent_id, timestamp);

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
