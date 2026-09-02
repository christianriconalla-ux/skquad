"use client";

import { Agent, ApiState, AuditEntry, BoardPayload, MeteringSummary } from "../lib/api";
import { StateNotice, formatCost, formatRelativeTime, leaseState } from "./shared";

export function SquadCockpit({
  agents,
  board,
  metering,
  audit,
}: {
  agents: ApiState<Agent[]>;
  board: ApiState<BoardPayload>;
  metering: ApiState<MeteringSummary>;
  audit: ApiState<AuditEntry[]>;
}) {
  const agentItems = agents.data || [];
  const tasks = board.data?.tasks || [];
  const running = tasks.filter((task) => leaseState(task) === "running").length;
  const stalled = tasks.filter((task) => leaseState(task) === "stalled").length;
  const blocked = tasks.filter((task) => task.status === "blocked").length;
  const open = tasks.filter((task) => task.status !== "done").length;
  const errored = agentItems.filter((agent) => isErrorStatus(agent.status)).length;
  const busy = agentItems.filter((agent) => agent.status === "busy").length;

  return (
    <div className="cockpit">
      <div className="metric-grid">
        <article className={errored > 0 ? "metric attention" : "metric"}>
          <span className="metric-label">Agents</span>
          <strong className="metric-value">{agentItems.length}</strong>
          <span className="metric-sub">
            {busy} busy · {agentItems.length - busy - errored} idle
            {errored > 0 && <b className="metric-flag"> · {errored} error</b>}
          </span>
        </article>

        <article className={stalled > 0 ? "metric attention" : "metric"}>
          <span className="metric-label">Work in flight</span>
          <strong className="metric-value">{running}</strong>
          <span className="metric-sub">
            {open} open · {blocked} blocked
            {stalled > 0 && <b className="metric-flag"> · {stalled} stalled</b>}
          </span>
        </article>

        <article className="metric">
          <span className="metric-label">Squad spend</span>
          <strong className="metric-value">{metering.loading ? "…" : metering.error ? "-" : formatCost(metering.data)}</strong>
          {/* Metering and audit are owner/platform-admin endpoints; a squad member
              with a read grant legitimately gets 403 here, which is not a fault. */}
          <span className={metering.error ? "metric-sub muted" : "metric-sub"}>
            {metering.error ? "owner and platform admins only" : formatTokens(metering.data)}
          </span>
        </article>

        <article className="metric">
          <span className="metric-label">Tasks done</span>
          <strong className="metric-value">{tasks.filter((task) => task.status === "done").length}</strong>
          <span className="metric-sub">of {tasks.length} total</span>
        </article>
      </div>

      <section className="feed">
        <h3 className="panel-title">Recent activity</h3>
        {audit.error ? (
          <div className="notice compact muted">Activity details are limited to the squad owner and platform admins.</div>
        ) : (
          <StateNotice state={audit} empty="No recorded activity for this squad" />
        )}
        <div className="feed-list">
          {(audit.data || []).slice(0, 12).map((entry) => (
            <article className="feed-row" key={entry.id}>
              <span className={entry.actor_type === "agent" ? "actor agent" : "actor user"}>{entry.actor_type}</span>
              <span className="feed-text">
                <strong>{entry.action}</strong> {entry.resource_type}
                <small>{entry.actor_id}</small>
              </span>
              <span className="feed-time">{formatRelativeTime(entry.timestamp)}</span>
            </article>
          ))}
        </div>
      </section>
    </div>
  );
}

function isErrorStatus(status?: string): boolean {
  return status === "error" || status === "failed";
}

function formatTokens(summary: MeteringSummary | null): string {
  if (!summary) {
    return "no usage recorded";
  }
  const input = summary.input_tokens ?? 0;
  const output = summary.output_tokens ?? 0;
  if (input === 0 && output === 0) {
    return "no usage recorded";
  }
  return `${compact(input)} in · ${compact(output)} out`;
}

function compact(value: number): string {
  return new Intl.NumberFormat(undefined, { notation: "compact", maximumFractionDigits: 1 }).format(value);
}
