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
  namespace?: string;
  status?: string;
  created_at?: string;
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
  const headers: Record<string, string> = {
    Accept: "application/json",
  };
  if (token.trim() !== "") {
    headers.Authorization = `Bearer ${token.trim()}`;
  }

  const response = await fetch(`${apiBaseUrl()}${path}`, {
    headers,
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
  return response.json() as Promise<T>;
}
