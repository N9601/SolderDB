import { useEffect, useMemo, useState } from "react";
import { GetAPIAddr } from "../wailsjs/go/bridge/DBService";
import { apiFetch } from "../lib/apiFetch";

type Method = "GET" | "POST" | "PATCH" | "PUT" | "DELETE";

type Endpoint = {
  id: string;
  method: Method;
  path: string;
  title: string;
  description: string;
  /** Defaults to fill in when picked. `:param` segments turn into editable inputs. */
  defaultBody?: string;
  group: "Auth" | "Collections" | "Records" | "Files" | "KV" | "Realtime" | "Admin";
};

const ENDPOINTS: Endpoint[] = [
  // Auth
  { id: "health", method: "GET", path: "/api/health", title: "Health", description: "Public, returns {ok:true}.", group: "Auth" },
  { id: "register", method: "POST", path: "/api/auth/register", title: "Register", description: "Public, creates a user. First user becomes admin.", group: "Auth", defaultBody: `{\n  "email": "you@example.com",\n  "password": "supersecret"\n}` },
  { id: "login", method: "POST", path: "/api/auth/login", title: "Login", description: "Public, returns a session token.", group: "Auth", defaultBody: `{\n  "email": "you@example.com",\n  "password": "supersecret"\n}` },
  { id: "me", method: "GET", path: "/api/auth/me", title: "Me", description: "Authed, returns the current user.", group: "Auth" },

  // Collections (admin for mutations)
  { id: "list-coll", method: "GET", path: "/api/collections", title: "List collections", description: "Authed, every collection's schema.", group: "Collections" },
  { id: "create-coll", method: "POST", path: "/api/collections", title: "Create collection", description: "Admin only.", group: "Collections", defaultBody: `{\n  "name": "notes",\n  "fields": [\n    {"name": "title", "type": "text", "required": true},\n    {"name": "pinned", "type": "bool"}\n  ]\n}` },
  { id: "get-coll", method: "GET", path: "/api/collections/:name", title: "Get collection", description: "Authed.", group: "Collections" },
  { id: "patch-coll", method: "PATCH", path: "/api/collections/:name", title: "Update schema", description: "Admin.", group: "Collections", defaultBody: `{\n  "fields": [\n    {"name": "title", "type": "text", "required": true}\n  ]\n}` },
  { id: "del-coll", method: "DELETE", path: "/api/collections/:name", title: "Delete collection", description: "Admin, also deletes every record.", group: "Collections" },

  // Records
  { id: "list-recs", method: "GET", path: "/api/collections/:name/records?limit=50", title: "List records", description: "Authed, paginated.", group: "Records" },
  { id: "create-rec", method: "POST", path: "/api/collections/:name/records", title: "Create record", description: "Authed.", group: "Records", defaultBody: `{\n  "title": "hello"\n}` },
  { id: "get-rec", method: "GET", path: "/api/collections/:name/records/:id", title: "Get record", description: "Authed.", group: "Records" },
  { id: "patch-rec", method: "PATCH", path: "/api/collections/:name/records/:id", title: "Update record", description: "Authed, partial patch.", group: "Records", defaultBody: `{\n  "title": "updated"\n}` },
  { id: "del-rec", method: "DELETE", path: "/api/collections/:name/records/:id", title: "Delete record", description: "Authed.", group: "Records" },

  // Files
  { id: "list-files", method: "GET", path: "/api/files?limit=50", title: "List files", description: "Authed.", group: "Files" },
  { id: "get-file", method: "GET", path: "/api/files/:id", title: "Download file", description: "Authed, streams raw bytes.", group: "Files" },
  { id: "del-file", method: "DELETE", path: "/api/files/:id", title: "Delete file", description: "Admin.", group: "Files" },

  // KV
  { id: "kv-get", method: "GET", path: "/api/kv/:key", title: "Get key", description: "Authed.", group: "KV" },
  { id: "kv-put", method: "PUT", path: "/api/kv/:key", title: "Set key", description: "Admin.", group: "KV", defaultBody: `{ "value": "hello" }` },
  { id: "kv-del", method: "DELETE", path: "/api/kv/:key", title: "Delete key", description: "Admin.", group: "KV" },

  // Stats / admin
  { id: "stats", method: "GET", path: "/api/stats", title: "Engine stats", description: "Admin.", group: "Admin" }
];

