# skquad — Web App & UX Design

> **Status:** Draft v1; first-pass squad, agent, task, registry, grant, admin,
> identity, and chat workflows are implemented in the current web app.
>
> The web app is a **SPA** (React / Next.js) — the primary interface for users.
> It is optimised for **simplicity**: the onboarding path is a few clicks, and
> the **Kanban board** is the centre of daily use.

---

## 1. UX Principles

- **Minutes to first agent** — the onboarding flow is the north star; keep it
  to a handful of clicks.
- **Board-first** — the Kanban board is the primary surface; everything else
  supports it.
- **Role-aware** — the UI adapts to the user's role (platform admin vs. user)
  and ownership (owner vs. granted).
- **Visible progress** — users can always see what each agent is doing and the
  squad's overall state.
- **Safe by default** — destructive actions (delete squad/agent, revoke grants)
  are confirmed and audited.

---

## 2. Onboarding — the "Minutes" Flow

```
1. Login (OIDC)
2. "Create a squad" → name + mission
3. "Add an agent" → name + role
   → one-click "Create agent identity"
4. "Pick an LLM provider" → choose a predefined provider
5. Done → the squad board is ready
```

- Each step is a single screen with sensible defaults.
- The **predefined LLM provider** makes step 4 trivial (no key entry).
- A **progress indicator** shows how close the user is to "done."
- After onboarding, the user lands on the **squad board**.

---

## 3. Layout & Navigation

```
┌──────────────────────────────────────────────────────────┐
│  skquad   [Squads ▾]   [Registry]   [Metering]   [⚙] [👤] │
├──────────────────────────────────────────────────────────┤
│                                                          │
│   Squad: <name>          [Mission] [Operating Model]     │
│   ┌──────────────────────────────────────────────────┐   │
│   │  TODO      │  IN-PROGRESS  │  IN-REVIEW  │  DONE  │   │
│   │  ┌───────┐ │  ┌───────┐    │  ┌───────┐   │       │   │
│   │  │ task  │ │  │ task  │    │  │ task  │   │       │   │
│   │  └───────┘ │  └───────┘    │  └───────┘   │       │   │
│   └──────────────────────────────────────────────────┘   │
│                                                          │
│   Agents: [🤖 coder] [🤖 reviewer]   [+ Add agent]       │
└──────────────────────────────────────────────────────────┘
```

- **Top nav:** Squads, Registry (admin), Metering, Settings, User menu.
- **Squad view:** the board (centre), squad details (mission, operating model),
  and the agent list.
- **Role-aware:** platform admins see Registry + platform-wide Metering/Audit;
  users see their squads.

---

## 4. Key Screens

### 4.1 Squad Board (primary)
- **Columns:** TODO, IN-PROGRESS, IN-REVIEW, DONE, BLOCKED.
- **Cards:** task title, assignee agent, status, age.
- **Interactions:** create task, drag between columns, assign to an agent, open
  task detail.
- **Task detail:** description, metadata, activity (who moved it when), assignee,
  (for agent-created tasks) the creating agent.
- **Live updates:** columns update as agents work (via polling or WebSocket).

Current implementation: users can create tasks, move them between statuses,
assign/reassign agents, and delete tasks. Rich task detail, activity, drag and
drop, and live updates remain follow-up work.

### 4.2 Agent Panel
- **List of agents** in the squad (name, role, state: idle/busy).
- **Agent detail:**
  - Role, default LLM provider/model.
  - **Identity** — status, "Create identity" / "Rotate" (owner).
  - **Permissions** — which registry resources the agent may use (grant/revoke).
  - **Metering** — tokens + cost for this agent.
- **Chat** — open a 1:1 chat with the agent.

Current implementation: users can create agents, create/rotate their identities,
select an agent, view queued chat history, enqueue consult messages, and manage
the selected agent's registry resource permissions. Per-agent metering panels
remain follow-up work.

### 4.3 Chat (secondary)
- A 1:1 conversation with an agent (ad-hoc questions / steering).
- Distinct from tasks — does not reset the task context.
- Available to the owner + granted users.

### 4.4 Squad Settings
- **Mission** — what the squad is for.
- **Operating model** — the role of each agent + how they collaborate (editable
  structured form).
- **Access grants** — grant/revoke other users (or other squads' agents) access
  to talk to the squad's agents.

### 4.5 Metering
- **Per squad** — aggregate tokens + cost over time (charts).
- **Per agent** — tokens + cost.
- **Per provider/model** — breakdown.
- **Platform-wide** (admin) — across all squads.
- Cost shown only where the provider has pricing configured.

### 4.6 Registry (platform admin)
- **Tabs** by resource type: LLM Providers, Skills, Tools, APIs, Knowledge
  Bases, Project Workspaces.
- **Register** a resource (definition + credential ref).
- **Deprecate** a resource.
- For LLM providers: models + per-token pricing.

Current implementation: platform admins can register and deprecate LLM providers
and generic registry resources, and squad owners can grant/revoke selected
registry resources to the selected agent.

### 4.7 Audit (admin / owner)
- Queryable log: filter by actor, squad, action, time range.
- Shows who did what (user + agent actions).

Current implementation: the admin screen loads platform audit and metering
summary endpoints when the current user has access.

### 4.8 Admin / Settings (platform admin)
- Platform config (OIDC, defaults, idle timeout, observability toggle).
- User management (roles, activate/deactivate).
- Platform health.

---

## 5. Role-Aware Views

| Screen | platform_admin | squad owner | granted user |
|--------|:--------------:|:-----------:|:------------:|
| Squad board | ✅ | ✅ (own) | ✅ (granted) |
| Agent panel (identity/permissions) | ✅ | ✅ (own) | — |
| Squad settings (mission/operating model) | ✅ | ✅ (own) | — |
| Access grants | ✅ | ✅ (own) | — |
| Chat with agent | ✅ | ✅ (own) | ✅ (granted) |
| Metering (squad) | ✅ | ✅ (own) | — |
| Registry | ✅ | — | — |
| Audit | ✅ (all) | ✅ (own) | — |
| User management | ✅ | — | — |

---

## 6. Real-Time Updates

- **Board updates** as agents move tasks — via **WebSocket** (preferred) or
  short **polling** (fallback).
- **Agent state** (idle/busy) updates live.
- **Chat** messages stream in real time.
- The backend exposes a subscription endpoint (or SSE) for live updates.

---

## 7. Accessibility & Polish

- Keyboard navigation for the board (create/assign/move tasks).
- Clear empty states ("No tasks yet — create one").
- Confirmation dialogs for destructive actions.
- Loading + error states for all async operations.
- Responsive layout (desktop-first; usable on tablet).

---

## 8. Tech Notes

- **Framework:** React / Next.js (TypeScript).
- **State:** client state (e.g. TanStack Query for server state + a store for UI).
- **Board:** a Kanban component (e.g. dnd-kit for drag-and-drop).
- **Auth:** OIDC redirect flow; store the JWT securely; attach to API calls.
- **Real-time:** WebSocket / SSE client for board + chat updates.
- **Charts:** a charting lib (e.g. Recharts) for metering.

---

## 9. Open Points

- **Multi-squad dashboard** — an overview across all of a user's squads (later).
- **Task templates** — quick-create from templates (later).
- **Notifications** — in-app + (optional) external notifications (later).
- **Theming / branding** — for self-hosted deployments (later).
- **i18n** — internationalisation (later).
