"use client";

import { AgentPermission, ApiState, LLMProvider, Message, MeteringSummary, RegistryResource, ResourceType, Task, TaskStatus } from "../lib/api";

export const taskStatuses: TaskStatus[] = ["todo", "in-progress", "in-review", "done", "blocked"];

export const registryTypes: Array<{ type: Exclude<ResourceType, "llm_provider">; label: string; path: string }> = [
  { type: "skill", label: "Skills", path: "skills" },
  { type: "tool", label: "Tools", path: "tools" },
  { type: "api", label: "APIs", path: "apis" },
  { type: "knowledge_base", label: "Knowledge Bases", path: "knowledge-bases" },
  { type: "project_workspace", label: "Project Workspaces", path: "project-workspaces" },
];

export function StateNotice<T>({ state, empty }: { state: ApiState<T>; empty: string }) {
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

export function messageText(message: Message) {
  const payload = message.payload || {};
  if (typeof payload.message === "string") {
    return payload.message;
  }
  return JSON.stringify(payload);
}

export function resourceLabel(permission: AgentPermission, providers: LLMProvider[], resources: RegistryResource[]) {
  if (permission.resource_type === "llm_provider") {
    return providers.find((provider) => provider.id === permission.resource_id)?.name || permission.resource_id;
  }
  return resources.find((resource) => resource.id === permission.resource_id)?.name || permission.resource_id;
}

export function formatCost(summary: MeteringSummary | null) {
  if (!summary) {
    return "-";
  }
  const cost = summary.cost ?? 0;
  const currency = summary.currency || "USD";
  return `${currency} ${cost.toFixed(4)}`;
}

export type LeaseState = "running" | "stalled" | "idle";

// A task holds a lease while an agent runtime is actively working it. An
// expired lease means the worker stopped heartbeating without completing.
export function leaseState(task: Task): LeaseState {
  if (!task.execution_id || !task.lease_expires_at) {
    return "idle";
  }
  const expiry = Date.parse(task.lease_expires_at);
  // Go serialises a zero time.Time as 0001-01-01T00:00:00Z rather than omitting
  // it, and that string is truthy but parses to a large negative timestamp.
  // Anything before the epoch means "no lease", not "lease expired long ago".
  if (Number.isNaN(expiry) || expiry <= 0) {
    return "idle";
  }
  return expiry > Date.now() ? "running" : "stalled";
}

export function formatRelativeTime(value?: string): string {
  if (!value) {
    return "";
  }
  const timestamp = Date.parse(value);
  if (Number.isNaN(timestamp)) {
    return "";
  }
  const deltaSec = Math.round((timestamp - Date.now()) / 1000);
  const absSec = Math.abs(deltaSec);
  const [amount, unit]: [number, Intl.RelativeTimeFormatUnit] =
    absSec < 60 ? [deltaSec, "second"]
    : absSec < 3600 ? [Math.round(deltaSec / 60), "minute"]
    : absSec < 86400 ? [Math.round(deltaSec / 3600), "hour"]
    : [Math.round(deltaSec / 86400), "day"];
  return new Intl.RelativeTimeFormat(undefined, { numeric: "auto" }).format(amount, unit);
}

export function messageDeliveryNote(message: Message): string {
  const parts: string[] = [];
  if (typeof message.attempts === "number" && message.attempts > 0) {
    const max = message.max_attempts ? `/${message.max_attempts}` : "";
    parts.push(`attempt ${message.attempts}${max}`);
  }
  if (message.next_retry_at) {
    // A retry time already in the past means the queue is behind, not that a
    // retry happened — say it is due rather than reporting it as history.
    const due = Date.parse(message.next_retry_at);
    parts.push(
      !Number.isNaN(due) && due <= Date.now() ? "retry due" : `retry ${formatRelativeTime(message.next_retry_at)}`,
    );
  }
  if (message.expires_at) {
    parts.push(`expires ${formatRelativeTime(message.expires_at)}`);
  }
  return parts.join(" · ");
}