type Params = Record<string, string>;

type Response = {
  status: number;
  ms: number;
  body: string;
  contentType: string;
};

export default function ApiExplorerView() {
  const [apiAddr, setApiAddr] = useState("");
  const [selectedId, setSelectedId] = useState<string>(ENDPOINTS[0]?.id ?? "");
  const [params, setParams] = useState<Params>({});
  const [body, setBody] = useState<string>("");
  const [sending, setSending] = useState(false);
  const [response, setResponse] = useState<Response | null>(null);

  const selected = useMemo(() => ENDPOINTS.find((e) => e.id === selectedId) ?? ENDPOINTS[0]!, [selectedId]);
  const pathParams = useMemo(() => extractParams(selected.path), [selected]);

  useEffect(() => {
    void (async () => {
      try {
        setApiAddr(await GetAPIAddr());
      } catch {
        // ignore
      }
    })();
  }, []);

  useEffect(() => {
    setBody(selected.defaultBody ?? "");
    setParams({});
    setResponse(null);
  }, [selectedId, selected.defaultBody]);

  const finalPath = useMemo(() => substituteParams(selected.path, params), [selected.path, params]);

  async function onSend() {
    if (!apiAddr) return;
    setSending(true);
    setResponse(null);
    const start = performance.now();
    try {
      const init: RequestInit = {
        method: selected.method,
        headers: body ? { "Content-Type": "application/json" } : undefined,
        body: ["GET", "DELETE"].includes(selected.method) ? undefined : body || undefined
      };
      const res = await apiFetch(apiAddr + finalPath, init);
      const ct = res.headers.get("Content-Type") ?? "";
      const text = await res.text();
      let prettified = text;
      if (ct.includes("application/json")) {
        try {
          prettified = JSON.stringify(JSON.parse(text), null, 2);
        } catch {
          // keep raw
        }
      }
      setResponse({ status: res.status, ms: Math.round(performance.now() - start), body: prettified, contentType: ct });
    } catch (e) {
      setResponse({ status: 0, ms: Math.round(performance.now() - start), body: String(e), contentType: "text/plain" });
    } finally {
      setSending(false);
    }
  }

  const grouped = useMemo(() => {
    const out: Record<string, Endpoint[]> = {};
    for (const e of ENDPOINTS) {
      const k = e.group;
      out[k] ??= [];
      out[k].push(e);
    }
    return out;
  }, []);

  const statusColor =
    !response ? "" :
    response.status === 0 ? "text-danger" :
    response.status < 300 ? "text-ok" :
    response.status < 500 ? "text-warn" : "text-danger";

  return (
    <div className="animate-slideUp grid grid-cols-12 gap-5">
      {/* Endpoints list */}
      <aside className="col-span-3">
        <div className="card overflow-hidden">
          <div className="border-b border-canvas-200 px-4 py-3 section-title">Endpoints</div>
          <div className="max-h-[70vh] overflow-auto">
            {Object.entries(grouped).map(([group, items]) => (
              <div key={group} className="py-1">
                <div className="px-4 py-1.5 text-[10px] font-semibold uppercase tracking-widest text-ink-400">
                  {group}
                </div>
                {items.map((e) => (
                  <button
                    key={e.id}
                    onClick={() => setSelectedId(e.id)}
                    className={`flex w-full items-center gap-2 border-l-2 px-4 py-1.5 text-left text-[12px] transition-colors ${
                      e.id === selectedId
                        ? "border-copper-500 bg-copper-50 text-ink-900"
                        : "border-transparent text-ink-700 hover:bg-canvas-100"
                    }`}
                  >
                    <MethodChip method={e.method} />
                    <span className="truncate">{e.title}</span>
                  </button>
                ))}
              </div>
            ))}
          </div>
        </div>
      </aside>

      {/* Request + response */}
      <section className="col-span-9 space-y-4">
        <div className="card card-pad">
          <div className="flex items-center gap-3">
            <MethodChip method={selected.method} large />
            <div className="font-mono text-[13px] text-ink-900">{selected.path}</div>
          </div>
          <div className="mt-1 text-[12px] text-ink-400">{selected.description}</div>

          {pathParams.length > 0 && (
            <div className="mt-4">
              <div className="label">Path parameters</div>
              <div className="mt-2 grid grid-cols-1 gap-2 md:grid-cols-2">
                {pathParams.map((p) => (
                  <div key={p}>
                    <label className="label">:{p}</label>
                    <input
                      value={params[p] ?? ""}
                      onChange={(e) => setParams({ ...params, [p]: e.target.value })}
                      className="field mt-1"
                      placeholder={p === "name" ? "notes" : p === "id" ? "abc…" : p}
                      spellCheck={false}
                    />
                  </div>
                ))}
              </div>
            </div>
          )}

          {!["GET", "DELETE"].includes(selected.method) && (
            <div className="mt-4">
              <div className="flex items-center justify-between">
                <label className="label">Body (JSON)</label>
                <button
                  className="btn btn-ghost"
                  onClick={() => {
                    try {
                      setBody(JSON.stringify(JSON.parse(body), null, 2));
                    } catch {
                      // ignore
                    }
                  }}
                >
                  {"{ }"} Format
                </button>
              </div>
              <textarea
                value={body}
                onChange={(e) => setBody(e.target.value)}
                className="field mt-1 font-mono"
                rows={10}
                spellCheck={false}
              />
            </div>
          )}

          <div className="mt-4 flex items-center justify-between">
            <div className="font-mono text-[11.5px] text-ink-400">{apiAddr}{finalPath}</div>
            <button className="btn btn-primary" onClick={() => void onSend()} disabled={sending || !apiAddr}>
              {sending ? "Sending…" : "Send →"}
            </button>
          </div>
        </div>

        <div className="card overflow-hidden">
          <div className="flex items-center justify-between border-b border-canvas-200 px-4 py-3">
            <div className="section-title">Response</div>
            {response && (
              <div className="flex items-center gap-3 text-[11.5px]">
                <span className={`font-mono font-semibold ${statusColor}`}>
                  {response.status === 0 ? "ERR" : response.status}
                </span>
                <span className="text-ink-400">{response.ms} ms</span>
                {response.contentType && <span className="chip chip-mono">{response.contentType.split(";")[0]}</span>}
              </div>
            )}
          </div>
          {response ? (
            <pre className="code-block max-h-[400px] m-0 rounded-none border-0 bg-canvas-50">
              {response.body || "(empty)"}
            </pre>
          ) : (
            <div className="px-4 py-10 text-center text-[12px] text-ink-400">
              Click <strong>Send →</strong> to make a request.
            </div>
          )}
        </div>
      </section>
    </div>
  );
}

function MethodChip({ method, large }: { method: Method; large?: boolean }) {
  const colorMap: Record<Method, string> = {
    GET: "bg-steel-100 text-steel-700",
    POST: "bg-copper-50 text-copper-700",
    PATCH: "bg-amber-50 text-amber-700",
    PUT: "bg-amber-50 text-amber-700",
    DELETE: "bg-red-50 text-red-700"
  };
  return (
    <span
      className={`inline-flex items-center rounded-md font-mono font-semibold uppercase tracking-wider ${colorMap[method]} ${
        large ? "px-2.5 py-1 text-[11px]" : "px-1.5 py-0.5 text-[9.5px]"
      }`}
    >
      {method}
    </span>
  );
}

function extractParams(path: string): string[] {
  const matches = path.match(/:([a-zA-Z_]+)/g) ?? [];
  return matches.map((m) => m.slice(1));
}

function substituteParams(path: string, params: Params): string {
  return path.replace(/:([a-zA-Z_]+)/g, (_, name: string) => {
    const v = params[name];
    return v ? encodeURIComponent(v) : ":" + name;
  });
}
