import { useEffect, useRef, useState } from "react";
import {
  Compact,
  Delete,
  Get,
  GetAPIAddr,
  GetStats,
  Scan,
  Set as SetKV,
  Snapshot
} from "./wailsjs/go/bridge/DBService";
import { bridge } from "./wailsjs/go/models";
import { Logo } from "./components/Logo";
import CollectionsView from "./views/CollectionsView";
import AuthView from "./views/AuthView";
import { VerifyToken } from "./wailsjs/go/bridge/AuthService";
import type { bridge as bridgeNS } from "./wailsjs/go/models";

type DBStats = bridge.Stats;

type Row = {
  key: string;
  preview: string;
  type: ValueType;
  loading: boolean;
};

type ValueType = "json" | "number" | "text" | "empty";

type NavId = "dashboard" | "collections" | "console" | "browser" | "snapshots" | "settings";

const PAGE_SIZE = 50;
const PREVIEW_BYTES = 80;

function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return "0 B";
  const units = ["B", "KB", "MB", "GB"] as const;
  let v = bytes;
  let i = 0;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return `${v.toFixed(i === 0 ? 0 : 1)} ${units[i]}`;
}

function truncate(s: string, n: number): string {
  if (s.length <= n) return s;
  return s.slice(0, n) + "…";
}

function tryFormatJSON(s: string): { formatted: string; ok: boolean } {
  if (!s.trim()) return { formatted: s, ok: false };
  try {
    const parsed = JSON.parse(s) as unknown;
    return { formatted: JSON.stringify(parsed, null, 2), ok: true };
  } catch {
    return { formatted: s, ok: false };
  }
}

function detectType(s: string): ValueType {
  if (!s) return "empty";
  const t = s.trim();
  if (t.length === 0) return "empty";
  const first = t[0];
  const last = t[t.length - 1];
  if ((first === "{" && last === "}") || (first === "[" && last === "]")) {
    try {
      JSON.parse(t);
      return "json";
    } catch {
      // fall through
    }
  }
  if (!Number.isNaN(Number(t)) && /^-?[\d.eE+-]+$/.test(t)) return "number";
  return "text";
}

const NAV: { id: NavId; label: string; icon: JSX.Element }[] = [
  {
    id: "dashboard",
    label: "Dashboard",
    icon: (
      <svg className="nav-item-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7">
        <rect x="3" y="3" width="7" height="9" rx="1.5" />
        <rect x="14" y="3" width="7" height="5" rx="1.5" />
        <rect x="14" y="12" width="7" height="9" rx="1.5" />
        <rect x="3" y="16" width="7" height="5" rx="1.5" />
      </svg>
    )
  },
  {
    id: "collections",
    label: "Collections",
    icon: (
      <svg className="nav-item-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7">
        <rect x="3" y="4" width="7" height="7" rx="1.5" />
        <rect x="14" y="4" width="7" height="7" rx="1.5" />
        <rect x="3" y="13" width="7" height="7" rx="1.5" />
        <rect x="14" y="13" width="7" height="7" rx="1.5" />
      </svg>
    )
  },
  {
    id: "console",
    label: "Console",
    icon: (
      <svg className="nav-item-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7">
        <polyline points="4 7 9 12 4 17" />
        <line x1="12" y1="17" x2="20" y2="17" />
      </svg>
    )
  },
  {
    id: "browser",
    label: "Data Browser",
    icon: (
      <svg className="nav-item-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7">
        <ellipse cx="12" cy="5" rx="8" ry="2.5" />
        <path d="M4 5v6c0 1.4 3.6 2.5 8 2.5s8-1.1 8-2.5V5" />
        <path d="M4 11v6c0 1.4 3.6 2.5 8 2.5s8-1.1 8-2.5v-6" />
      </svg>
    )
  },
  {
    id: "snapshots",
    label: "Snapshots",
    icon: (
      <svg className="nav-item-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7">
        <rect x="3" y="6" width="18" height="14" rx="2" />
        <circle cx="12" cy="13" r="3.5" />
        <path d="M8 6l2-2h4l2 2" />
      </svg>
    )
  },
  {
    id: "settings",
    label: "Settings",
    icon: (
      <svg className="nav-item-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7">
        <circle cx="12" cy="12" r="3" />
        <path d="M19.4 15a1.7 1.7 0 0 0 .3 1.8l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.7 1.7 0 0 0-1.8-.3 1.7 1.7 0 0 0-1 1.5V21a2 2 0 1 1-4 0v-.1A1.7 1.7 0 0 0 9 19.4a1.7 1.7 0 0 0-1.8.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1.7 1.7 0 0 0 .3-1.8 1.7 1.7 0 0 0-1.5-1H3a2 2 0 1 1 0-4h.1A1.7 1.7 0 0 0 4.6 9a1.7 1.7 0 0 0-.3-1.8l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1.7 1.7 0 0 0 1.8.3H9a1.7 1.7 0 0 0 1-1.5V3a2 2 0 1 1 4 0v.1a1.7 1.7 0 0 0 1 1.5 1.7 1.7 0 0 0 1.8-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.7 1.7 0 0 0-.3 1.8V9a1.7 1.7 0 0 0 1.5 1H21a2 2 0 1 1 0 4h-.1a1.7 1.7 0 0 0-1.5 1z" />
      </svg>
    )
  }
];

