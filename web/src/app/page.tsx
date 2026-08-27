"use client";

import { FormEvent, useEffect, useMemo, useState } from "react";
import { ApiError, ApiState, ApiUser, Squad, apiBaseUrl, apiGet } from "../lib/api";

type Section = "squads" | "agents" | "tasks" | "registry" | "admin";

const navItems: Array<{ id: Section; label: string }> = [
  { id: "squads", label: "Squads" },
  { id: "agents", label: "Agents" },
  { id: "tasks", label: "Tasks" },
  { id: "registry", label: "Registry" },
  { id: "admin", label: "Admin" },
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
  const [refreshTick, setRefreshTick] = useState(0);
  const [user, setUser] = useState<ApiState<ApiUser>>(emptyState);
  const [squads, setSquads] = useState<ApiState<Squad[]>>(emptyState);

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
      setUser(stateFromResult(userResult));
      setSquads(stateFromResult(squadResult, []));
    });

    return () => {
      cancelled = true;
    };
  }, [token, refreshTick]);

  const connected = user.data !== null && user.error === "";
  const stats = useMemo(() => {
    const list = squads.data || [];
    const active = list.filter((item) => item.status !== "deleted").length;
    return [
      { label: "Squads", value: String(active), tone: "strong" },
      { label: "API", value: connected ? "Connected" : "Waiting", tone: connected ? "good" : "warn" },
      { label: "Mode", value: token ? "Bearer" : "Dev", tone: "neutral" },
    ];
  }, [connected, squads.data, token]);

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

  function refresh() {
    setRefreshTick((current) => current + 1);
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
            <button className="secondary" type="button" onClick={refresh}>
              Refresh
            </button>
          </div>

          {activeSection === "squads" && <SquadsView state={squads} />}
          {activeSection === "agents" && <Placeholder title="Agents" detail="Agent fleet views are next." />}
          {activeSection === "tasks" && <Placeholder title="Tasks" detail="Board workflows are next." />}
          {activeSection === "registry" && <Placeholder title="Registry" detail="Provider and plugin grants are next." />}
          {activeSection === "admin" && <AdminView user={user} />}
        </section>
      </section>
    </main>
  );
}

function stateFromResult<T>(result: PromiseSettledResult<T>, fallback: T | null = null): ApiState<T> {
  if (result.status === "fulfilled") {
    return { data: result.value, loading: false, error: "" };
  }
  const reason = result.reason;
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

function SquadsView({ state }: { state: ApiState<Squad[]> }) {
  if (state.loading) {
    return <div className="notice">Loading squads</div>;
  }
  if (state.error) {
    return <div className="notice error">{state.error}</div>;
  }
  const squads = state.data || [];
  if (squads.length === 0) {
    return <div className="notice">No squads yet</div>;
  }
  return (
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
            <tr key={squad.id}>
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
  );
}

function AdminView({ user }: { user: ApiState<ApiUser> }) {
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
