# skquad — Identity, AuthN & RBAC Design

> **Status:** Draft v1 · **Decision:** [ADR-0007](adr/0007-agent-identity.md)
>
> skquad has **two kinds of principals**: **human users** (OIDC) and **agents**
> (owner-created identities). Authorization is **two-layer RBAC**: **user RBAC**
> (managed by the platform admin) and **agent permissions** (managed by the
> squad owner). All significant actions are **audited**.

---

## 1. Principals

| Principal | Identity | Credential | Created by |
|-----------|----------|------------|------------|
| **Human user** | OIDC subject (email) | OIDC session / JWT | OIDC IdP (first login) or platform admin |
| **Agent** | `AgentIdentity` (owner-created) | K8s secret (in squad namespace) + LLM gateway virtual key | Squad owner (platform-facilitated) |

---

## 2. Human Identity & AuthN (OIDC)

- Users authenticate via **OIDC** against a configured IdP (Keycloak, Okta,
  Azure AD, Google, …).
- On first login, the **API server** provisions a `User` record (email, display
  name) and assigns the default role (`user`).
- The API server issues a short-lived **JWT** (or session) for subsequent
  requests. The JWT carries the user's **id** and **role(s)**.
- The API server validates the JWT on every request (authN) and then applies
  **user RBAC** (authZ).

```
Browser → IdP (OIDC login) → callback → API Server issues JWT
Browser → API Server (Bearer JWT) → authN (validate JWT) → authZ (RBAC)
```

---

## 3. User RBAC (Layer 1 — platform admin managed)

Roles for **human users**, managed by the **platform administrator**:

| Role | Capabilities |
|------|--------------|
| **platform_admin** | Manage users/roles, register **registry resources** (LLM providers, skills, tools, APIs, KBs, workspaces), configure the platform, view all squads/metering, manage the LLM gateway. |
| **user** | Create/own squads, add agents, manage their own squads (board, tasks, agent permissions, access grants), view their own metering. |

- Role assignment is a **platform-admin** action.
- A `user` can only act on **squads they own** (or have been granted access to).
- Authorization checks are enforced **centrally in the API server** on every
  request.

---

## 4. Agent Identity (owner-created)

- An **`AgentIdentity`** is a first-class entity, **created by the squad owner**
  (platform-facilitated via a one-click action — see ADR-0007).
- Creation generates:
  - An **identity** (a stable agent principal, e.g. a subject id).
  - A **credential** stored as a **K8s secret** in the squad namespace.
  - A **virtual key** for the **LLM gateway** (so the agent's LLM calls are
    attributable and permission-checked).
