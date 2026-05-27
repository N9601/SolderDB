// Lightweight wrapper around fetch that injects the SolderDB bearer token
// and surfaces { error } responses as thrown Errors.
//
// Why this exists: every frontend call to the local REST API needs the
// Authorization header now. Centralizing here means UI code can keep using
// fetch-style calls without remembering the header.

const TOKEN_KEY = "solderdb.token";

export function getToken(): string {
  return window.localStorage.getItem(TOKEN_KEY) ?? "";
}

export function setToken(v: string): void {
  if (v) window.localStorage.setItem(TOKEN_KEY, v);
  else window.localStorage.removeItem(TOKEN_KEY);
}

/**
 * Append `?token=<bearer>` to a URL — required for EventSource subscriptions
 * because the browser EventSource API can't set custom headers.
 */
export function withAuthQuery(url: string): string {
  const t = getToken();
  if (!t) return url;
  const sep = url.includes("?") ? "&" : "?";
  return `${url}${sep}token=${encodeURIComponent(t)}`;
}

export async function apiFetch(input: string, init?: RequestInit): Promise<Response> {
  const headers = new Headers(init?.headers);
  const token = getToken();
  if (token && !headers.has("Authorization")) {
    headers.set("Authorization", `Bearer ${token}`);
  }
  return fetch(input, { ...init, headers });
}

/**
 * apiJSON throws on non-2xx responses with the server-supplied { error }
 * message attached. Returns the parsed JSON body on success.
 */
export async function apiJSON<T>(input: string, init?: RequestInit): Promise<T> {
  const res = await apiFetch(input, init);
  const text = await res.text();
  let parsed: unknown = null;
  if (text) {
    try {
      parsed = JSON.parse(text) as unknown;
    } catch {
      // non-JSON; keep parsed null
    }
  }
  if (!res.ok) {
    const msg =
      parsed && typeof parsed === "object" && parsed !== null && "error" in parsed
        ? String((parsed as { error: unknown }).error)
        : `HTTP ${res.status}`;
    throw new Error(msg);
  }
  return parsed as T;
}
