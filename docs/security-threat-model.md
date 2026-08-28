# skquad — Security & Threat Model

> **Status:** Draft v1
>
> skquad is an **enterprise, multi-tenant** platform. This document identifies
> the **assets**, **trust boundaries**, **threats**, and **mitigations**. It
> complements [identity-security.md](identity-security.md) (the controls) and
> [deployment-operator.md](deployment-operator.md) (isolation).
>
> This is the target threat model, not a production certification. Open
> implementation gaps are tracked in
> [`implementation-status.md`](implementation-status.md).

---

## 1. Assets

| Asset | Why it matters |
|-------|----------------|
| **User data** (squads, tasks, chat) | Confidential business content. |
| **Agent credentials** (identity, virtual keys) | Grant access to LLMs + resources. |
| **LLM provider keys** (BYOM) | Cost + access to external models. |
| **Squad isolation** | One squad must not read/control another. |
| **Metering integrity** | Cost accounting must be accurate. |
| **Audit log** | Compliance / forensics; must be tamper-evident. |
| **Registry** | Governed catalog; must not be tampered with. |

---

## 2. Trust Boundaries

```
┌─────────────────────────────────────────────────────────────┐
│  Untrusted: Browser / external clients                       │
├─────────────────────────────────────────────────────────────┤
│  Control plane (skquad-system): API, gateway, operator       │  ← enforcement
│  (authN, RBAC, access grants, metering, audit)              │
├─────────────────────────────────────────────────────────────┤
│  Data plane: squad namespaces (agent pods)                   │  ← isolated
│  (each squad isolated; agents have scoped credentials)       │
├─────────────────────────────────────────────────────────────┤
│  External: LLM providers, KBs, git/Jira/Confluence           │  ← untrusted I/O
└─────────────────────────────────────────────────────────────┘
```

- **Control plane** is the **enforcement point** (authN, RBAC, access grants,
  metering, audit).
- **Squad namespaces** are **isolated** (network policies; no direct
  pod-to-pod between squads).
- **External systems** (LLM providers, KBs, workspaces) are **untrusted I/O** —
  their content can be adversarial (prompt injection).

---

## 3. Threats & Mitigations