const TOKEN_KEY = "solderdb.token";

type AuthState =
  | { state: "loading" }
  | { state: "signedOut" }
  | { state: "signedIn"; user: bridgeNS.User; token: string };

export default function App() {
  const [auth, setAuth] = useState<AuthState>({ state: "loading" });

  useEffect(() => {
    const token = window.localStorage.getItem(TOKEN_KEY);
    if (!token) {
      setAuth({ state: "signedOut" });
      return;
    }
    void (async () => {
      try {
        const user = await VerifyToken(token);
        setAuth({ state: "signedIn", user, token });
      } catch {
        window.localStorage.removeItem(TOKEN_KEY);
        setAuth({ state: "signedOut" });
      }
    })();
  }, []);

  if (auth.state === "loading") {
    return (
      <div className="flex h-screen w-screen items-center justify-center bg-canvas-50">
        <Logo size={32} />
      </div>
    );
  }
  if (auth.state === "signedOut") {
    return (
      <AuthView
        onSignedIn={(sess) => {
          window.localStorage.setItem(TOKEN_KEY, sess.token);
          setAuth({ state: "signedIn", user: sess.user, token: sess.token });
        }}
      />
    );
  }

  return (
    <AppShell
      user={auth.user}
      onSignOut={() => {
        window.localStorage.removeItem(TOKEN_KEY);
        setAuth({ state: "signedOut" });
      }}
    />
  );
}

