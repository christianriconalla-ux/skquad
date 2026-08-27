# skquad — Control Plane API Design

> **Status:** Draft v1
>
> The **API server** exposes a **REST API** for the web app and external
> clients. It is the single entry point: it performs **authN** (OIDC JWT),
> **authZ** (user RBAC + access grants), and all domain CRUD. It also creates
> the `Squad`/`Agent` CRs the operator reconciles.

---

## 1. Conventions

- **Base URL:** `/api/v1`
- **AuthN:** `Authorization: Bearer <JWT>` (OIDC-issued).
- **AuthZ:** enforced per endpoint (role + ownership + access grants).
- **Format:** JSON (request + response).
- **Errors:** consistent error envelope (see §10).
- **Pagination:** `?cursor=` + `?limit=` (default limit 50).
- **Idempotency:** `Idempotency-Key` header for mutating endpoints (create/assign).
- **Versioning:** URI versioning (`/api/v1`); breaking changes bump the version.

---

## 2. Auth

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/auth/login` | Redirect to the OIDC IdP. |
| `GET` | `/api/v1/auth/callback` | OIDC callback; issues a JWT. |
| `GET` | `/api/v1/auth/me` | Current user (id, email, role). |
| `POST` | `/api/v1/auth/logout` | Invalidate the session. |

---

## 3. Users (platform admin)

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/users` | List users (admin). |
| `GET` | `/api/v1/users/:id` | Get a user (admin). |
| `PATCH` | `/api/v1/users/:id/role` | Set a user's role (admin). |
| `PATCH` | `/api/v1/users/:id/status` | Activate/deactivate (admin). |

---

## 4. Squads

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/v1/squads` | Create a squad (owner = caller). Creates the `Squad` CR. |
| `GET` | `/api/v1/squads` | List squads the caller owns / has access to. |
| `GET` | `/api/v1/squads/:id` | Get a squad (owner or granted). |
| `PATCH` | `/api/v1/squads/:id` | Update name / mission / operating model (owner). |
| `DELETE` | `/api/v1/squads/:id` | Delete a squad (owner). Deletes the `Squad` CR (cascades). |
| `POST` | `/api/v1/squads/:id/archive` | Archive a squad (owner). |

---

## 5. Agents

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/v1/squads/:id/agents` | Create an agent in a squad (owner). Creates the `Agent` CR. |
| `GET` | `/api/v1/squads/:id/agents` | List agents in a squad. |
| `GET` | `/api/v1/agents/:id` | Get an agent (owner or granted). |
| `PATCH` | `/api/v1/agents/:id` | Update role / default model / idle timeout (owner). |
| `DELETE` | `/api/v1/agents/:id` | Delete an agent (owner). Deletes the `Agent` CR. |
| `POST` | `/api/v1/agents/:id/identity` | **Create the agent identity** (one-click; owner). |
| `POST` | `/api/v1/agents/:id/identity/rotate` | Rotate the agent credential / virtual key (owner). |
| `GET` | `/api/v1/agents/:id/permissions` | List the agent's resource permissions. |
| `PUT` | `/api/v1/agents/:id/permissions` | Set the agent's resource permissions (owner). |

Identity responses expose credential and virtual-key references only. Raw agent
credential material is written to the configured Secret backend and is not
returned by the API.

---

## 6. Boards & Tasks

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/squads/:id/board` | Get the squad's board (columns + tasks). |
| `POST` | `/api/v1/squads/:id/board/tasks` | Create a task on the board. |
| `GET` | `/api/v1/tasks/:id` | Get a task. |
| `PATCH` | `/api/v1/tasks/:id` | Update title / description / metadata. |
| `POST` | `/api/v1/tasks/:id/assign` | Assign a task to an agent (signals scale-up). |
| `POST` | `/api/v1/tasks/:id/move` | Move a task to a column (`{ status }`). |
| `DELETE` | `/api/v1/tasks/:id` | Delete a task. |

> **Agent-facing task endpoints** (used by the agent runtime, authenticated by
> the agent's identity):
>
> | Method | Path | Description |
> |--------|------|-------------|
> | `GET` | `/api/v1/agents/me/tasks` | Tasks assigned to this agent. |
> | `GET` | `/api/v1/agents/me/resources` | Active registry resources granted to this agent; secret refs are not returned. |
> | `POST` | `/api/v1/agents/me/tasks/claim` | Idempotently claim the current `in-progress` task or the next assigned `todo` task. |
> | `POST` | `/api/v1/agents/me/tasks/:id/start` | Mark a task `in-progress` (context reset). |
> | `POST` | `/api/v1/agents/me/tasks/:id/complete` | Mark a task done / in-review. |
> | `POST` | `/api/v1/agents/me/tasks/:id/block` | Mark a task `blocked`. |
> | `POST` | `/api/v1/agents/me/heartbeat` | Report `idle`, `busy`, or `error`. |

---

## 7. Chat (user ↔ agent)

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/v1/agents/:id/chat` | Send a message to an agent (owner or granted). |
| `GET` | `/api/v1/agents/:id/chat` | Get the chat history with an agent. |

