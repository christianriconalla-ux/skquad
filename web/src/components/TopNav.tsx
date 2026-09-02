"use client";

import { FormEvent, useEffect, useRef, useState } from "react";
import { ApiState, ApiUser } from "../lib/api";

export function TopNav({
  user,
  connected,
  mode,
  openTasks,
  draftToken,
  setDraftToken,
  onSaveToken,
}: {
  user: ApiState<ApiUser>;
  connected: boolean;
  mode: string;
  openTasks: number | null;
  draftToken: string;
  setDraftToken: (value: string) => void;
  onSaveToken: (event: FormEvent<HTMLFormElement>) => void;
}) {
  const [menuOpen, setMenuOpen] = useState(false);
  const wrapRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!menuOpen) {
      return;
    }
    function onPointerDown(event: MouseEvent) {
      if (wrapRef.current && !wrapRef.current.contains(event.target as Node)) {
        setMenuOpen(false);
      }
    }
    window.addEventListener("mousedown", onPointerDown);
    return () => {
      window.removeEventListener("mousedown", onPointerDown);
    };
  }, [menuOpen]);

  return (
    <header className="topnav">
      <div className="brand">
        <span className="brand-mark">sq</span>
        <span>
          <strong>skquad</strong>
          <small>control plane</small>
        </span>
      </div>
      <div className="topnav-status">
        {openTasks !== null && (
          <span className="status-pill accent">Open tasks · {openTasks}</span>
        )}
        <span className="status-pill">Mode · {mode}</span>
        <span className={connected ? "status-pill good" : "status-pill warn"}>
          <span className={connected ? "status-dot online" : "status-dot"} />
          {connected ? "API connected" : "API waiting"}
        </span>
      </div>
      <div className="profile-wrap" ref={wrapRef}>
        <button
          type="button"
          className="profile-button"
          aria-label="User profile"
          aria-expanded={menuOpen}
          onClick={() => setMenuOpen((open) => !open)}
        >
          {userInitials(user)}
        </button>
        {menuOpen && (
          <div className="profile-menu">
            <div className="profile-identity">
              <span className="profile-button">{userInitials(user)}</span>
              <span>
                <strong>{userName(user)}</strong>
                <small>{user.data?.email || "No active session"}</small>
              </span>
            </div>
            <div className="profile-role">
              <span>Role</span>
              <strong>{user.data?.role || "-"}</strong>
            </div>
            <form className="token-form" onSubmit={onSaveToken}>
              <label htmlFor="auth-token">API token</label>
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
        )}
      </div>
    </header>
  );
}

function userName(state: ApiState<ApiUser>): string {
  if (state.loading) {
    return "Checking session";
  }
  if (state.data) {
    return state.data.name || state.data.email;
  }
  return "Not connected";
}

function userInitials(state: ApiState<ApiUser>): string {
  const name = state.data?.name || state.data?.email || "";
  if (!name) {
    return "?";
  }
  const parts = name.replace(/@.*$/, "").split(/[\s._-]+/).filter(Boolean);
  const initials = parts.slice(0, 2).map((part) => part[0].toUpperCase()).join("");
  return initials || "?";
}
