// SolderDB JavaScript SDK
// ------------------------
// Single-file client for the local SolderDB REST API. Works in browsers and
// Node 18+ (uses global fetch + EventSource). No runtime deps.
//
// Quick start:
//   import { SolderDB } from "solderdb";
//   const db = new SolderDB("http://localhost:8787");
//   await db.auth.login("you@example.com", "secret");
//   const notes = db.collection("notes");
//   const all = await notes.list();
//   const created = await notes.create({ title: "hello" });
//   notes.subscribe((evt) => console.log(evt));

export type Role = "admin" | "user";

export interface User {
  id: string;
  email: string;
  role: Role;
  created: string;
  updated: string;
}

export interface Session {
  token: string;
  user: User;
  expires: string;
}

export interface Field {
  name: string;
  type: "text" | "number" | "bool" | "json" | "date";
  required?: boolean;
  unique?: boolean;
}

export interface CollectionMeta {
  name: string;
  fields: Field[];
  created: string;
  updated: string;
}

export interface Document<T extends Record<string, unknown> = Record<string, unknown>> {
  id: string;
  created: string;
  updated: string;
  data: T;
}

export interface ListResult<T extends Record<string, unknown> = Record<string, unknown>> {
  records: Document<T>[];
  nextAfter: string;
}

export interface FileMeta {
  id: string;
  name: string;
  size: number;
  mimeType: string;
  sha256: string;
  created: string;
}

export type RealtimeEvent<T extends Record<string, unknown> = Record<string, unknown>> = {
  kind: "create" | "update" | "delete";
  collection?: string;
  id?: string;
  key?: string;
  data?: Document<T>;
  timestamp: string;
};

export class SolderDBError extends Error {
  readonly status: number;
  constructor(status: number, message: string) {
    super(message);
    this.status = status;
    this.name = "SolderDBError";
  }
}

interface Store {
  get(): string;
  set(token: string): void;
}

function defaultStore(): Store {
  if (typeof window !== "undefined" && typeof window.localStorage !== "undefined") {
    const KEY = "solderdb.token";
    return {
      get: () => window.localStorage.getItem(KEY) ?? "",
      set: (t) => (t ? window.localStorage.setItem(KEY, t) : window.localStorage.removeItem(KEY))
    };
  }
  let mem = "";
  return { get: () => mem, set: (t) => void (mem = t) };
}

export interface SolderDBOptions {
  /** Override token storage. Defaults to localStorage in browsers, memory in Node. */
  tokenStore?: Store;
  /** fetch implementation. Defaults to globalThis.fetch. */
  fetch?: typeof fetch;
}

export class SolderDB {
  readonly baseURL: string;
  readonly auth: AuthAPI;
  readonly files: FilesAPI;
  readonly admin: AdminAPI;
  private readonly store: Store;
  private readonly fetchImpl: typeof fetch;

  constructor(baseURL: string, opts: SolderDBOptions = {}) {
    this.baseURL = baseURL.replace(/\/$/, "");
    this.store = opts.tokenStore ?? defaultStore();
    this.fetchImpl = opts.fetch ?? globalThis.fetch.bind(globalThis);
    this.auth = new AuthAPI(this);
    this.files = new FilesAPI(this);
    this.admin = new AdminAPI(this);
  }

  /** Returns the API for a specific collection. */
  collection<T extends Record<string, unknown> = Record<string, unknown>>(name: string): CollectionAPI<T> {
    return new CollectionAPI<T>(this, name);
  }

  token(): string {
    return this.store.get();
  }

  setToken(t: string): void {
    this.store.set(t);
  }

  url(path: string): string {
    return this.baseURL + (path.startsWith("/") ? path : "/" + path);
  }

  /** Internal: low-level request helper. */
  async request<T>(path: string, init: RequestInit = {}): Promise<T> {
    const headers = new Headers(init.headers);
    const tok = this.store.get();
    if (tok && !headers.has("Authorization")) {
      headers.set("Authorization", `Bearer ${tok}`);
    }
    if (init.body && !(init.body instanceof FormData) && !headers.has("Content-Type")) {
      headers.set("Content-Type", "application/json");
    }
    const res = await this.fetchImpl(this.url(path), { ...init, headers });
    const text = await res.text();
    let parsed: unknown = null;
    if (text) {
      try {
        parsed = JSON.parse(text) as unknown;
      } catch {
        // raw response
      }
    }
    if (!res.ok) {
      const msg =
        parsed && typeof parsed === "object" && "error" in parsed
          ? String((parsed as { error: unknown }).error)
          : `HTTP ${res.status}`;
      throw new SolderDBError(res.status, msg);
    }
    return parsed as T;
  }

  /** Builds a URL with the auth token in the query string, required for
   *  EventSource and <img>/<a> tags where headers can't be set. */
  authQuery(path: string): string {
    const tok = this.store.get();
    const u = this.url(path);
    if (!tok) return u;
    return u + (u.includes("?") ? "&" : "?") + "token=" + encodeURIComponent(tok);
  }
}

// ---------------- Auth ----------------

