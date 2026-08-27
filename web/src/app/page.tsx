"use client";

import { FormEvent, useEffect, useMemo, useState } from "react";
import {
  Agent,
  AgentIdentity,
  ApiError,
  ApiState,
  ApiUser,
  BoardPayload,
  Message,
  Squad,
  Task,
  TaskStatus,
  apiBaseUrl,
  apiDelete,
  apiGet,
  apiPatch,
  apiPost,
} from "../lib/api";

type Section = "squads" | "agents" | "tasks" | "registry" | "admin";

const navItems: Array<{ id: Section; label: string }> = [
  { id: "squads", label: "Squads" },
  { id: "agents", label: "Agents" },
  { id: "tasks", label: "Tasks" },
  { id: "registry", label: "Registry" },
  { id: "admin", label: "Admin" },
];

const taskStatuses: TaskStatus[] = ["todo", "in-progress", "in-review", "done", "blocked"];

const emptyState = {
  data: null,
  loading: true,
  error: "",
};

export default function Home() {
  const [token, setToken] = useState("");
  const [draftToken, setDraftToken] = useState("");
  const [activeSection, setActiveSection] = useState<Section>("squads");
  const [selectedSquadID, setSelectedSquadID] = useState("");
  const [selectedAgentID, setSelectedAgentID] = useState("");
  const [refreshTick, setRefreshTick] = useState(0);
  const [actionMessage, setActionMessage] = useState("");
  const [user, setUser] = useState<ApiState<ApiUser>>(emptyState);
  const [squads, setSquads] = useState<ApiState<Squad[]>>(emptyState);
  const [agents, setAgents] = useState<ApiState<Agent[]>>({ data: [], loading: false, error: "" });
  const [board, setBoard] = useState<ApiState<BoardPayload>>({ data: null, loading: false, error: "" });
  const [chat, setChat] = useState<ApiState<Message[]>>({ data: [], loading: false, error: "" });
  const [newSquadForm, setNewSquadForm] = useState({ name: "", mission: "" });
  const [squadMissionDraft, setSquadMissionDraft] = useState("");
  const [agentForm, setAgentForm] = useState({ name: "", role: "", default_model: "", idle_timeout_sec: "300" });
  const [taskForm, setTaskForm] = useState({ title: "", description: "", assignee_agent_id: "" });
  const [chatDraft, setChatDraft] = useState("");

  useEffect(() => {
    const stored = window.localStorage.getItem("skquad.authToken") || "";
    setToken(stored);
    setDraftToken(stored);
  }, []);

  useEffect(() => {
    let cancelled = false;
    setUser({ data: null, loading: true, error: "" });
    setSquads({ data: null, loading: true, error: "" });

    Promise.allSettled([
      apiGet<ApiUser>("/auth/me", token),
      apiGet<Squad[]>("/squads", token),
    ]).then(([userResult, squadResult]) => {
      if (cancelled) {
        return;
      }
      const nextSquads = stateFromResult(squadResult, []);
      setUser(stateFromResult(userResult));
      setSquads(nextSquads);
      const list = nextSquads.data || [];
      if (list.length > 0 && !list.some((squad) => squad.id === selectedSquadID)) {
        setSelectedSquadID(list[0].id);
      }
      if (list.length === 0) {
        setSelectedSquadID("");
      }
    });

    return () => {
      cancelled = true;
    };
  }, [token, refreshTick, selectedSquadID]);

  useEffect(() => {
    if (!selectedSquadID) {
      setAgents({ data: [], loading: false, error: "" });
      setBoard({ data: null, loading: false, error: "" });
      setSelectedAgentID("");
      return;
    }

    let cancelled = false;
    setAgents({ data: null, loading: true, error: "" });
    setBoard({ data: null, loading: true, error: "" });
    Promise.allSettled([
      apiGet<Agent[]>(`/squads/${selectedSquadID}/agents`, token),
      apiGet<BoardPayload>(`/squads/${selectedSquadID}/board`, token),
    ]).then(([agentResult, boardResult]) => {
      if (cancelled) {
        return;
      }
      const nextAgents = stateFromResult(agentResult, []);
      setAgents(nextAgents);
      setBoard(stateFromResult(boardResult));
      const list = nextAgents.data || [];
      if (list.length > 0 && !list.some((agent) => agent.id === selectedAgentID)) {
        setSelectedAgentID(list[0].id);
      }
      if (list.length === 0) {
        setSelectedAgentID("");
      }
    });

    return () => {
      cancelled = true;
    };
  }, [selectedSquadID, token, refreshTick, selectedAgentID]);

  useEffect(() => {
    const selected = (squads.data || []).find((squad) => squad.id === selectedSquadID);
    if (selected) {
      setSquadMissionDraft(selected.mission || "");
    }
  }, [selectedSquadID, squads.data]);

  useEffect(() => {
    if (!selectedAgentID) {
      setChat({ data: [], loading: false, error: "" });
      return;
    }
    let cancelled = false;
    setChat({ data: null, loading: true, error: "" });
    apiGet<Message[]>(`/agents/${selectedAgentID}/chat`, token)
      .then((messages) => {
        if (!cancelled) {
          setChat({ data: messages, loading: false, error: "" });
        }
      })
      .catch((error) => {
        if (!cancelled) {
          setChat(errorState(error, []));
        }
      });
    return () => {
      cancelled = true;
    };
  }, [selectedAgentID, token, refreshTick]);

  const connected = user.data !== null && user.error === "";
  const selectedSquad = (squads.data || []).find((squad) => squad.id === selectedSquadID) || null;
  const selectedAgent = (agents.data || []).find((agent) => agent.id === selectedAgentID) || null;
  const stats = useMemo(() => {
    const activeSquads = (squads.data || []).filter((item) => item.status !== "deleted").length;
    const activeAgents = (agents.data || []).length;
    const tasks = board.data?.tasks || [];
    const openTasks = tasks.filter((task) => task.status !== "done").length;
    return [
      { label: "Squads", value: String(activeSquads), tone: "strong" },
      { label: "Agents", value: String(activeAgents), tone: "neutral" },
      { label: "Open Tasks", value: String(openTasks), tone: openTasks > 0 ? "warn" : "good" },
      { label: "API", value: connected ? "Connected" : "Waiting", tone: connected ? "good" : "warn" },
      { label: "Mode", value: token ? "Bearer" : "Dev", tone: "neutral" },
    ];
  }, [agents.data, board.data, connected, squads.data, token]);

  function refresh(message = "") {
    setActionMessage(message);
    setRefreshTick((current) => current + 1);
  }

  function saveToken(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const next = draftToken.trim();
    if (next) {
      window.localStorage.setItem("skquad.authToken", next);
    } else {
      window.localStorage.removeItem("skquad.authToken");
    }
    setToken(next);
  }

  async function submitSquad(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    await runAction("Squad created", async () => {
      const created = await apiPost<Squad>("/squads", token, {
        name: newSquadForm.name,
        mission: newSquadForm.mission,
        operating_model: {},
      });
      setNewSquadForm({ name: "", mission: "" });
      setSelectedSquadID(created.id);
    });
  }

  async function updateSquadMission(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!selectedSquad) {
      return;
    }
    await runAction("Squad updated", async () => {
      await apiPatch<Squad>(`/squads/${selectedSquad.id}`, token, {
        name: selectedSquad.name,
        mission: squadMissionDraft,
      });
    });
  }

  async function submitAgent(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!selectedSquadID) {
      return;
    }
    await runAction("Agent created", async () => {
      const created = await apiPost<Agent>(`/squads/${selectedSquadID}/agents`, token, {
        name: agentForm.name,
        role: agentForm.role,
        default_model: agentForm.default_model,
        permissions: [],
        idle_timeout_sec: Number(agentForm.idle_timeout_sec) || 300,
      });
      setAgentForm({ name: "", role: "", default_model: "", idle_timeout_sec: "300" });
      setSelectedAgentID(created.id);
    });
  }

  async function createIdentity(agentID: string) {
    await runAction("Identity created", async () => {
      await apiPost<AgentIdentity>(`/agents/${agentID}/identity`, token, {});
    });
  }

  async function rotateIdentity(agentID: string) {
    await runAction("Identity rotated", async () => {
      await apiPost<AgentIdentity>(`/agents/${agentID}/identity/rotate`, token, {});
    });
  }

  async function submitTask(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!selectedSquadID) {
      return;
    }
    await runAction("Task created", async () => {
      await apiPost<Task>(`/squads/${selectedSquadID}/board/tasks`, token, taskForm);
      setTaskForm({ title: "", description: "", assignee_agent_id: "" });
    });
  }

  async function moveTask(taskID: string, status: TaskStatus) {
    await runAction("Task moved", async () => {
      await apiPost<Task>(`/tasks/${taskID}/move`, token, { status });
    });
  }

  async function assignTask(taskID: string, assigneeAgentID: string) {
    await runAction("Task assignment updated", async () => {
      await apiPatch<Task>(`/tasks/${taskID}`, token, { assignee_agent_id: assigneeAgentID });
    });
  }

  async function deleteTask(taskID: string) {
    if (!window.confirm("Delete this task?")) {
      return;
    }
    await runAction("Task deleted", async () => {
      await apiDelete(`/tasks/${taskID}`, token);
    });
  }

  async function sendChat(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!selectedAgentID || chatDraft.trim() === "") {
      return;
    }
    await runAction("Message queued", async () => {
      await apiPost<Message>(`/agents/${selectedAgentID}/chat`, token, {
        type: "consult",
        message: chatDraft,
      });
      setChatDraft("");
    });
  }

  async function runAction(success: string, action: () => Promise<void>) {
    setActionMessage("");
    try {
      await action();
      refresh(success);
    } catch (error) {
      const state = errorState(error);
      setActionMessage(state.error || "Request failed");
    }
  }

  return (
    <main className="app-shell">
      <aside className="sidebar" aria-label="Primary navigation">
        <div className="brand">
          <span className="brand-mark">sq</span>
          <span>
            <strong>skquad</strong>
            <small>control plane</small>
          </span>
        </div>
        <nav className="nav-list">
          {navItems.map((item) => (
            <button
              key={item.id}
              className={item.id === activeSection ? "nav-item active" : "nav-item"}
              type="button"
              onClick={() => setActiveSection(item.id)}
            >
              {item.label}
            </button>
          ))}
        </nav>
      </aside>

      <section className="workspace">
        <header className="topbar">
          <div>
            <p className="eyebrow">Workspace</p>
            <h1>{sectionTitle(activeSection)}</h1>
          </div>
          <div className="auth-panel">
            <div className="identity">
              <span className={connected ? "status-dot online" : "status-dot"} />
              <span>{userLabel(user)}</span>
            </div>
            <form className="token-form" onSubmit={saveToken}>
              <label htmlFor="auth-token">Token</label>
              <input
                id="auth-token"
                type="password"
                value={draftToken}
                onChange={(event) => setDraftToken(event.target.value)}
                placeholder="dev or bearer"
              />
              <button type="submit">Apply</button>
            </form>
          </div>
        </header>

        <section className="summary-grid" aria-label="System summary">
          {stats.map((item) => (
            <article className={`summary-tile ${item.tone}`} key={item.label}>
              <span>{item.label}</span>
              <strong>{item.value}</strong>
            </article>
          ))}
        </section>

        <section className="content-band">
          <div className="section-head">
            <div>
              <p className="eyebrow">API base</p>
              <h2>{apiBaseUrl()}</h2>
            </div>
            <button className="secondary" type="button" onClick={() => refresh()}>
              Refresh
            </button>
          </div>
          {actionMessage && <div className={actionMessage.includes(":") ? "notice error compact" : "notice good compact"}>{actionMessage}</div>}

          {activeSection === "squads" && (
            <SquadsView
              state={squads}
              selectedSquadID={selectedSquadID}
              form={newSquadForm}
              setForm={setNewSquadForm}
              missionDraft={squadMissionDraft}
              setMissionDraft={setSquadMissionDraft}
              selectedSquad={selectedSquad}
              onSelect={setSelectedSquadID}
              onCreate={submitSquad}
              onUpdateMission={updateSquadMission}
            />
          )}
          {activeSection === "agents" && (
            <AgentsView
              selectedSquad={selectedSquad}
              state={agents}
              selectedAgentID={selectedAgentID}
              form={agentForm}
              setForm={setAgentForm}
              onSelect={setSelectedAgentID}
              onCreate={submitAgent}
              onCreateIdentity={createIdentity}
              onRotateIdentity={rotateIdentity}
              chat={chat}
              chatDraft={chatDraft}
              setChatDraft={setChatDraft}
              onSendChat={sendChat}
            />
          )}
          {activeSection === "tasks" && (
            <TasksView
              selectedSquad={selectedSquad}
              board={board}
              agents={agents.data || []}
              form={taskForm}
              setForm={setTaskForm}
              onCreate={submitTask}
              onMove={moveTask}
              onAssign={assignTask}
              onDelete={deleteTask}
            />
          )}
          {activeSection === "registry" && <Placeholder title="Registry" detail="Provider and plugin grant workflows are tracked in the next UI card." />}
          {activeSection === "admin" && <AdminView user={user} selectedSquad={selectedSquad} selectedAgent={selectedAgent} />}
        </section>
      </section>
    </main>
  );
}

