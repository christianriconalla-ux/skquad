"use client";

import { FormEvent, useEffect, useMemo, useState } from "react";
import {
  Agent,
  AgentIdentity,
  AgentPermission,
  ApiError,
  ApiState,
  ApiUser,
  AccessGrant,
  AuditEntry,
  BoardPayload,
  LLMProvider,
  MeteringSummary,
  Message,
  RegistryResource,
  ResourceType,
  Squad,
  Task,
  TaskStatus,
  apiBaseUrl,
  apiDelete,
  apiGet,
  apiPatch,
  apiPost,
  apiPut,
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
const registryTypes: Array<{ type: Exclude<ResourceType, "llm_provider">; label: string; path: string }> = [
  { type: "skill", label: "Skills", path: "skills" },
  { type: "tool", label: "Tools", path: "tools" },
  { type: "api", label: "APIs", path: "apis" },
  { type: "knowledge_base", label: "Knowledge Bases", path: "knowledge-bases" },
  { type: "project_workspace", label: "Project Workspaces", path: "project-workspaces" },
];

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
  const [providers, setProviders] = useState<ApiState<LLMProvider[]>>({ data: [], loading: false, error: "" });
  const [resources, setResources] = useState<ApiState<RegistryResource[]>>({ data: [], loading: false, error: "" });
  const [agentPermissions, setAgentPermissions] = useState<ApiState<AgentPermission[]>>({ data: [], loading: false, error: "" });
  const [accessGrants, setAccessGrants] = useState<ApiState<AccessGrant[]>>({ data: [], loading: false, error: "" });
  const [audit, setAudit] = useState<ApiState<AuditEntry[]>>({ data: [], loading: false, error: "" });
  const [metering, setMetering] = useState<ApiState<MeteringSummary>>({ data: null, loading: false, error: "" });
  const [newSquadForm, setNewSquadForm] = useState({ name: "", mission: "" });
  const [squadMissionDraft, setSquadMissionDraft] = useState("");
  const [agentForm, setAgentForm] = useState({ name: "", role: "", default_model: "", idle_timeout_sec: "300" });
  const [taskForm, setTaskForm] = useState({ title: "", description: "", assignee_agent_id: "" });
  const [chatDraft, setChatDraft] = useState("");
  const [providerForm, setProviderForm] = useState({ name: "", kind: "openai", base_url: "", api_key_ref: "", default_model: "", models: "" });
  const [resourceForm, setResourceForm] = useState({ type: "skill" as Exclude<ResourceType, "llm_provider">, name: "", description: "", endpoint: "", auth_ref: "", manifest: "{}" });
  const [permissionForm, setPermissionForm] = useState({ resource_type: "llm_provider" as ResourceType, resource_id: "" });
  const [grantForm, setGrantForm] = useState({ grantee_type: "user" as "user" | "agent", grantee_id: "", permissions: "talk" });

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

  useEffect(() => {
    if (activeSection !== "registry" && activeSection !== "agents") {
      return;
    }
    let cancelled = false;
    setProviders({ data: null, loading: true, error: "" });
    setResources({ data: null, loading: true, error: "" });
    Promise.allSettled([
      apiGet<LLMProvider[]>("/registry/llm-providers", token),
      ...registryTypes.map((item) => apiGet<RegistryResource[]>(`/registry/${item.path}`, token)),
    ]).then(([providerResult, ...resourceResults]) => {
      if (cancelled) {
        return;
      }
      setProviders(stateFromResult(providerResult, []));
      const combined: RegistryResource[] = [];
      let error = "";
      for (const result of resourceResults) {
        const state = stateFromResult(result, []);
        if (state.error && !error) {
          error = state.error;
        }
        combined.push(...(state.data || []));
      }
      setResources({ data: combined, loading: false, error });
    });
    return () => {
      cancelled = true;
    };
  }, [activeSection, token, refreshTick]);

  useEffect(() => {
    if (!selectedAgentID) {
      setAgentPermissions({ data: [], loading: false, error: "" });
      return;
    }
    let cancelled = false;
    setAgentPermissions({ data: null, loading: true, error: "" });
    apiGet<AgentPermission[]>(`/agents/${selectedAgentID}/permissions`, token)
      .then((items) => {
        if (!cancelled) {
          setAgentPermissions({ data: items, loading: false, error: "" });
        }
      })
      .catch((error) => {
        if (!cancelled) {
          setAgentPermissions(errorState(error, []));
        }
      });
    return () => {
      cancelled = true;
    };
  }, [selectedAgentID, token, refreshTick]);

  useEffect(() => {
    if (!selectedSquadID) {
      setAccessGrants({ data: [], loading: false, error: "" });
      return;
    }
    let cancelled = false;
    setAccessGrants({ data: null, loading: true, error: "" });
    apiGet<AccessGrant[]>(`/squads/${selectedSquadID}/access-grants`, token)
      .then((items) => {
        if (!cancelled) {
          setAccessGrants({ data: items, loading: false, error: "" });
        }
      })
      .catch((error) => {
        if (!cancelled) {
          setAccessGrants(errorState(error, []));
        }
      });
    return () => {
      cancelled = true;
    };
  }, [selectedSquadID, token, refreshTick]);

  useEffect(() => {
    if (activeSection !== "admin") {
      return;
    }
    let cancelled = false;
    setAudit({ data: null, loading: true, error: "" });
    setMetering({ data: null, loading: true, error: "" });
    Promise.allSettled([
      apiGet<AuditEntry[]>("/audit", token),
      apiGet<MeteringSummary>("/metering/summary", token),
    ]).then(([auditResult, meteringResult]) => {
      if (cancelled) {
        return;
      }
      setAudit(stateFromResult(auditResult, []));
      setMetering(stateFromResult(meteringResult));
    });
    return () => {
      cancelled = true;
    };
  }, [activeSection, token, refreshTick]);

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

  async function submitProvider(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    await runAction("Provider registered", async () => {
      await apiPost<LLMProvider>("/registry/llm-providers", token, {
        name: providerForm.name,
        kind: providerForm.kind,
        base_url: providerForm.base_url,
        api_key_ref: providerForm.api_key_ref,
        default_model: providerForm.default_model,
        models: parseJSONList(providerForm.models),
        pricing: {},
      });
      setProviderForm({ name: "", kind: "openai", base_url: "", api_key_ref: "", default_model: "", models: "" });
    });
  }

  async function submitResource(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const route = registryTypes.find((item) => item.type === resourceForm.type)?.path || "skills";
    await runAction("Resource registered", async () => {
      await apiPost<RegistryResource>(`/registry/${route}`, token, {
        name: resourceForm.name,
        description: resourceForm.description,
        endpoint: resourceForm.endpoint,
        auth_ref: resourceForm.auth_ref,
        manifest: parseJSONObject(resourceForm.manifest),
      });
      setResourceForm({ type: resourceForm.type, name: "", description: "", endpoint: "", auth_ref: "", manifest: "{}" });
    });
  }

  async function deprecateProvider(providerID: string) {
    await runAction("Provider deprecated", async () => {
      await apiPost<void>(`/registry/llm-providers/${providerID}/deprecate`, token, {});
    });
  }

  async function deprecateResource(resource: RegistryResource) {
    const route = registryTypes.find((item) => item.type === resource.type)?.path;
    if (!route) {
      return;
    }
    await runAction("Resource deprecated", async () => {
      await apiPost<void>(`/registry/${route}/${resource.id}/deprecate`, token, {});
    });
  }

  async function grantAgentPermission(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!selectedAgentID || permissionForm.resource_id === "") {
      return;
    }
    await runAction("Agent permission updated", async () => {
      const current = agentPermissions.data || [];
      const next = [
        ...current.map((item) => ({ resource_type: item.resource_type, resource_id: item.resource_id })),
        { resource_type: permissionForm.resource_type, resource_id: permissionForm.resource_id },
      ];
      await apiPut<AgentPermission[]>(`/agents/${selectedAgentID}/permissions`, token, uniquePermissions(next));
      setPermissionForm({ ...permissionForm, resource_id: "" });
    });
  }

  async function revokeAgentPermission(permission: AgentPermission) {
    if (!selectedAgentID) {
      return;
    }
    await runAction("Agent permission revoked", async () => {
      const next = (agentPermissions.data || [])
        .filter((item) => item.id !== permission.id)
        .map((item) => ({ resource_type: item.resource_type, resource_id: item.resource_id }));
      await apiPut<AgentPermission[]>(`/agents/${selectedAgentID}/permissions`, token, next);
    });
  }

  async function submitAccessGrant(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!selectedSquadID) {
      return;
    }
    await runAction("Access grant created", async () => {
      await apiPost<AccessGrant>(`/squads/${selectedSquadID}/access-grants`, token, grantForm);
      setGrantForm({ ...grantForm, grantee_id: "" });
    });
  }

  async function revokeAccessGrant(grantID: string) {
    await runAction("Access grant revoked", async () => {
      await apiDelete(`/access-grants/${grantID}`, token);
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
          {activeSection === "registry" && (
            <RegistryView
              providers={providers}
              resources={resources}
              agents={agents.data || []}
              selectedAgent={selectedAgent}
              permissions={agentPermissions}
              accessGrants={accessGrants}
              providerForm={providerForm}
              setProviderForm={setProviderForm}
              resourceForm={resourceForm}
              setResourceForm={setResourceForm}
              permissionForm={permissionForm}
              setPermissionForm={setPermissionForm}
              grantForm={grantForm}
              setGrantForm={setGrantForm}
              onCreateProvider={submitProvider}
              onCreateResource={submitResource}
              onDeprecateProvider={deprecateProvider}
              onDeprecateResource={deprecateResource}
              onGrantPermission={grantAgentPermission}
              onRevokePermission={revokeAgentPermission}
              onCreateGrant={submitAccessGrant}
              onRevokeGrant={revokeAccessGrant}
            />
          )}
          {activeSection === "admin" && (
            <AdminView
              user={user}
              selectedSquad={selectedSquad}
              selectedAgent={selectedAgent}
              metering={metering}
              audit={audit}
            />
          )}
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

function RegistryView({
  providers,
  resources,
  agents,
  selectedAgent,
  permissions,
  accessGrants,
  providerForm,
  setProviderForm,
  resourceForm,
  setResourceForm,
  permissionForm,
  setPermissionForm,
  grantForm,
  setGrantForm,
  onCreateProvider,
  onCreateResource,
  onDeprecateProvider,
  onDeprecateResource,
  onGrantPermission,
  onRevokePermission,
  onCreateGrant,
  onRevokeGrant,
}: {
  providers: ApiState<LLMProvider[]>;
  resources: ApiState<RegistryResource[]>;
  agents: Agent[];
  selectedAgent: Agent | null;
  permissions: ApiState<AgentPermission[]>;
  accessGrants: ApiState<AccessGrant[]>;
  providerForm: { name: string; kind: string; base_url: string; api_key_ref: string; default_model: string; models: string };
  setProviderForm: (form: { name: string; kind: string; base_url: string; api_key_ref: string; default_model: string; models: string }) => void;
  resourceForm: { type: Exclude<ResourceType, "llm_provider">; name: string; description: string; endpoint: string; auth_ref: string; manifest: string };
  setResourceForm: (form: { type: Exclude<ResourceType, "llm_provider">; name: string; description: string; endpoint: string; auth_ref: string; manifest: string }) => void;
  permissionForm: { resource_type: ResourceType; resource_id: string };
  setPermissionForm: (form: { resource_type: ResourceType; resource_id: string }) => void;
  grantForm: { grantee_type: "user" | "agent"; grantee_id: string; permissions: string };
  setGrantForm: (form: { grantee_type: "user" | "agent"; grantee_id: string; permissions: string }) => void;
  onCreateProvider: (event: FormEvent<HTMLFormElement>) => void;
  onCreateResource: (event: FormEvent<HTMLFormElement>) => void;
  onDeprecateProvider: (id: string) => void;
  onDeprecateResource: (resource: RegistryResource) => void;
  onGrantPermission: (event: FormEvent<HTMLFormElement>) => void;
  onRevokePermission: (permission: AgentPermission) => void;
  onCreateGrant: (event: FormEvent<HTMLFormElement>) => void;
  onRevokeGrant: (id: string) => void;
}) {
  const providerItems = providers.data || [];
  const resourceItems = resources.data || [];
  const permissionItems = permissions.data || [];
  const grantItems = accessGrants.data || [];
  const grantableResources = [
    ...providerItems.map((provider) => ({ type: "llm_provider" as ResourceType, id: provider.id, name: provider.name })),
    ...resourceItems.map((resource) => ({ type: resource.type, id: resource.id, name: resource.name })),
  ].filter((item) => item.type === permissionForm.resource_type);

  return (
    <div className="workflow-grid">
      <form className="form-panel" onSubmit={onCreateProvider}>
        <h3>Register LLM Provider</h3>
        <label>
          Name
          <input value={providerForm.name} onChange={(event) => setProviderForm({ ...providerForm, name: event.target.value })} required />
        </label>
        <label>
          Kind
          <input value={providerForm.kind} onChange={(event) => setProviderForm({ ...providerForm, kind: event.target.value })} required />
        </label>
        <label>
          Base URL
          <input value={providerForm.base_url} onChange={(event) => setProviderForm({ ...providerForm, base_url: event.target.value })} required />
        </label>
        <label>
          API key ref
          <input value={providerForm.api_key_ref} onChange={(event) => setProviderForm({ ...providerForm, api_key_ref: event.target.value })} />
        </label>
        <label>
          Default model
          <input value={providerForm.default_model} onChange={(event) => setProviderForm({ ...providerForm, default_model: event.target.value })} />
        </label>
        <label>
          Models JSON array
          <textarea value={providerForm.models} onChange={(event) => setProviderForm({ ...providerForm, models: event.target.value })} rows={3} placeholder='["gpt-4.1-mini"]' />
        </label>
        <button type="submit">Register Provider</button>
      </form>

      <form className="form-panel" onSubmit={onCreateResource}>
        <h3>Register Resource</h3>
        <label>
          Type
          <select value={resourceForm.type} onChange={(event) => setResourceForm({ ...resourceForm, type: event.target.value as Exclude<ResourceType, "llm_provider"> })}>
            {registryTypes.map((item) => (
              <option key={item.type} value={item.type}>
                {item.label}
              </option>
            ))}
          </select>
        </label>
        <label>
          Name
          <input value={resourceForm.name} onChange={(event) => setResourceForm({ ...resourceForm, name: event.target.value })} required />
        </label>
        <label>
          Description
          <textarea value={resourceForm.description} onChange={(event) => setResourceForm({ ...resourceForm, description: event.target.value })} rows={3} />
        </label>
        <label>
          Endpoint
          <input value={resourceForm.endpoint} onChange={(event) => setResourceForm({ ...resourceForm, endpoint: event.target.value })} />
        </label>
        <label>
          Auth ref
          <input value={resourceForm.auth_ref} onChange={(event) => setResourceForm({ ...resourceForm, auth_ref: event.target.value })} />
        </label>
        <label>
          Manifest JSON
          <textarea value={resourceForm.manifest} onChange={(event) => setResourceForm({ ...resourceForm, manifest: event.target.value })} rows={3} />
        </label>
        <button type="submit">Register Resource</button>
      </form>

      <div className="form-panel">
        <h3>Grant Agent Resource</h3>
        {selectedAgent ? (
          <form className="nested-form" onSubmit={onGrantPermission}>
            <label>
              Agent
              <input value={selectedAgent.name} readOnly />
            </label>
            <label>
              Resource type
              <select value={permissionForm.resource_type} onChange={(event) => setPermissionForm({ resource_type: event.target.value as ResourceType, resource_id: "" })}>
                <option value="llm_provider">LLM Provider</option>
                {registryTypes.map((item) => (
                  <option key={item.type} value={item.type}>
                    {item.label}
                  </option>
                ))}
              </select>
            </label>
            <label>
              Resource
              <select value={permissionForm.resource_id} onChange={(event) => setPermissionForm({ ...permissionForm, resource_id: event.target.value })}>
                <option value="">Select resource</option>
                {grantableResources.map((resource) => (
                  <option key={`${resource.type}:${resource.id}`} value={resource.id}>
                    {resource.name}
                  </option>
                ))}
              </select>
            </label>
            <button type="submit">Grant</button>
          </form>
        ) : (
          <div className="notice compact">Select an agent before granting resources</div>
        )}
      </div>

      <div className="span-2">
        <h3 className="panel-title">Provider Catalog</h3>
        <StateNotice state={providers} empty="No providers registered" />
        {providerItems.length > 0 && (
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>Name</th>
                  <th>Kind</th>
                  <th>Model</th>
                  <th>Status</th>
                  <th />
                </tr>
              </thead>
              <tbody>
                {providerItems.map((provider) => (
                  <tr key={provider.id}>
                    <td>
                      <strong>{provider.name}</strong>
                      <small>{provider.id}</small>
                    </td>
                    <td>{provider.kind}</td>
                    <td>{provider.default_model || "-"}</td>
                    <td>{provider.status}</td>
                    <td>
                      <button type="button" className="secondary small" onClick={() => onDeprecateProvider(provider.id)}>
                        Deprecate
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      <div>
        <h3 className="panel-title">Agent Permissions</h3>
        <StateNotice state={permissions} empty="No resources granted" />
        <div className="stack-list">
          {permissionItems.map((permission) => (
            <article className="message-item" key={permission.id}>
              <strong>{permission.resource_type}</strong>
              <span>{resourceLabel(permission, providerItems, resourceItems)}</span>
              <button type="button" className="secondary small" onClick={() => onRevokePermission(permission)}>
                Revoke
              </button>
            </article>
          ))}
        </div>
      </div>

      <div className="span-2">
        <h3 className="panel-title">Resource Catalog</h3>
        <StateNotice state={resources} empty="No generic resources registered" />
        {resourceItems.length > 0 && (
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>Name</th>
                  <th>Type</th>
                  <th>Endpoint</th>
                  <th>Status</th>
                  <th />
                </tr>
              </thead>
              <tbody>
                {resourceItems.map((resource) => (
                  <tr key={resource.id}>
                    <td>
                      <strong>{resource.name}</strong>
                      <small>{resource.id}</small>
                    </td>
                    <td>{resource.type}</td>
                    <td>{resource.endpoint || "-"}</td>
                    <td>{resource.status}</td>
                    <td>
                      <button type="button" className="secondary small" onClick={() => onDeprecateResource(resource)}>
                        Deprecate
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      <form className="form-panel" onSubmit={onCreateGrant}>
        <h3>Squad Access Grant</h3>
        <label>
          Grantee type
          <select value={grantForm.grantee_type} onChange={(event) => setGrantForm({ ...grantForm, grantee_type: event.target.value as "user" | "agent" })}>
            <option value="user">User</option>
            <option value="agent">Agent</option>
          </select>
        </label>
        <label>
          Grantee ID
          <input value={grantForm.grantee_id} onChange={(event) => setGrantForm({ ...grantForm, grantee_id: event.target.value })} required />
        </label>
        <label>
          Permissions
          <input value={grantForm.permissions} onChange={(event) => setGrantForm({ ...grantForm, permissions: event.target.value })} />
        </label>
        <button type="submit">Create Grant</button>
      </form>

      <div className="span-3">
        <h3 className="panel-title">Squad Access Grants</h3>
        <StateNotice state={accessGrants} empty="No access grants" />
        <div className="stack-list">
          {grantItems.map((grant) => (
            <article className="message-item" key={grant.id}>
              <strong>{grant.grantee_type}: {grant.grantee_id}</strong>
              <span>{grant.permissions || "talk"}</span>
              <button type="button" className="secondary small" onClick={() => onRevokeGrant(grant.id)}>
                Revoke
              </button>
            </article>
          ))}
        </div>
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

function AdminView({
  user,
  selectedSquad,
  selectedAgent,
  metering,
  audit,
}: {
  user: ApiState<ApiUser>;
  selectedSquad: Squad | null;
  selectedAgent: Agent | null;
  metering: ApiState<MeteringSummary>;
  audit: ApiState<AuditEntry[]>;
}) {
  const entries = audit.data || [];
  return (
    <div className="admin-stack">
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
        <article>
          <span>Platform Cost</span>
          <strong>{formatCost(metering.data)}</strong>
        </article>
      </div>

      <section>
        <h3 className="panel-title">Platform Audit</h3>
        <StateNotice state={audit} empty="No audit entries" />
        {entries.length > 0 && (
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>Action</th>
                  <th>Actor</th>
                  <th>Resource</th>
                  <th>Squad</th>
                  <th>Time</th>
                </tr>
              </thead>
              <tbody>
                {entries.map((entry) => (
                  <tr key={entry.id}>
                    <td>{entry.action}</td>
                    <td>
                      <strong>{entry.actor_type}</strong>
                      <small>{entry.actor_id}</small>
                    </td>
                    <td>
                      <strong>{entry.resource_type}</strong>
                      <small>{entry.resource_id}</small>
                    </td>
                    <td>{entry.squad_id || "-"}</td>
                    <td>{entry.timestamp ? new Date(entry.timestamp).toLocaleString() : "-"}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>
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

function parseJSONList(value: string): unknown[] {
  const trimmed = value.trim();
  if (!trimmed) {
    return [];
  }
  const parsed = JSON.parse(trimmed);
  return Array.isArray(parsed) ? parsed : [];
}

function parseJSONObject(value: string): Record<string, unknown> {
  const trimmed = value.trim();
  if (!trimmed) {
    return {};
  }
  const parsed = JSON.parse(trimmed);
  return parsed && typeof parsed === "object" && !Array.isArray(parsed) ? parsed as Record<string, unknown> : {};
}

function uniquePermissions(items: Array<{ resource_type: ResourceType; resource_id: string }>) {
  const seen = new Set<string>();
  return items.filter((item) => {
    const key = `${item.resource_type}:${item.resource_id}`;
    if (seen.has(key)) {
      return false;
    }
    seen.add(key);
    return true;
  });
}

function resourceLabel(permission: AgentPermission, providers: LLMProvider[], resources: RegistryResource[]) {
  if (permission.resource_type === "llm_provider") {
    return providers.find((provider) => provider.id === permission.resource_id)?.name || permission.resource_id;
  }
  return resources.find((resource) => resource.id === permission.resource_id)?.name || permission.resource_id;
}

function formatCost(summary: MeteringSummary | null) {
  if (!summary) {
    return "-";
  }
  const cost = summary.cost ?? 0;
  const currency = summary.currency || "USD";
  return `${currency} ${cost.toFixed(4)}`;
}