> Chat is a **lightweight, non-task** interaction. It does not reset the task
> context. Enforced by access grants for non-owners.

---

## 8. Messaging (agent ↔ agent)

> Agent-facing (authenticated by the agent's identity). The control plane
> enforces **access grants** for cross-squad messages.

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/v1/agents/me/messages` | Send a message (`{ to_id, type, payload }`). |
| `GET` | `/api/v1/agents/me/messages` | Get this agent's inbox (pending). |
| `POST` | `/api/v1/agents/me/messages/:id/ack` | Acknowledge a delivered message. |

- **Cross-squad** messages are rejected (and audited) without an access grant.
- **Delegation / handoff** create a task on the target board + a ping.

---

## 9. Resource Registry

> **Registration** is platform-admin only. **Granting** is squad-owner only.

### 9.1 LLM Providers
| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/v1/registry/llm-providers` | Register a provider (admin). |
| `GET` | `/api/v1/registry/llm-providers` | List providers. |
| `GET` | `/api/v1/registry/llm-providers/:id` | Get a provider. |
| `PATCH` | `/api/v1/registry/llm-providers/:id` | Update (admin). |
| `POST` | `/api/v1/registry/llm-providers/:id/deprecate` | Deprecate (admin). |

### 9.2 Skills / Tools / APIs / Knowledge Bases / Workspaces
The same CRUD pattern applies to each type:

| Type | Base path |
|------|-----------|
| Skills | `/api/v1/registry/skills` |
| Tools | `/api/v1/registry/tools` |
| APIs | `/api/v1/registry/apis` |
| Knowledge bases | `/api/v1/registry/knowledge-bases` |
| Project workspaces | `/api/v1/registry/project-workspaces` |

Each supports `POST` (register, admin), `GET` (list/get), `PATCH` (admin),
`POST /:id/deprecate` (admin).

---

## 10. Access Grants

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/v1/squads/:id/access-grants` | Grant a user/agent access (owner). |
| `GET` | `/api/v1/squads/:id/access-grants` | List grants for a squad. |
| `DELETE` | `/api/v1/access-grants/:id` | Revoke a grant (owner). |

---

## 11. Metering

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/squads/:id/metering` | Squad metering (tokens + cost, time-range). |
| `GET` | `/api/v1/agents/:id/metering` | Agent metering (tokens + cost, time-range). |
| `GET` | `/api/v1/metering/summary` | Platform-wide summary (admin). |

Query params: `?from=`, `?to=`, `?groupBy=agent|squad|provider|model`.

---

## 12. Audit

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/audit` | Query the audit log (admin; filters by actor, squad, action, time). |
| `GET` | `/api/v1/squads/:id/audit` | Audit log for a squad (owner/admin). |

---

## 13. Admin (platform config)

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/admin/config` | Get platform config (admin). |
| `PATCH` | `/api/v1/admin/config` | Update platform config (admin). |
| `GET` | `/api/v1/admin/health` | Platform health (all components). |

---

## 14. Error Model

```json
{
  "error": {
    "code": "forbidden",
    "message": "You do not have access to this squad.",
    "details": { "squad_id": "..." }
  }
}
```

| HTTP | Code | Meaning |
|------|------|---------|
| 400 | `bad_request` | Invalid input. |
| 401 | `unauthorized` | Missing/invalid JWT. |
| 403 | `forbidden` | Authenticated but not allowed (RBAC / access grant). |
| 404 | `not_found` | Resource does not exist. |
| 409 | `conflict` | Version conflict / duplicate. |
| 422 | `unprocessable` | Semantically invalid. |
| 429 | `rate_limited` | Too many requests. |
| 500 | `internal` | Unexpected error. |

---

## 15. AuthZ Matrix (summary)

| Action | platform_admin | squad owner | granted user | agent |
|--------|:--------------:|:-----------:|:------------:|:-----:|
| Manage users / registry | ✅ | — | — | — |
| Create / delete squad | ✅ | ✅ (own) | — | — |
| Manage squad agents | ✅ | ✅ (own) | — | — |
| Create / assign tasks | ✅ | ✅ (own) | ✅ (granted) | ✅ (per grants) |
| Chat with agent | ✅ | ✅ (own) | ✅ (granted) | — |
| Cross-squad message | ✅ | ✅ (per grants) | — | ✅ (per grants) |
| View metering | ✅ (all) | ✅ (own) | — | — |
| View audit | ✅ (all) | ✅ (own) | — | — |

---

## 16. Open Points

- **Webhooks** — notify external systems on task/agent events (later).
- **GraphQL** — whether to add a GraphQL layer (start REST).
- **Rate limiting** — per-user and per-agent limits (the gateway handles LLM
  calls; the API server handles API calls).
- **OpenAPI spec** — generate and publish an OpenAPI 3 document (implementation).
