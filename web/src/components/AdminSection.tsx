"use client";

import { Agent, ApiState, ApiUser, AuditEntry, MeteringSummary, Squad } from "../lib/api";
import { StateNotice, formatCost } from "./shared";

export function AdminSection({
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