function AppShell(props: { user: bridgeNS.User; onSignOut: () => void }) {
  const [nav, setNav] = useState<NavId>("dashboard");

  const [key, setKey] = useState<string>("");
  const [value, setValue] = useState<string>("");
  const [readValue, setReadValue] = useState<string>("");
  const [status, setStatus] = useState<string>("Ready");
  const [stats, setStats] = useState<DBStats | null>(null);

  const [keyPrefix, setKeyPrefix] = useState<string>("");
  const [rows, setRows] = useState<Row[]>([]);
  const [scanAfter, setScanAfter] = useState<string>("");
  const [scanNextAfter, setScanNextAfter] = useState<string>("");
  const [selectedKey, setSelectedKey] = useState<string>("");

  const [writePulse, setWritePulse] = useState<boolean>(false);
  const [apiAddr, setApiAddr] = useState<string>("");
  const ledTimer = useRef<number | null>(null);

  function pulseWrite() {
    setWritePulse(true);
    if (ledTimer.current !== null) window.clearTimeout(ledTimer.current);
    ledTimer.current = window.setTimeout(() => setWritePulse(false), 700);
  }

  async function refreshStats() {
    try {
      setStats(await GetStats());
    } catch (e) {
      setStatus(`Stats error: ${String(e)}`);
    }
  }

  async function refreshKeys() {
    try {
      const res = await Scan({
        prefix: keyPrefix,
        after: scanAfter,
        limit: PAGE_SIZE
      } as bridge.ScanOptions);
      const keys = res.keys ?? [];
      setScanNextAfter(res.nextAfter ?? "");
      const initial: Row[] = keys.map((k) => ({ key: k, preview: "", type: "empty", loading: true }));
      setRows(initial);
      const previews = await Promise.all(keys.map((k) => Get(k).catch(() => "")));
      setRows(
        keys.map((k, i) => {
          const v = previews[i] ?? "";
          return {
            key: k,
            preview: truncate(v, PREVIEW_BYTES),
            type: detectType(v),
            loading: false
          };
        })
      );
    } catch (e) {
      setStatus(`Scan error: ${String(e)}`);
    }
  }

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
    void refreshStats();
    void refreshKeys();
    const id = window.setInterval(() => {
      void refreshStats();
    }, 1000);
    return () => {
      window.clearInterval(id);
      if (ledTimer.current !== null) window.clearTimeout(ledTimer.current);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    void refreshKeys();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [keyPrefix, scanAfter]);

  async function onGet() {
    if (!key) {
      setStatus("Key required");
      return;
    }
    try {
      const v = await Get(key);
      setReadValue(v);
      setStatus(v ? "Read OK" : "Key not found");
    } catch (e) {
      setStatus(`Get error: ${String(e)}`);
    }
  }

  async function onSet() {
    if (!key) {
      setStatus("Key required");
      return;
    }
    try {
      await SetKV(key, value);
      pulseWrite();
      setStatus(`Wrote ${key}`);
      await Promise.all([refreshStats(), refreshKeys()]);
    } catch (e) {
      setStatus(`Set error: ${String(e)}`);
    }
  }

  async function onDelete() {
    if (!key) {
      setStatus("Key required");
      return;
    }
    try {
      await Delete(key);
      pulseWrite();
      setStatus(`Deleted ${key}`);
      await Promise.all([refreshStats(), refreshKeys()]);
    } catch (e) {
      setStatus(`Delete error: ${String(e)}`);
    }
  }

  async function onCompact() {
    try {
      setStatus("Compacting…");
      await Compact();
      setStatus("Compaction complete");
      await Promise.all([refreshStats(), refreshKeys()]);
    } catch (e) {
      setStatus(`Compact error: ${String(e)}`);
    }
  }

  async function onSnapshot() {
    try {
      setStatus("Creating snapshot…");
      const path = await Snapshot();
      setStatus(`Snapshot saved → ${path}`);
    } catch (e) {
      setStatus(`Snapshot error: ${String(e)}`);
    }
  }

  function onFormatValue() {
    const { formatted, ok } = tryFormatJSON(value);
    if (ok) {
      setValue(formatted);
      setStatus("JSON formatted");
    } else {
      setStatus("Not valid JSON");
    }
  }

  function selectRow(k: string) {
    setSelectedKey(k);
    setKey(k);
    void (async () => {
      try {
        const v = await Get(k);
        setReadValue(v);
        setValue(v);
      } catch {
        // ignore
      }
    })();
  }

  const readType = detectType(readValue);
  const valueType = detectType(value);

  return (
    <div className="flex h-screen w-screen overflow-hidden bg-canvas-50">
      {/* Sidebar */}
      <aside className="sidebar flex w-[224px] flex-shrink-0 flex-col bg-gunmetal-900 text-canvas-200">
        <div className="px-4 py-5">
          <Logo size={28} withWordmark variant="dark" />
        </div>
        <nav className="flex-1 px-2 py-2">
          {NAV.map((n) => (
            <button
              key={n.id}
              className={`nav-item ${nav === n.id ? "active" : ""}`}
              onClick={() => setNav(n.id)}
            >
              {n.icon}
              <span>{n.label}</span>
            </button>
          ))}
        </nav>
        <div className="border-t border-gunmetal-800 px-3 py-3">
          <div className="mb-2 flex items-center gap-2 rounded-md bg-gunmetal-850 px-2.5 py-2">
            <div className="flex h-7 w-7 items-center justify-center rounded-full bg-copper-500 text-[11px] font-semibold text-white">
              {props.user.email.charAt(0).toUpperCase()}
            </div>
            <div className="min-w-0 flex-1">
              <div className="truncate text-[12px] font-medium text-white">{props.user.email}</div>
              <div className="text-[10px] uppercase tracking-wider text-canvas-300">{props.user.role}</div>
            </div>
            <button
              className="text-canvas-300 hover:text-white"
              onClick={props.onSignOut}
              title="Sign out"
            >
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8">
                <path d="M16 17l5-5-5-5" />
                <path d="M21 12H9" />
                <path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4" />
              </svg>
            </button>
          </div>
          <div className="flex items-center justify-between text-[10px] text-canvas-300">
            <span className="font-mono">v0.1.0</span>
            <span className="chip-mono inline-flex items-center gap-1.5 text-canvas-200">
              <span className={`dot ${writePulse ? "" : "dot-idle"}`} />
              {writePulse ? "WRITE" : "IDLE"}
            </span>
          </div>
        </div>
      </aside>

      {/* Main */}
      <main className="flex min-w-0 flex-1 flex-col">
        {/* Topbar */}
        <header className="flex h-14 items-center justify-between border-b border-canvas-200 bg-white px-6">
          <div className="flex items-center gap-3">
            <h1 className="text-[15px] font-semibold text-ink-900">
              {NAV.find((n) => n.id === nav)?.label}
            </h1>
            <span className="chip">{status}</span>
            {apiAddr && (
              <a
                href={apiAddr + "/api/health"}
                target="_blank"
                rel="noreferrer"
                className="chip chip-copper chip-mono hover:underline"
                title="Local REST API — click for health check"
              >
                ⟁ API · {apiAddr.replace(/^https?:\/\//, "")}
              </a>
            )}
          </div>
          <div className="flex items-center gap-2">
            <button className="btn-ghost btn" onClick={() => void refreshStats()}>
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8">
                <path d="M21 12a9 9 0 1 1-3-6.7" />
                <path d="M21 4v5h-5" />
              </svg>
              Refresh
            </button>
            <button className="btn" onClick={() => void onSnapshot()}>
              ⎘ Snapshot
            </button>
            <button className="btn btn-primary" onClick={() => void onCompact()}>
              Compact
            </button>
          </div>
        </header>

        {/* Content */}
        <div className="flex-1 overflow-auto px-6 py-6">
          {nav === "dashboard" && (
            <DashboardView stats={stats} writePulse={writePulse} />
          )}
          {nav === "collections" && <CollectionsView onStatus={setStatus} />}
          {nav === "console" && (
            <ConsoleView
              key_={key}
              value={value}
              readValue={readValue}
              readType={readType}
              valueType={valueType}
              onChangeKey={setKey}
              onChangeValue={setValue}
              onGet={() => void onGet()}
              onSet={() => void onSet()}
              onDelete={() => void onDelete()}
              onFormat={onFormatValue}
            />
          )}
          {nav === "browser" && (
            <BrowserView
              rows={rows}
              prefix={keyPrefix}
              onChangePrefix={(s) => {
                setKeyPrefix(s);
                setScanAfter("");
              }}
              hasNext={!!scanNextAfter}
              onFirst={() => setScanAfter("")}
              onNext={() => scanNextAfter && setScanAfter(scanNextAfter)}
              selectedKey={selectedKey}
              onSelect={selectRow}
            />
          )}
          {nav === "snapshots" && <SnapshotsView dataDir={stats?.dataDir ?? ""} onSnapshot={() => void onSnapshot()} />}
          {nav === "settings" && <SettingsView stats={stats} apiAddr={apiAddr} />}
        </div>
      </main>
    </div>
  );
}

/* ---------------------- Views ---------------------- */

function DashboardView(props: { stats: DBStats | null; writePulse: boolean }) {
  const { stats, writePulse } = props;
  return (
    <div className="animate-slideUp space-y-6">
      <div className="grid grid-cols-2 gap-3 md:grid-cols-4">
        <Stat label="Live Keys" value={stats ? String(stats.liveKeys) : "—"} accent={writePulse} />
        <Stat label="Tombstones" value={stats ? String(stats.tombstones) : "—"} />
        <Stat label="Memtable" value={stats ? formatBytes(stats.memtableBytes) : "—"} />
        <Stat label="WAL" value={stats ? formatBytes(stats.walBytes) : "—"} />
        <Stat label="SSTables" value={stats ? String(stats.ssTableCount) : "—"} />
        <Stat label="Total Keys" value={stats ? String(stats.keys) : "—"} />
        <StatMono label="Data Dir" value={stats?.dataDir ?? "—"} />
        <StatMono label="WAL Path" value={stats?.walPath ?? "—"} />
      </div>

      <div className="card card-pad">
        <div className="mb-3 flex items-center justify-between">
          <div>
            <div className="section-title">Engine Architecture</div>
            <div className="section-sub">Log-Structured Merge Tree · built from scratch in Go</div>
          </div>
          <div className="flex gap-1.5">
            <span className="chip chip-copper">Memtable</span>
            <span className="chip chip-steel">WAL · CRC32C</span>
            <span className="chip chip-steel">SSTables · Bloom</span>
          </div>
        </div>
        <ArchDiagram />
      </div>
    </div>
  );
}

function ConsoleView(props: {
  key_: string;
  value: string;
  readValue: string;
  readType: ValueType;
  valueType: ValueType;
  onChangeKey: (s: string) => void;
  onChangeValue: (s: string) => void;
  onGet: () => void;
  onSet: () => void;
  onDelete: () => void;
  onFormat: () => void;
}) {
  return (
    <div className="animate-slideUp grid grid-cols-1 gap-5 lg:grid-cols-2">
      <div className="card card-pad space-y-3">
        <div className="section-title">Write / Read</div>
        <div>
          <label className="label">Key</label>
          <input
            value={props.key_}
            onChange={(e) => props.onChangeKey(e.target.value)}
            className="field mt-1"
            placeholder="e.g. user:123"
            spellCheck={false}
          />
        </div>
        <div>
          <div className="flex items-center justify-between">
            <label className="label">Value</label>
            <TypeChip type={props.valueType} />
          </div>
          <textarea
            value={props.value}
            onChange={(e) => props.onChangeValue(e.target.value)}
            className="field mt-1"
            placeholder='{ "name": "hello" } or any string'
            rows={8}
            spellCheck={false}
          />
          <div className="mt-2 flex flex-wrap items-center gap-2">
            <button className="btn btn-primary" onClick={props.onSet}>
              Set
            </button>
            <button className="btn" onClick={props.onGet}>
              Get
            </button>
            <button className="btn btn-danger" onClick={props.onDelete}>
              Delete
            </button>
            <div className="ml-auto">
              <button className="btn btn-ghost" onClick={props.onFormat}>
                {"{ }"} Format JSON
              </button>
            </div>
          </div>
        </div>
      </div>

      <div className="card card-pad space-y-3">
        <div className="flex items-center justify-between">
          <div className="section-title">Read Result</div>
          <TypeChip type={props.readType} />
        </div>
        <pre className="code-block">
          {props.readValue
            ? props.readType === "json"
              ? tryFormatJSON(props.readValue).formatted
              : props.readValue
            : "—"}
        </pre>
        <div className="text-[11px] text-ink-400">
          Click any row in the Data Browser to load it here.
        </div>
      </div>
    </div>
  );
}

function BrowserView(props: {
  rows: Row[];
  prefix: string;
  onChangePrefix: (s: string) => void;
  hasNext: boolean;
  onFirst: () => void;
  onNext: () => void;
  selectedKey: string;
  onSelect: (k: string) => void;
}) {
  return (
    <div className="animate-slideUp space-y-4">
      <div className="card card-pad">
        <div className="flex flex-wrap items-center gap-3">
          <div className="flex-1 min-w-[200px]">
            <input
              value={props.prefix}
              onChange={(e) => props.onChangePrefix(e.target.value)}
              className="field"
              placeholder="Filter by key prefix (e.g. user:)"
              spellCheck={false}
            />
          </div>
          <button className="btn" onClick={props.onFirst}>
            First
          </button>
          <button className="btn" disabled={!props.hasNext} onClick={props.onNext}>
            Next →
          </button>
          <span className="chip">
            {props.rows.length} {props.rows.length === 1 ? "key" : "keys"}
          </span>
        </div>
      </div>

      <div className="card overflow-hidden">
        <div className="t-header">
          <div></div>
          <div>Key</div>
          <div>Value Preview</div>
          <div className="text-right">Type</div>
        </div>
        <div>
          {props.rows.length === 0 ? (
            <div className="px-6 py-10 text-center text-sm text-ink-400">
              No keys match this filter.
            </div>
          ) : (
            props.rows.map((r) => (
              <div
                key={r.key}
                className={`t-row ${r.key === props.selectedKey ? "selected" : ""}`}
                onClick={() => props.onSelect(r.key)}
              >
                <div>
                  <span className={`dot ${r.key === props.selectedKey ? "" : "dot-idle"}`} />
                </div>
                <div className="t-key truncate">{r.key}</div>
                <div className="t-preview truncate">
                  {r.loading ? <span className="text-ink-300">…</span> : r.preview || <span className="text-ink-300">∅</span>}
                </div>
                <div className="t-type">
                  <TypeChip type={r.type} compact />
                </div>
              </div>
            ))
          )}
        </div>
      </div>
    </div>
  );
}

function SnapshotsView(props: { dataDir: string; onSnapshot: () => void }) {
  return (
    <div className="animate-slideUp space-y-4">
      <div className="card card-pad">
        <div className="flex items-center justify-between">
          <div>
            <div className="section-title">Snapshots</div>
            <div className="section-sub">
              A snapshot is a consistent copy of the WAL + every SSTable, written to a timestamped folder.
            </div>
          </div>
          <button className="btn btn-primary" onClick={props.onSnapshot}>
            ⎘ Create Snapshot
          </button>
        </div>
        <div className="mt-4 rounded-lg border border-canvas-200 bg-canvas-100 p-4">
          <div className="label mb-1">Snapshots Folder</div>
          <div className="font-mono text-[12px] text-ink-700 break-all">
            {props.dataDir ? `${props.dataDir}\\snapshots\\<timestamp>\\` : "—"}
          </div>
        </div>
      </div>
    </div>
  );
}

function SettingsView(props: { stats: DBStats | null; apiAddr: string }) {
  return (
    <div className="animate-slideUp space-y-4">
      <div className="card card-pad">
        <div className="section-title">REST API</div>
        <div className="section-sub">Local HTTP server · everything the UI can do is callable from any client.</div>
        <div className="mt-4 space-y-3">
          <KV label="Base URL" value={props.apiAddr || "(not running)"} mono />
          <details className="rounded-lg border border-canvas-200 bg-canvas-100 p-3 text-[12px]">
            <summary className="cursor-pointer font-medium text-ink-900">Endpoint reference</summary>
            <pre className="mt-3 font-mono text-[11.5px] leading-relaxed text-ink-700">
{`GET    /api/health
GET    /api/stats

GET    /api/collections
POST   /api/collections                  { name, fields:[{name,type,required}] }
GET    /api/collections/:name
PATCH  /api/collections/:name            { fields:[...] }
DELETE /api/collections/:name

GET    /api/collections/:name/records?after=&limit=
POST   /api/collections/:name/records    { ...fields }
GET    /api/collections/:name/records/:id
PATCH  /api/collections/:name/records/:id { ...partial }
DELETE /api/collections/:name/records/:id

GET    /api/kv/:key
PUT    /api/kv/:key                      { value }
DELETE /api/kv/:key`}
            </pre>
          </details>
          {props.apiAddr && (
            <div className="rounded-lg border border-canvas-200 bg-canvas-100 p-3 text-[12px]">
              <div className="label">Try it</div>
              <pre className="mt-2 font-mono text-[11.5px] text-ink-700">
{`curl ${props.apiAddr}/api/health
curl -X POST ${props.apiAddr}/api/collections \\
  -H "Content-Type: application/json" \\
  -d '{"name":"notes","fields":[{"name":"title","type":"text","required":true}]}'
curl -X POST ${props.apiAddr}/api/collections/notes/records \\
  -H "Content-Type: application/json" -d '{"title":"hello from curl"}'`}
              </pre>
            </div>
          )}
        </div>
      </div>

      <div className="card card-pad">
        <div className="section-title">Engine</div>
        <div className="section-sub">Read-only configuration · runtime stats</div>
        <div className="mt-4 grid grid-cols-1 gap-3 md:grid-cols-2">
          <KV label="Data Directory" value={props.stats?.dataDir ?? "—"} mono />
          <KV label="WAL Path" value={props.stats?.walPath ?? "—"} mono />
          <KV label="Memtable Bytes" value={props.stats ? formatBytes(props.stats.memtableBytes) : "—"} />
          <KV label="WAL Bytes" value={props.stats ? formatBytes(props.stats.walBytes) : "—"} />
          <KV label="SSTable Count" value={String(props.stats?.ssTableCount ?? "—")} />
          <KV label="Total Keys (incl. tombstones)" value={String(props.stats?.keys ?? "—")} />
        </div>
      </div>
    </div>
  );
}

/* ---------------------- Small components ---------------------- */

function Stat(props: { label: string; value: string; accent?: boolean }) {
  return (
    <div className={`stat ${props.accent ? "animate-pulseCopper" : ""}`}>
      <div className="stat-label">{props.label}</div>
      <div className="stat-value">{props.value}</div>
    </div>
  );
}

function StatMono(props: { label: string; value: string }) {
  return (
    <div className="stat">
      <div className="stat-label">{props.label}</div>
      <div className="stat-value-sm">{props.value}</div>
    </div>
  );
}

function KV(props: { label: string; value: string; mono?: boolean }) {
  return (
    <div className="rounded-lg border border-canvas-200 bg-canvas-100 px-3 py-2">
      <div className="label">{props.label}</div>
      <div className={`mt-1 text-[13px] text-ink-900 break-all ${props.mono ? "font-mono" : ""}`}>
        {props.value}
      </div>
    </div>
  );
}

function TypeChip(props: { type: ValueType; compact?: boolean }) {
  const map: Record<ValueType, { label: string; cls: string }> = {
    json: { label: "JSON", cls: "chip-copper" },
    number: { label: "NUM", cls: "chip-steel" },
    text: { label: "TEXT", cls: "chip" },
    empty: { label: "—", cls: "chip" }
  };
  const { label, cls } = map[props.type];
  if (props.compact) {
    return <span className={`chip ${cls} chip-mono`}>{label}</span>;
  }
  return <span className={`chip ${cls} chip-mono`}>{label}</span>;
}

function ArchDiagram() {
  return (
    <div className="grid grid-cols-3 gap-3">
      <div className="rounded-lg border border-copper-100 bg-copper-50 p-4">
        <div className="text-[11px] font-semibold uppercase tracking-wide text-copper-700">
          Memtable
        </div>
        <div className="mt-1 text-[12px] text-copper-700">
          In-memory map, RWMutex-protected. Writes land here first.
        </div>
      </div>
      <div className="rounded-lg border border-steel-100 bg-steel-100/50 p-4">
        <div className="text-[11px] font-semibold uppercase tracking-wide text-steel-700">
          WAL
        </div>
        <div className="mt-1 text-[12px] text-steel-700">
          Append-only binary log. CRC32C per record. Replayed on startup.
        </div>
      </div>
      <div className="rounded-lg border border-canvas-200 bg-canvas-100 p-4">
        <div className="text-[11px] font-semibold uppercase tracking-wide text-ink-500">
          SSTables
        </div>
        <div className="mt-1 text-[12px] text-ink-500">
          Sorted, immutable. Bloom filter per file. Flushed at 1 MB, compacted on demand.
        </div>
      </div>
    </div>
  );
}