function stateFromResult<T>(result: PromiseSettledResult<T>, fallback: T | null = null): ApiState<T> {
  if (result.status === "fulfilled") {
    return { data: result.value, loading: false, error: "" };
  }
  return errorState(result.reason, fallback);
}

function errorState<T>(reason: unknown, fallback: T | null = null): ApiState<T> {
  if (reason instanceof ApiError) {
    return { data: fallback, loading: false, error: `${reason.status}: ${reason.message}` };
  }
  return { data: fallback, loading: false, error: reason instanceof Error ? reason.message : "Request failed" };
}

function userLabel(state: ApiState<ApiUser>) {
  if (state.loading) {
    return "Checking session";
  }
  if (state.data) {
    return state.data.name || state.data.email;
  }
  return "Not connected";
}

function sectionTitle(section: Section) {
  return {
    squads: "Squads",
    agents: "Agents",
    tasks: "Tasks",
    registry: "Registry",
    admin: "Admin",
  }[section];
}

function SquadsView({
  state,
  selectedSquadID,
  form,
  setForm,
  missionDraft,
  setMissionDraft,
  selectedSquad,
  onSelect,
  onCreate,
  onUpdateMission,
}: {
  state: ApiState<Squad[]>;
  selectedSquadID: string;
  form: { name: string; mission: string };
  setForm: (form: { name: string; mission: string }) => void;
  missionDraft: string;
  setMissionDraft: (value: string) => void;
  selectedSquad: Squad | null;
  onSelect: (id: string) => void;
  onCreate: (event: FormEvent<HTMLFormElement>) => void;
  onUpdateMission: (event: FormEvent<HTMLFormElement>) => void;
}) {
  const squads = state.data || [];
  return (
    <div className="workflow-grid">
      <form className="form-panel" onSubmit={onCreate}>
        <h3>Create Squad</h3>
        <label>
          Name
          <input value={form.name} onChange={(event) => setForm({ ...form, name: event.target.value })} required />
        </label>
        <label>
          Mission
          <textarea value={form.mission} onChange={(event) => setForm({ ...form, mission: event.target.value })} rows={4} />
        </label>
        <button type="submit">Create</button>
      </form>

      <div className="span-2">
        <StateNotice state={state} empty="No squads yet" />
        {squads.length > 0 && (
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>Name</th>
                  <th>Status</th>
                  <th>Namespace</th>
                  <th>Mission</th>
                </tr>
              </thead>
              <tbody>
                {squads.map((squad) => (
                  <tr
                    key={squad.id}
                    className={squad.id === selectedSquadID ? "selected-row" : ""}
                    onClick={() => {
                      onSelect(squad.id);
                      setMissionDraft(squad.mission || "");
                    }}
                  >
                    <td>
                      <strong>{squad.name}</strong>
                      <small>{squad.id}</small>
                    </td>
                    <td>{squad.status || "active"}</td>
                    <td>{squad.namespace || "-"}</td>
                    <td>{squad.mission || "-"}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {selectedSquad && (
        <form className="form-panel span-3" onSubmit={onUpdateMission}>
          <h3>Selected Squad</h3>
          <div className="key-grid">
            <span>ID</span>
            <strong>{selectedSquad.id}</strong>
            <span>Owner</span>
            <strong>{selectedSquad.owner_id || "-"}</strong>
          </div>
          <label>
            Mission
            <textarea
              value={missionDraft}
              onChange={(event) => setMissionDraft(event.target.value)}
              rows={3}
            />
          </label>
          <button type="submit">Save Mission</button>
        </form>
      )}
    </div>
  );
}

function AgentsView({
  selectedSquad,
  state,
  selectedAgentID,
  form,
  setForm,
  onSelect,
  onCreate,
  onCreateIdentity,
  onRotateIdentity,
  chat,
  chatDraft,
  setChatDraft,
  onSendChat,
}: {
  selectedSquad: Squad | null;
  state: ApiState<Agent[]>;
  selectedAgentID: string;
  form: { name: string; role: string; default_model: string; idle_timeout_sec: string };
  setForm: (form: { name: string; role: string; default_model: string; idle_timeout_sec: string }) => void;
  onSelect: (id: string) => void;
  onCreate: (event: FormEvent<HTMLFormElement>) => void;
  onCreateIdentity: (id: string) => void;
  onRotateIdentity: (id: string) => void;
  chat: ApiState<Message[]>;
  chatDraft: string;
  setChatDraft: (value: string) => void;
  onSendChat: (event: FormEvent<HTMLFormElement>) => void;
}) {
  const agents = state.data || [];
  const selectedAgent = agents.find((agent) => agent.id === selectedAgentID) || null;
  if (!selectedSquad) {
    return <div className="notice">Select or create a squad first</div>;
  }
  return (
    <div className="workflow-grid">
      <form className="form-panel" onSubmit={onCreate}>
        <h3>Add Agent</h3>
        <label>
          Name
          <input value={form.name} onChange={(event) => setForm({ ...form, name: event.target.value })} required />
        </label>
        <label>
          Role
          <textarea value={form.role} onChange={(event) => setForm({ ...form, role: event.target.value })} rows={3} />
        </label>
        <label>
          Default model
          <input value={form.default_model} onChange={(event) => setForm({ ...form, default_model: event.target.value })} placeholder="provider/model" />
        </label>
        <label>
          Idle timeout seconds
          <input
            type="number"
            min="1"
            value={form.idle_timeout_sec}
            onChange={(event) => setForm({ ...form, idle_timeout_sec: event.target.value })}
          />
        </label>
        <button type="submit">Add Agent</button>
      </form>

      <div className="span-2">
        <StateNotice state={state} empty="No agents in this squad" />
        {agents.length > 0 && (
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>Agent</th>
                  <th>Status</th>
                  <th>Model</th>
                  <th>Identity</th>
                </tr>
              </thead>
              <tbody>
                {agents.map((agent) => (
                  <tr key={agent.id} className={agent.id === selectedAgentID ? "selected-row" : ""} onClick={() => onSelect(agent.id)}>
                    <td>
                      <strong>{agent.name}</strong>
                      <small>{agent.role || agent.id}</small>
                    </td>
                    <td>{agent.status || "idle"}</td>
                    <td>{agent.default_model || agent.default_provider_id || "-"}</td>
                    <td>
                      <div className="button-row">
                        <button type="button" className="secondary small" onClick={() => onCreateIdentity(agent.id)}>
                          Create
                        </button>
                        <button type="button" className="secondary small" onClick={() => onRotateIdentity(agent.id)}>
                          Rotate
                        </button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      <form className="form-panel span-3" onSubmit={onSendChat}>
        <h3>Agent Chat</h3>
        {selectedAgent ? (
          <>
            <div className="message-list">
              <StateNotice state={chat} empty="No chat messages yet" />
              {(chat.data || []).map((message) => (
                <article key={message.id} className="message-item">
                  <strong>{message.from_type}</strong>
                  <span>{messageText(message)}</span>
                  <small>{message.status} · {message.type}</small>
                </article>
              ))}
            </div>
            <label>
              Message to {selectedAgent.name}
              <textarea value={chatDraft} onChange={(event) => setChatDraft(event.target.value)} rows={3} />
            </label>
            <button type="submit">Send</button>
          </>
        ) : (
          <div className="notice compact">Select an agent to view or send messages</div>
        )}
      </form>
    </div>
  );
}

function TasksView({
  selectedSquad,
  board,
  agents,
  form,
  setForm,
  onCreate,
  onMove,
  onAssign,
  onDelete,
}: {
  selectedSquad: Squad | null;
  board: ApiState<BoardPayload>;
  agents: Agent[];
  form: { title: string; description: string; assignee_agent_id: string };
  setForm: (form: { title: string; description: string; assignee_agent_id: string }) => void;
  onCreate: (event: FormEvent<HTMLFormElement>) => void;
  onMove: (taskID: string, status: TaskStatus) => void;
  onAssign: (taskID: string, assigneeAgentID: string) => void;
  onDelete: (taskID: string) => void;
}) {
  const tasks = board.data?.tasks || [];
  if (!selectedSquad) {
    return <div className="notice">Select or create a squad first</div>;
  }
  return (
    <div className="workflow-grid">
      <form className="form-panel" onSubmit={onCreate}>
        <h3>Create Task</h3>
        <label>
          Title
          <input value={form.title} onChange={(event) => setForm({ ...form, title: event.target.value })} required />
        </label>
        <label>
          Description
          <textarea value={form.description} onChange={(event) => setForm({ ...form, description: event.target.value })} rows={4} />
        </label>
        <label>
          Assignee
          <select value={form.assignee_agent_id} onChange={(event) => setForm({ ...form, assignee_agent_id: event.target.value })}>
            <option value="">Unassigned</option>
            {agents.map((agent) => (
              <option key={agent.id} value={agent.id}>
                {agent.name}
              </option>
            ))}
          </select>
        </label>
        <button type="submit">Create Task</button>
      </form>

      <div className="span-2">
        <StateNotice state={board} empty="No tasks yet" />
        {tasks.length > 0 && (
          <div className="board-grid">
            {taskStatuses.map((status) => (
              <section className="task-column" key={status}>
                <h3>{status}</h3>
                {tasks.filter((task) => task.status === status).map((task) => (
                  <article className="task-card" key={task.id}>
                    <strong>{task.title}</strong>
                    <p>{task.description || "-"}</p>
                    <label>
                      Status
                      <select value={task.status} onChange={(event) => onMove(task.id, event.target.value as TaskStatus)}>
                        {taskStatuses.map((item) => (
                          <option key={item} value={item}>
                            {item}
                          </option>
                        ))}
                      </select>
                    </label>
                    <label>
                      Assignee
                      <select value={task.assignee_agent_id || ""} onChange={(event) => onAssign(task.id, event.target.value)}>
                        <option value="">Unassigned</option>
                        {agents.map((agent) => (
                          <option key={agent.id} value={agent.id}>
                            {agent.name}
                          </option>
                        ))}
                      </select>
                    </label>
                    <button type="button" className="secondary small" onClick={() => onDelete(task.id)}>
                      Delete
                    </button>
                  </article>
                ))}
              </section>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}

function StateNotice<T>({ state, empty }: { state: ApiState<T>; empty: string }) {
  const arrayData = Array.isArray(state.data) ? state.data : null;
  if (state.loading) {
    return <div className="notice compact">Loading</div>;
  }
  if (state.error) {
    return <div className="notice error compact">{state.error}</div>;
  }
  if (!state.data || (arrayData && arrayData.length === 0)) {
    return <div className="notice compact">{empty}</div>;
  }
  return null;
}

function AdminView({ user, selectedSquad, selectedAgent }: { user: ApiState<ApiUser>; selectedSquad: Squad | null; selectedAgent: Agent | null }) {
  return (
    <div className="admin-grid">
      <article>
        <span>Role</span>
        <strong>{user.data?.role || "-"}</strong>
      </article>
      <article>
        <span>User ID</span>
        <strong>{user.data?.id || "-"}</strong>
      </article>
      <article>
        <span>Email</span>
        <strong>{user.data?.email || "-"}</strong>
      </article>
      <article>
        <span>Selected Squad</span>
        <strong>{selectedSquad?.name || "-"}</strong>
      </article>
      <article>
        <span>Selected Agent</span>
        <strong>{selectedAgent?.name || "-"}</strong>
      </article>
    </div>
  );
}

function Placeholder({ title, detail }: { title: string; detail: string }) {
  return (
    <div className="notice">
      <strong>{title}</strong>
      <span>{detail}</span>
    </div>
  );
}

function messageText(message: Message) {
  const payload = message.payload || {};
  if (typeof payload.message === "string") {
    return payload.message;
  }
  return JSON.stringify(payload);
}
