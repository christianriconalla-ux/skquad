"use client";

import { AgentPermission, ApiState, LLMProvider, Message, MeteringSummary, RegistryResource, ResourceType, TaskStatus } from "../lib/api";

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
