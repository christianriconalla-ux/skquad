export type ApiUser = {
  id: string;
  email: string;
  name: string;
  role: string;
};

export type Squad = {
  id: string;
  name: string;
  mission?: string;
  owner_id?: string;
  namespace?: string;
  status?: string;
  created_at?: string;
};

export type Agent = {
  id: string;
  squad_id: string;
  name: string;
  role?: string;
  identity_id?: string;
  default_provider_id?: string;
  default_model?: string;
  idle_timeout_sec?: number;
  status?: string;
  created_at?: string;
  updated_at?: string;
};

export type AgentIdentity = {
  id: string;
  agent_id: string;
  credential_ref: string;
  virtual_key_ref?: string;
  created_by: string;
  created_at?: string;
  rotated_at?: string;
};

export type TaskStatus = "todo" | "in-progress" | "in-review" | "done" | "blocked";

export type Task = {
  id: string;
  board_id: string;
  squad_id: string;
  title: string;
  description?: string;
  status: TaskStatus;
  assignee_agent_id?: string;
  created_by_type?: string;
  created_by_id?: string;
  position?: number;
  created_at?: string;
  updated_at?: string;
};

export type BoardPayload = {
  board: {
    id: string;
    squad_id: string;
    created_at?: string;
  };
  tasks: Task[];
};

export type Message = {
  id: string;
  from_type: string;
  from_id: string;
  to_agent_id: string;
  squad_id: string;
  type: string;
  payload?: {
    message?: string;
    [key: string]: unknown;
  };
  status: string;
  correlation_id?: string;
  created_at?: string;
  delivered_at?: string;
};

export type ApiState<T> = {
  data: T | null;
  loading: boolean;
  error: string;
};

export function apiBaseUrl(): string {
  const configured = process.env.NEXT_PUBLIC_SKQUAD_API_BASE_URL || "/api/v1";
  return configured.replace(/\/$/, "");
}

export class ApiError extends Error {
  status: number;

  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

export async function apiGet<T>(path: string, token: string): Promise<T> {
  return apiRequest<T>(path, token, { method: "GET" });
}

export async function apiPost<T>(path: string, token: string, body: unknown): Promise<T> {
  return apiRequest<T>(path, token, { method: "POST", body });
}

export async function apiPatch<T>(path: string, token: string, body: unknown): Promise<T> {
  return apiRequest<T>(path, token, { method: "PATCH", body });
}

export async function apiDelete(path: string, token: string): Promise<void> {
  await apiRequest<void>(path, token, { method: "DELETE" });
}

async function apiRequest<T>(path: string, token: string, options: { method: string; body?: unknown }): Promise<T> {
  const headers: Record<string, string> = {
    Accept: "application/json",
  };
  if (options.body !== undefined) {
    headers["Content-Type"] = "application/json";
  }
  if (token.trim() !== "") {
    headers.Authorization = `Bearer ${token.trim()}`;
  }

  const response = await fetch(`${apiBaseUrl()}${path}`, {
    method: options.method,
    headers,
    body: options.body === undefined ? undefined : JSON.stringify(options.body),
    credentials: "same-origin",
    cache: "no-store",
  });
  if (!response.ok) {
    let message = response.statusText;
    try {
      const body = await response.json();
      message = body?.error?.message || message;
    } catch {
      // Keep the HTTP status text when the body is not JSON.
    }
    throw new ApiError(response.status, message);
  }
  if (response.status === 204) {
    return undefined as T;
  }
  return response.json() as Promise<T>;
}