class AuthAPI {
  constructor(private readonly db: SolderDB) {}

  async register(email: string, password: string): Promise<Session> {
    const sess = await this.db.request<Session>("/api/auth/register", {
      method: "POST",
      body: JSON.stringify({ email, password })
    });
    this.db.setToken(sess.token);
    return sess;
  }

  async login(email: string, password: string): Promise<Session> {
    const sess = await this.db.request<Session>("/api/auth/login", {
      method: "POST",
      body: JSON.stringify({ email, password })
    });
    this.db.setToken(sess.token);
    return sess;
  }

  async me(): Promise<User> {
    return this.db.request<User>("/api/auth/me");
  }

  logout(): void {
    this.db.setToken("");
  }
}

// ---------------- Collections ----------------

class CollectionAPI<T extends Record<string, unknown>> {
  constructor(private readonly db: SolderDB, public readonly name: string) {}

  list(opts: { after?: string; limit?: number } = {}): Promise<ListResult<T>> {
    const q = new URLSearchParams();
    if (opts.after) q.set("after", opts.after);
    if (opts.limit) q.set("limit", String(opts.limit));
    const qs = q.toString();
    return this.db.request<ListResult<T>>(`/api/collections/${this.name}/records${qs ? "?" + qs : ""}`);
  }

  get(id: string): Promise<Document<T>> {
    return this.db.request<Document<T>>(`/api/collections/${this.name}/records/${encodeURIComponent(id)}`);
  }

  create(data: Partial<T>): Promise<Document<T>> {
    return this.db.request<Document<T>>(`/api/collections/${this.name}/records`, {
      method: "POST",
      body: JSON.stringify(data)
    });
  }

  update(id: string, patch: Partial<T>): Promise<Document<T>> {
    return this.db.request<Document<T>>(`/api/collections/${this.name}/records/${encodeURIComponent(id)}`, {
      method: "PATCH",
      body: JSON.stringify(patch)
    });
  }

  delete(id: string): Promise<{ deleted: string }> {
    return this.db.request<{ deleted: string }>(`/api/collections/${this.name}/records/${encodeURIComponent(id)}`, {
      method: "DELETE"
    });
  }

  /** Subscribe to live changes on this collection. Returns an unsubscribe function. */
  subscribe(handler: (evt: RealtimeEvent<T>) => void): () => void {
    if (typeof EventSource === "undefined") {
      throw new Error("subscribe() requires EventSource (browser or Node 22+)");
    }
    const url = this.db.authQuery(`/api/realtime?topic=coll:${encodeURIComponent(this.name)}`);
    const es = new EventSource(url);
    const wrap = (e: MessageEvent) => {
      try {
        handler(JSON.parse(e.data) as RealtimeEvent<T>);
      } catch {
        // ignore malformed payload
      }
    };
    es.addEventListener("create", wrap as EventListener);
    es.addEventListener("update", wrap as EventListener);
    es.addEventListener("delete", wrap as EventListener);
    return () => es.close();
  }
}

// ---------------- Admin (collection schema management) ----------------

class AdminAPI {
  constructor(private readonly db: SolderDB) {}

  listCollections(): Promise<CollectionMeta[]> {
    return this.db.request<CollectionMeta[]>("/api/collections");
  }

  createCollection(meta: Omit<CollectionMeta, "created" | "updated">): Promise<CollectionMeta> {
    return this.db.request<CollectionMeta>("/api/collections", {
      method: "POST",
      body: JSON.stringify(meta)
    });
  }

  deleteCollection(name: string): Promise<{ deleted: string }> {
    return this.db.request<{ deleted: string }>(`/api/collections/${encodeURIComponent(name)}`, {
      method: "DELETE"
    });
  }

  stats(): Promise<Record<string, unknown>> {
    return this.db.request<Record<string, unknown>>("/api/stats");
  }
}

// ---------------- Files ----------------

class FilesAPI {
  constructor(private readonly db: SolderDB) {}

  list(opts: { after?: string; limit?: number } = {}): Promise<{ files: FileMeta[]; nextAfter: string }> {
    const q = new URLSearchParams();
    if (opts.after) q.set("after", opts.after);
    if (opts.limit) q.set("limit", String(opts.limit));
    const qs = q.toString();
    return this.db.request<{ files: FileMeta[]; nextAfter: string }>(`/api/files${qs ? "?" + qs : ""}`);
  }

  upload(file: File | Blob, filename?: string): Promise<FileMeta> {
    const form = new FormData();
    const name = filename ?? (file instanceof File ? file.name : "upload.bin");
    form.append("file", file, name);
    return this.db.request<FileMeta>("/api/files", { method: "POST", body: form });
  }

  delete(id: string): Promise<{ deleted: string }> {
    return this.db.request<{ deleted: string }>(`/api/files/${encodeURIComponent(id)}`, { method: "DELETE" });
  }

  /** Returns a fully-qualified URL for the file, including the auth token in
   *  the query so it can be used as <img src> or <a href>. */
  url(id: string): string {
    return this.db.authQuery(`/api/files/${encodeURIComponent(id)}`);
  }
}