### T1. Multi-tenant isolation bypass
- **Threat:** Squad A reads or controls squad B's agents/tasks/data.
- **Mitigations:**
  - Squad = **namespace**; **network policies** default-deny between squads.
  - Squad namespace egress is limited to DNS plus API server / LLM gateway pods
    selected in the control-plane namespace; direct Postgres egress is not part
    of the default agent policy.
  - All cross-squad interaction goes through the **control plane**, which
    enforces **access grants**.
  - API layer always filters by `squad_id` + ownership (optionally Postgres
    RLS).
  - Agents have **scoped credentials** (only their squad's resources).

### T2. Agent credential theft / leak
- **Threat:** An agent's credential or virtual key is stolen and abused.
- **Mitigations:**
  - Credentials stored as **K8s secrets** (scoped per agent/squad); never
    returned to clients.
  - The API server gets generated Secret write/delete authority through
    operator-created RoleBindings in each managed squad namespace, not through a
    chart-level cluster-wide Secret writer role.
  - **Virtual keys** are per-agent, revocable, and rate-limited.
  - **Rotation** without re-creating the agent; old keys revoked on rotation.
  - **Least privilege** — a key only grants the agent's permitted models.
  - **Audit** all credential use; alert on anomalies.

### T3. Prompt injection (via untrusted content)
- **Threat:** Adversarial content in a **task description**, **knowledge base**,
  **workspace**, or **message** manipulates an agent into harmful actions
  (exfiltration, unauthorized calls).
- **Mitigations:**
  - **Least privilege** — an agent can only use **permitted** resources; a
    prompt cannot grant new access.
  - **Egress controls** — network policies restrict where agents can send data
    (only permitted endpoints).
  - **Tool allow-lists** — agents can only invoke permitted tools/skills.
  - **Audit** all agent actions (LLM calls, tool calls, messages) for review.
  - **Content provenance** — task-context memory is labeled with trust level,
    provenance, review status, and source task. Raw completion summaries remain
    `raw_model_output` / `pending_review`; runtime prompts tell the model that
    memory is contextual evidence, not executable instruction.
  - Injection detection / guardrails at the gateway remain later work.
  - **Human review** — sensitive actions (e.g. cross-squad handoff, external
    writes) can require approval (later).

### T4. LLM gateway abuse
- **Threat:** An agent (or compromised agent) makes excessive or unauthorized
  LLM calls (cost abuse, using models it shouldn't).
- **Mitigations:**
  - **Permission enforcement** at the gateway (agent can only use permitted
    providers/models).
  - **Rate limiting** + **budgets** per virtual key (per agent/squad).
  - **Metering** — all calls recorded; cost visible; alert on spend anomalies.
  - **Virtual keys** are revocable.

### T5. RBAC bypass (user)
- **Threat:** A user accesses a squad they don't own / aren't granted.
- **Mitigations:**
  - **Central authZ** in the API server on every request (role + ownership +
    access grants).
  - **JWT** carries identity + role; validated on every call.
  - **Deny by default** — no access without an explicit grant.
  - **Audit** all access attempts (including denials).

### T6. Access-grant abuse (cross-squad)
- **Threat:** A granted agent/user abuses cross-squad access.
- **Mitigations:**
  - Grants are **explicit, scoped** (specific permissions), and **revocable**.
  - Cross-squad messages/task creation are **enforced + audited**.
  - Owners can **revoke** grants; revocation takes effect on the next check.
  - **Least privilege** — grant only what's needed (e.g. `read`, `talk`,
    `ping`, or `add_task`).

### T7. Supply chain (malicious plugin)
- **Threat:** A malicious or compromised **plugin** (skill/tool/connector)
  exfiltrates data or misbehaves.
- **Mitigations:**
  - Plugins run **inside the agent pod** (isolated per squad namespace).
  - **Sandboxing** for untrusted/community plugins (restricted sidecar).
  - **Manifest-declared permissions** — a plugin only gets what the agent is
    permitted.
  - **Provenance** — plugin packages signed / checksummed (later); registry
    tracks source.
  - **Audit** plugin invocations.

### T8. Data exfiltration (agent → external)
- **Threat:** An agent sends confidential data to an external endpoint.
- **Mitigations:**
  - **Egress network policies** — agents can only reach **permitted** endpoints
    (LLM gateway, queue, granted resources).
  - **Tool allow-lists** — only permitted tools can make external calls.
  - **Audit** all external calls.
  - (Later) **DLP** / content inspection at the gateway for sensitive data.

### T9. Audit log tampering
- **Threat:** An attacker modifies/deletes audit records to hide actions.
- **Mitigations:**
  - Audit log is **append-only** (no update/delete for normal roles).
  - **DB roles** — only the control plane can write; admins can read.
  - (Later) **hash chaining** / external logging for tamper-evidence.
  - **Retention** — audit retained even after squad/agent deletion.

### T10. Denial of service
- **Threat:** Excessive tasks/agents/calls degrade the platform.
- **Mitigations:**
  - **Resource quotas** per squad namespace.
  - **Rate limiting** (API + gateway).
  - **Scale-to-zero** — idle agents consume no resources.
  - **Backpressure** — message queue + task backlog bounds concurrent work.
  - **Alerts** on resource saturation.

---

## 4. Defense in Depth

| Layer | Control |
|-------|---------|
| **Network** | Namespace isolation, egress policies, default-deny. |
| **Identity** | OIDC (users), owner-created agent identities, virtual keys. |
| **Authorization** | Two-layer RBAC (user + agent), access grants, deny-by-default. |
| **Data** | Scoped credentials, no raw secrets in DB, RLS (optional). |
| **Runtime** | Plugin sandboxing, tool allow-lists, least privilege. |
| **Accountability** | Append-only audit, metering, alerts. |
| **Resilience** | Quotas, rate limits, scale-to-zero, backpressure. |

---

## 5. Residual Risks

- **Prompt injection** cannot be fully eliminated with LLMs; mitigations reduce
  blast radius (least privilege, egress controls, audit) but a determined
  attacker with a permitted tool could still cause harm within that scope.
  **Mitigation:** keep scopes narrow; require human approval for sensitive
  actions (later).
- **Compromised control plane** — if the API server is compromised, enforcement
  is bypassed. **Mitigation:** harden the control plane, least-privilege DB
  roles, audit, and (later) external secrets manager + mTLS.
- **Plugin supply chain** — untrusted plugins are a risk; **mitigation:**
  sandboxing + provenance + signing (later).

---

## 6. Security Requirements Traceability

| Requirement | Covered by |
|-------------|------------|
| OIDC for humans | [identity-security.md](identity-security.md) §2 |
| Two-layer RBAC | [identity-security.md](identity-security.md) §3, §5 |
| Audit logging | [identity-security.md](identity-security.md) §8 |
| Multi-tenant isolation | [deployment-operator.md](deployment-operator.md) §8 |
| Agent identity/credentials | [identity-security.md](identity-security.md) §4, §7 |
| Metering integrity | [observability-metering.md](observability-metering.md) §2 |
| BYOM credential safety | [resource-registry.md](resource-registry.md) §7 |

---

## 7. Open Points

- **External secrets manager** (Vault / ExternalSecrets) for agent credentials
  (later).
- **mTLS** between control plane and agent pods (hardening).
- **Prompt-injection guardrails** at the gateway (later).
- **Human-approval gates** for sensitive actions (later).
- **Penetration test** before GA (implementation).
