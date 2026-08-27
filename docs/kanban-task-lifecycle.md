# skquad — Kanban Board & Task Lifecycle Design

> **Status:** Draft v1
>
> Each squad has a **Kanban board** — the **primary work surface** for the
> squad. **Tasks** are the unit of work. Agents **pick up tasks** assigned to
> them; their **working context resets before each new task**.
>
> Basic task assignment, claim, status, runtime context, execution leases, and
> fenced terminal updates are implemented. Remaining hardening is tracked in
> [`implementation-status.md`](implementation-status.md).

---

## 1. Board

- **One board per squad** (1:1).
- The board is the **primary way a user interacts with the squad**: create /
  assign tasks, watch progress.
- Columns (states):

| Column | Meaning |
|--------|---------|
| `todo` | Backlog / ready to be assigned. |
| `in-progress` | An agent is actively working on it. |
| `in-review` | Work done, awaiting review (by an agent or user). |
| `done` | Complete. |
| `blocked` | Cannot proceed (dependency, missing access, error). |

- The board is **owned by the squad** and isolated in the squad namespace.
- Users with access (owner + granted users) can view and manage the board.

---

## 2. Task Model

```
task(
  id,
  board_id,          # the squad's board
  title,
  description,       # what to do, acceptance criteria, context
  status,            # todo | in-progress | in-review | done | blocked
  assignee_agent_id, # the agent working on it (nullable)
  created_by,        # user id OR agent id (polymorphic)
  position,          # ordering within a column
  metadata,          # JSON (labels, links, attachments, etc.)
  created_at,
  updated_at
)
```

- **`created_by` is polymorphic** — a task can be created by a **user** (on
  their squad's board) or by an **agent** (including on **another squad's
  board**, if permitted — see cross-squad handoff).
- A task has **one assignee agent** at a time.

---

## 3. Task Lifecycle

```mermaid
stateDiagram-v2
    [*] --> todo: created
    todo --> in-progress: assigned + agent picks up (context reset)
    in-progress --> in-review: agent marks ready
    in-progress --> blocked: agent/user flags blocker
    in-review --> done: approved
    in-review --> in-progress: changes requested
    blocked --> todo: unblocked
    blocked --> in-progress: unblocked + picked up
    done --> [*]
```

- **Create (`todo`):** a user (or agent) creates the task on a board.
- **Assign:** a user (or agent, via delegation) assigns the task to an agent.
- **Pick up (`in-progress`):** the agent's **working context is reset**, the
  operator **scales the agent 0 → 1**, and the agent starts the task.
- **In review:** the agent marks the task ready; a reviewer (agent or user)
  checks it.
- **Done:** approved → `done`.
- **Blocked:** flagged when it cannot proceed; returns to `todo` or
  `in-progress` when unblocked.

---

## 4. Task Creation & Assignment

### 4.1 By a user
- A user creates a task on **their squad's board** (or a board they have access
  to).
- The user **assigns** it to an agent in the squad.
- Assigning a task to an agent **signals the operator** to scale that agent up
  (if it is at 0).

### 4.2 By an agent
- An agent can **create a task** — typically as part of **delegation** or
  **cross-squad handoff** (see [collaboration-messaging.md](collaboration-messaging.md)).
- An agent can create a task on **its own squad's board** or on **another
  squad's board** (if an access grant permits it).
- Agent-created tasks are **audited** (actor = agent).

---

## 5. Agent Pickup & Context Reset

When an agent picks up a task:

1. The **operator** scales the agent's Deployment **0 → 1** (if idle).
2. The agent runtime **resets its working context** (clears the previous task's
   conversation/scratch state).
3. The runtime **loads the task** (title, description, metadata).
4. (Optionally) the runtime **retrieves relevant long-term memory** into the
   fresh context.
5. The agent **runs the core loop** for the task (see
   [agent-runtime.md](agent-runtime.md)).
6. On completion, the agent submits the execution ID and fencing token with
   its terminal status/result summary. The control plane stores the result on
   the execution attempt in the same transaction as the task status update,
   then optionally attempts to store a bounded memory summary. Memory write
   failure is audited but does not make the completed task look retryable.

> **Key invariant:** an agent works on **one task at a time**. Its working
> context is scoped to the current task and reset before the next. Incoming
> messages while working are **queued** (not delivered) to protect the context.

---

## 6. Interaction Model

- **Primary:** the user interacts with the squad **through the board** — create
  tasks, assign them, watch columns move, review results.
- **Secondary:** the user can **chat directly** with an agent (a 1:1
  conversation) for ad-hoc questions or steering. Chat is **not** a task; it
  does not reset the task context (it is a separate, lightweight interaction).
- The board gives the user **visibility** into what each agent is doing and the
  squad's overall progress.

---

## 7. Relationship to Other Components

- **Control plane** — mirrors assigned `todo`/`in-progress` task state into the
  Agent CR so `desiredActive` wakes or keeps the assignee warm.
- **Operator** — scales the assignee Deployment up from `desiredActive`, records
  `idleSince` when the control plane clears pending work, and scales back to
  zero after the agent's idle timeout elapses.
- **Agent runtime** — picks up the task, resets context, runs the loop, updates
  status.
- **Message queue** — delegation / cross-squad handoff create tasks + pings
  (see [collaboration-messaging.md](collaboration-messaging.md)).
- **Identity & AuthZ** — enforces who can create/assign tasks on a board
  (owner + granted users; agents per access grants).
- **Postgres** — stores boards + tasks.
- **Web app** — board UI (columns, cards, drag/assign, detail view).

---

## 8. Concurrency & Consistency

- **One assignee at a time** — a task has a single `assignee_agent_id`.
- **Column moves** are atomic (single write) to avoid races.
- **Pickup is lease-backed** — claim creates a `task_execution` attempt with a
  worker ID, lease expiry, and fencing token. A second runtime cannot claim
  while the active lease is valid.
- **Crash recovery is lease-based** — if an agent pod dies mid-task, another
  runtime can reclaim the assigned `in-progress` task only after the previous
  execution lease expires.
- **Terminal updates are fenced** — complete/block requests must include the
  active execution ID and fencing token. Stale tokens are rejected with
  conflict and cannot overwrite the task result.
- **Optimistic concurrency** on task updates (version/updated_at check) to
  prevent lost updates when a user and agent both act remains a follow-up for
  user-driven board edits.

---

## 9. Open Points

- **Subtasks** — whether tasks can have subtasks (start flat; add later).
- **Task templates** — reusable task definitions (later).
- **WIP limits** — per-column work-in-progress limits (later).
- **Task history** — full column-move history for audit/analysis (the audit log
  covers significant moves; a dedicated history table is optional).
