import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { GetAPIAddr } from "../wailsjs/go/bridge/DBService";
import { apiJSON, withAuthQuery } from "../lib/apiFetch";

type LogEntry = {
  timestamp: string;
  method: string;
  path: string;
  status: number;
  durationMs: number;
  user?: string;
  remote?: string;
};

type Filter = "all" | "errors" | "writes";

export default function LogsView() {
  const [apiAddr, setApiAddr] = useState("");
  const [entries, setEntries] = useState<LogEntry[]>([]);
  const [filter, setFilter] = useState<Filter>("all");
  const [paused, setPaused] = useState(false);
  const pausedRef = useRef(paused);
  pausedRef.current = paused;

  useEffect(() => {
    void (async () => {
      try {
        setApiAddr(await GetAPIAddr());
      } catch {
        // ignore
      }
    })();
  }, []);

  const refresh = useCallback(async () => {
    if (!apiAddr) return;
    try {
      const tail = await apiJSON<LogEntry[]>(`${apiAddr}/api/logs?limit=500`);
      setEntries(tail ?? []);
    } catch {
      // ignore — likely not admin
    }
  }, [apiAddr]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  // Live tail via SSE on the logs topic.
  useEffect(() => {
    if (!apiAddr) return;
    const es = new EventSource(withAuthQuery(`${apiAddr}/api/realtime?topic=logs`));
    es.addEventListener("create", (raw) => {
      if (pausedRef.current) return;
      try {
        const payload = JSON.parse((raw as MessageEvent).data) as { data?: LogEntry };
        if (payload.data) {
          setEntries((prev) => [payload.data!, ...prev].slice(0, 500));
        }
      } catch {
        // ignore
      }
    });
    return () => es.close();
  }, [apiAddr]);

  const filtered = useMemo(() => {
    switch (filter) {
      case "errors":
        return entries.filter((e) => e.status === 0 || e.status >= 400);
      case "writes":
        return entries.filter((e) => ["POST", "PATCH", "PUT", "DELETE"].includes(e.method));
      default:
        return entries;
    }
  }, [entries, filter]);

  return (
    <div className="animate-slideUp space-y-4">
      <div className="card card-pad">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div>
            <div className="section-title">Activity</div>
            <div className="section-sub">Live tail of every API request hitting this database.</div>
          </div>
          <div className="flex items-center gap-2">
            <FilterPill active={filter === "all"} onClick={() => setFilter("all")}>All</FilterPill>
            <FilterPill active={filter === "writes"} onClick={() => setFilter("writes")}>Writes</FilterPill>
            <FilterPill active={filter === "errors"} onClick={() => setFilter("errors")}>Errors</FilterPill>
            <button className={`btn ${paused ? "btn-primary" : ""}`} onClick={() => setPaused((p) => !p)}>
              {paused ? "▶ Resume" : "⏸ Pause"}
            </button>
            <button className="btn" onClick={() => void refresh()}>Refresh</button>
          </div>
        </div>
      </div>

      <div className="card overflow-hidden">
        <div className="grid grid-cols-12 gap-2 border-b border-canvas-200 bg-canvas-100 px-4 py-2 text-[10.5px] font-semibold uppercase tracking-wider text-ink-400">
          <div className="col-span-2">Time</div>
          <div className="col-span-1">Method</div>
          <div className="col-span-1">Status</div>
          <div className="col-span-5">Path</div>
          <div className="col-span-2">User</div>
          <div className="col-span-1 text-right">ms</div>
        </div>
        <div className="max-h-[65vh] overflow-auto font-mono text-[11.5px]">
          {filtered.length === 0 ? (
            <div className="px-4 py-10 text-center text-ink-400">No activity yet — make a request.</div>
          ) : (
            filtered.map((e, i) => (
              <div key={i} className="grid grid-cols-12 gap-2 border-b border-canvas-150 px-4 py-1.5 hover:bg-canvas-100">
                <div className="col-span-2 truncate text-ink-400">{fmtTime(e.timestamp)}</div>
                <div className="col-span-1">
                  <MethodPill method={e.method} />
                </div>
                <div className={`col-span-1 font-semibold ${statusColor(e.status)}`}>{e.status === 0 ? "ERR" : e.status}</div>
                <div className="col-span-5 truncate text-ink-900" title={e.path}>{e.path}</div>
                <div className="col-span-2 truncate text-ink-500">{e.user || "—"}</div>
                <div className="col-span-1 text-right text-ink-500">{e.durationMs}</div>
              </div>
            ))
          )}
        </div>
      </div>
    </div>
  );
}

function FilterPill({ active, onClick, children }: { active: boolean; onClick: () => void; children: React.ReactNode }) {
  return (
    <button
      onClick={onClick}
      className={`rounded-md border px-3 py-1.5 text-[12px] font-medium transition-colors ${
        active
          ? "border-copper-500 bg-copper-50 text-copper-700"
          : "border-canvas-200 bg-white text-ink-500 hover:bg-canvas-100"
      }`}
    >
      {children}
    </button>
  );
}

function MethodPill({ method }: { method: string }) {
  const color =
    method === "GET" ? "text-steel-700" :
    method === "POST" ? "text-copper-700" :
    method === "PATCH" || method === "PUT" ? "text-amber-700" :
    method === "DELETE" ? "text-red-700" : "text-ink-500";
  return <span className={`font-semibold ${color}`}>{method}</span>;
}

function statusColor(s: number): string {
  if (s === 0) return "text-danger";
  if (s < 300) return "text-ok";
  if (s < 400) return "text-steel-700";
  if (s < 500) return "text-warn";
  return "text-danger";
}

function fmtTime(iso: string): string {
  if (!iso) return "—";
  try {
    const d = new Date(iso);
    return d.toLocaleTimeString(undefined, { hour12: false }) + "." + String(d.getMilliseconds()).padStart(3, "0");
  } catch {
    return iso;
  }
}