- The **owner owns** the identity: they can **rotate** the credential, **replace**
  it, or **delete** it (which revokes the agent's access).
- The agent's **permissions** (which registry resources it may use) are set
  **independently** of the identity (see §5).

```
Squad owner → "Create agent identity" (one-click)
  → platform generates AgentIdentity + credential (K8s secret) + virtual key
  → attached to the Agent
  → owner can rotate / replace / delete
```

---

## 5. Agent Permissions (Layer 2 — squad owner managed)

- Each agent has a **permission set**: which **registry resources** it may use.
- Managed by the **squad owner** (via the API / web app).
- Permission set includes:
  - **LLM providers / models** the agent may call (enforced at the LLM gateway).
  - **Tools** and **skills** the agent may load (enforced by the agent runtime's
    plugin loader).
  - **Knowledge bases** the agent may query (enforced by the RAG connector).
  - **Project workspaces** (git/Jira/Confluence) the agent may access (enforced
    by the resource connectors).
- Enforcement points:
  - **LLM gateway** — provider/model permissions.
  - **Agent runtime** — plugin loading (only permitted plugins).
  - **Resource connectors** — workspace/KB access.
- Changing an agent's permissions takes effect on the next authorization check
  (and, for the LLM gateway, on the next call).

---

## 6. Access Grants (cross-user / cross-squad)

- A **squad owner** can **grant** another **user** (or another squad's **agent**)
  the right to **talk to** the squad's agents.
- An **`AccessGrant`** records: `squad_id`, `grantee_type` (user/agent),
  `grantee_id`, `permissions` (e.g. `talk_to_agents`, `add_task`, `ping`),
  `granted_by`, `created_at`.
- **Enforcement:**
  - **User → agent chat:** the API server checks the grant before allowing a
    user to message an agent they don't own.
  - **Agent → agent (cross-squad):** the control plane checks the grant before
    enqueuing a cross-squad message or allowing a task to be added to another
    squad's board.
- Grants are **revocable**; revocation takes effect on the next check.

---

## 7. Credential Management

- **Human:** OIDC session / JWT (short-lived). No long-lived secrets on the
  client.
- **Agent:**
  - **Credential** — K8s secret in the squad namespace (scoped to the agent).
  - **Virtual key** — for the LLM gateway (attributable, revocable).
- **Rotation:** the owner can rotate an agent's credential / virtual key without
  re-creating the agent. The old key is revoked on rotation.
- **Storage:** secrets live in K8s (optionally backed by an external secrets
  manager). The control plane stores only a one-way verifier hash for the agent
  credential and stores references for both the agent credential and LLM gateway
  virtual key. It never returns raw secrets to clients.
- **Least privilege:** each agent's credential is scoped to that agent; the
  LLM gateway virtual key is scoped to the agent's permitted models.

---

## 8. Audit Logging

- An **append-only audit log** (Postgres) records **who did what**:
  - **User actions:** login, squad/agent create/delete, permission changes,
    access grants, task create/assign, registry changes, metering views.
  - **Agent actions:** task start/complete, LLM calls (via gateway), memory
    writes, messages sent, resource access.
- Each audit row: `actor_type` (user/agent), `actor_id`, `action`,
  `resource_type`, `resource_id`, `squad_id`, `metadata` (JSON), `timestamp`.
- The audit log is **retained** even after a squad/agent is deleted.
- The **platform admin** can query the audit log (for compliance / forensics).

```
audit_log(
  id, actor_type, actor_id, action,
  resource_type, resource_id, squad_id,
  metadata jsonb, timestamp
)
```

---

## 9. Enforcement Summary

| Check | Where | Enforced by |
|-------|-------|-------------|
| Human authN | API server | OIDC JWT validation |
| User RBAC | API server | Role + ownership checks |
| Agent → provider/model | LLM gateway | Agent permission set |
| Agent → plugin (tool/skill) | Agent runtime | Plugin loader (permissions) |
| Agent → KB / workspace | Resource connectors | Agent permission set |
| User → agent (chat) | API server | Access grant |
| Agent → agent (cross-squad) | Control plane / message queue | Access grant |
| All significant actions | Control plane | Audit log |

Current implementation note: when Kubernetes CR writing is enabled, the control
plane writes the generated agent credential and LLM gateway virtual key into
separate Kubernetes Secrets named by `credential_ref` and `virtual_key_ref`.
The Agent CR update that exposes those Secret names is delivered through the
Kubernetes outbox, and the operator mounts both Secrets into the agent pod once
the CR converges. This requires the API server service account to have Secret
write/delete access for squad namespaces. Raw token material is not written to
the outbox.

---

## 10. Relationship to Other Components

- **API server** — authN (OIDC) + user RBAC + access-grant enforcement + audit.
- **LLM gateway** — agent provider/model permission enforcement + metering.
- **Agent runtime** — plugin loading per permissions.
- **Resource connectors** — KB/workspace access per permissions.
- **Postgres** — users, roles, agent identities, permissions, access grants,
  audit log.

---

## 11. Open Points

- **IdP choice** — which OIDC provider(s) to support out of the box (Keycloak
  as the default open-source option; Okta/Azure AD/Google via standard OIDC).
- **External secrets manager** — whether to integrate (e.g. Vault,
  ExternalSecrets) for agent credentials (later).
- **Session vs JWT** — confirm token strategy (stateless JWT recommended).
- **MFA / conditional access** — delegated to the IdP (out of scope for skquad
  core).

See [security-threat-model.md](security-threat-model.md) for the threat model
and mitigations.
