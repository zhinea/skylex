const API_BASE = "/api";

function getToken(): string | null {
  if (typeof window === "undefined") return null;
  return localStorage.getItem("skylex_token");
}

export function setToken(token: string) {
  localStorage.setItem("skylex_token", token);
}

export function clearToken() {
  localStorage.removeItem("skylex_token");
}

class ApiError extends Error {
  code: string;
  status: number;

  constructor(message: string, code: string, status: number) {
    super(message);
    this.name = "ApiError";
    this.code = code;
    this.status = status;
  }
}

async function request<T>(path: string, body: unknown): Promise<T> {
  if (typeof window === "undefined") {
    throw new ApiError("Cannot make API requests during SSR", "ssr", 0);
  }

  const token = getToken();
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
    "Connect-Protocol-Version": "1",
  };
  if (token) {
    headers["Authorization"] = `Bearer ${token}`;
  }

  const res = await fetch(`${API_BASE}${path}`, {
    method: "POST",
    headers,
    body: JSON.stringify(body),
  });

  const data = await res.json();

  if (!res.ok) {
    if (res.status === 401) {
      clearToken();
      localStorage.removeItem("skylex_refresh_token");
      window.location.href = "/login";
      throw new ApiError("Session expired", "unauthenticated", 401);
    }
    throw new ApiError(data.message || "Request failed", data.code || "unknown", res.status);
  }

  return data as T;
}

export { ApiError };

export const api = {
  post: request,
};