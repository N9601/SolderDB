import { useEffect, useRef, useState } from "react";
import {
  Compact,
  Delete,
  Get,
  GetAPIAddr,
  GetStats,
  ListSnapshots,
  Scan,
  Set as SetKV,
  Snapshot
} from "./wailsjs/go/bridge/DBService";
import { bridge } from "./wailsjs/go/models";
import { Logo } from "./components/Logo";
import CollectionsView from "./views/CollectionsView";
import FilesView from "./views/FilesView";
import ApiExplorerView from "./views/ApiExplorerView";
import LogsView from "./views/LogsView";
import LifecycleView from "./views/LifecycleView";
import AuthView from "./views/AuthView";
import { VerifyToken } from "./wailsjs/go/bridge/AuthService";
import { GetStatus as GetHardwareStatus, GetThresholds, SetThresholds } from "./wailsjs/go/bridge/HardwareService";
import type { bridge as bridgeNS } from "./wailsjs/go/models";
import { getToken, setToken } from "./lib/apiFetch";
import { ToastProvider, useStatusToast, useToast } from "./components/Toast";
import { CommandPalette, type CommandItem } from "./components/CommandPalette";
import { CountUp } from "./components/CountUp";
import { StatSkeleton } from "./components/Skeleton";

type DBStats = bridge.Stats;

type Row = {
  key: string;
  preview: string;
  type: ValueType;
  loading: boolean;
};

type ValueType = "json" | "number" | "text" | "empty";

type NavId = "dashboard" | "lifecycle" | "collections" | "files" | "api" | "logs" | "console" | "browser" | "snapshots" | "settings";

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
    id: "lifecycle",
    label: "Lifecycle",
    icon: (
      <svg className="nav-item-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7">
        <path d="M21 12a9 9 0 1 1-3-6.7" />
        <path d="M21 4v5h-5" />
        <circle cx="12" cy="12" r="2" fill="currentColor" />
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
    id: "files",
    label: "Files",
    icon: (
      <svg className="nav-item-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7">
        <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z" />
        <polyline points="14 2 14 8 20 8" />
      </svg>
    )
  },
  {
    id: "logs",
    label: "Logs",
    icon: (
      <svg className="nav-item-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7">
        <path d="M3 4h18v4H3z" />
        <path d="M3 10h18v4H3z" />
        <path d="M3 16h18v4H3z" />
      </svg>
    )
  },
  {
    id: "api",
    label: "API Explorer",
    icon: (
      <svg className="nav-item-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7">
        <polyline points="16 18 22 12 16 6" />
        <polyline points="8 6 2 12 8 18" />
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

type AuthState =
  | { state: "loading" }
  | { state: "signedOut" }
  | { state: "signedIn"; user: bridgeNS.User; token: string };

export default function App() {
  const [auth, setAuth] = useState<AuthState>({ state: "loading" });

  useEffect(() => {
    const token = getToken();
    if (!token) {
      setAuth({ state: "signedOut" });
      return;
    }
    void (async () => {
      try {
        const user = await VerifyToken(token);
        setAuth({ state: "signedIn", user, token });
      } catch {
        setToken("");
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
          setToken(sess.token);
          setAuth({ state: "signedIn", user: sess.user, token: sess.token });
        }}
      />
    );
  }

  return (
    <AppShell
      user={auth.user}
      onSignOut={() => {
        setToken("");
        setAuth({ state: "signedOut" });
      }}
    />
  );
}

const SIDEBAR_COLLAPSED_KEY = "solderdb.sidebar.collapsed";

function AppShell(props: { user: bridgeNS.User; onSignOut: () => void }) {
  const [nav, setNav] = useState<NavId>("dashboard");
  const [paletteOpen, setPaletteOpen] = useState(false);
  const [sidebarCollapsed, setSidebarCollapsed] = useState<boolean>(() => {
    return window.localStorage.getItem(SIDEBAR_COLLAPSED_KEY) === "1";
  });
  const toast = useToast();

  function setSidebar(c: boolean) {
    setSidebarCollapsed(c);
    window.localStorage.setItem(SIDEBAR_COLLAPSED_KEY, c ? "1" : "0");
  }

  const [key, setKey] = useState<string>("");
  const [value, setValue] = useState<string>("");
  const [readValue, setReadValue] = useState<string>("");
  const [status, setStatus] = useState<string>("Ready");
  const [stats, setStats] = useState<DBStats | null>(null);

  const [keyPrefix, setKeyPrefix] = useState<string>("");
  const [scanStart, setScanStart] = useState<string>("");
  const [scanEnd, setScanEnd] = useState<string>("");
  const [rows, setRows] = useState<Row[]>([]);
  const [scanAfter, setScanAfter] = useState<string>("");
  const [scanNextAfter, setScanNextAfter] = useState<string>("");
  const [selectedKey, setSelectedKey] = useState<string>("");

  const [writePulse, setWritePulse] = useState<boolean>(false);
  const [apiAddr, setApiAddr] = useState<string>("");
  const [hw, setHw] = useState<bridgeNS.HardwareStatus | null>(null);

  useStatusToast(status);

  // G+digit nav shortcut — type "g" then a number key within 1.5s.
  useEffect(() => {
    let armed = false;
    let timer: number | null = null;
    function disarm() {
      armed = false;
      if (timer !== null) {
        window.clearTimeout(timer);
        timer = null;
      }
    }
    function onKey(e: KeyboardEvent) {
      const target = e.target as HTMLElement | null;
      if (target && /^(INPUT|TEXTAREA|SELECT)$/.test(target.tagName)) return;
      if (target && target.isContentEditable) return;
      if (!armed && e.key.toLowerCase() === "g") {
        armed = true;
        timer = window.setTimeout(disarm, 1500);
        return;
      }
      if (armed) {
        const n = parseInt(e.key, 10);
        if (!Number.isNaN(n) && n >= 1 && n <= NAV.length) {
          const target = NAV[n - 1];
          if (target) setNav(target.id);
        }
        disarm();
      }
    }
    window.addEventListener("keydown", onKey);
    return () => {
      window.removeEventListener("keydown", onKey);
      disarm();
    };
  }, []);

  const commands: CommandItem[] = [
    ...NAV.map((n, i) => ({
      id: "nav-" + n.id,
      label: "Go to " + n.label,
      group: "Navigation",
      shortcut: i < 9 ? `G ${i + 1}` : undefined,
      run: () => setNav(n.id)
    })),
    {
      id: "action-compact",
      label: "Compact SSTables",
      hint: "merge SSTables — admin",
      group: "Actions",
      run: () => void onCompact()
    },
    {
      id: "action-snapshot",
      label: "Create snapshot",
      hint: "WAL + every SSTable copied to disk",
      group: "Actions",
      run: () => void onSnapshot()
    },
    {
      id: "action-refresh",
      label: "Refresh stats",
      group: "Actions",
      run: () => void refreshStats()
    },
    {
      id: "action-signout",
      label: "Sign out",
      hint: props.user.email,
      group: "Account",
      run: () => {
        toast.push("Signed out", "info");
        props.onSignOut();
      }
    }
  ];
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
    try {
      setHw(await GetHardwareStatus());
    } catch {
      // hardware service may not be present in all builds
    }
  }

  async function refreshKeys() {
    try {
      const res = await Scan({
        prefix: keyPrefix,
        after: scanAfter,
        start: scanStart,
        end: scanEnd,
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
  }, [keyPrefix, scanAfter, scanStart, scanEnd]);

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
      <aside
        className={`sidebar flex flex-shrink-0 flex-col bg-gunmetal-900 text-canvas-200 transition-[width] duration-200 ease-out ${
          sidebarCollapsed ? "w-[64px]" : "w-[228px]"
        }`}
      >
        <div className={`flex items-center ${sidebarCollapsed ? "justify-center px-2" : "px-4"} py-5`}>
          {sidebarCollapsed ? (
            <Logo size={26} variant="dark" />
          ) : (
            <Logo size={28} withWordmark variant="dark" />
          )}
        </div>
        <nav className="flex-1 px-2 py-2">
          {NAV.map((n, i) => (
            <button
              key={n.id}
              className={`nav-item group ${nav === n.id ? "active" : ""} ${sidebarCollapsed ? "justify-center" : ""}`}
              onClick={() => setNav(n.id)}
              title={sidebarCollapsed ? `${n.label} · G then ${i + 1}` : undefined}
            >
              {n.icon}
              {!sidebarCollapsed && (
                <>
                  <span className="flex-1">{n.label}</span>
                  {i < 9 && (
                    <span className="font-mono text-[9.5px] text-canvas-300 opacity-0 transition-opacity group-hover:opacity-100">
                      G {i + 1}
                    </span>
                  )}
                </>
              )}
            </button>
          ))}
        </nav>

        <div className="border-t border-gunmetal-800">
          {!sidebarCollapsed ? (
            <div className="px-3 py-3">
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
                <span className="font-mono">v0.3.0</span>
                <span className="chip-mono inline-flex items-center gap-1.5 text-canvas-200">
                  <span className={`dot ${writePulse ? "" : "dot-idle"}`} />
                  {writePulse ? "WRITE" : "IDLE"}
                </span>
              </div>
              <button
                onClick={() => setSidebar(true)}
                className="mt-3 flex w-full items-center justify-center gap-1 rounded-md border border-gunmetal-700 bg-transparent px-2 py-1.5 text-[10.5px] uppercase tracking-wider text-canvas-300 transition-colors hover:bg-gunmetal-800 hover:text-white"
                title="Collapse sidebar"
              >
                <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                  <polyline points="15 18 9 12 15 6" />
                </svg>
                Collapse
              </button>
            </div>
          ) : (
            <div className="flex flex-col items-center gap-3 px-2 py-3">
              <div
                className="flex h-7 w-7 items-center justify-center rounded-full bg-copper-500 text-[11px] font-semibold text-white"
                title={`${props.user.email} · ${props.user.role}`}
              >
                {props.user.email.charAt(0).toUpperCase()}
              </div>
              <button
                onClick={() => setSidebar(false)}
                className="text-canvas-300 transition-colors hover:text-white"
                title="Expand sidebar"
              >
                <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                  <polyline points="9 18 15 12 9 6" />
                </svg>
              </button>
            </div>
          )}
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
            {hw?.throttled && (
              <span
                className="chip chip-mono border-amber-200 bg-amber-50 text-amber-800"
                title={`Compaction paused — ${hw.reason}`}
              >
                ⚠ Compaction paused · {hw.reason}
              </span>
            )}
          </div>
          <div className="flex items-center gap-2">
            <button
              className="hidden items-center gap-2 rounded-md border border-canvas-200 bg-canvas-50 px-3 py-1.5 text-[12px] text-ink-400 transition-colors hover:bg-canvas-100 sm:inline-flex"
              onClick={() => setPaletteOpen(true)}
              title="Command palette"
            >
              <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8">
                <circle cx="11" cy="11" r="7" />
                <line x1="21" y1="21" x2="16.65" y2="16.65" />
              </svg>
              <span>Search</span>
              <kbd className="rounded border border-canvas-300 bg-white px-1 py-0.5 font-mono text-[9.5px] text-ink-500">
                {navigator.platform.toUpperCase().includes("MAC") ? "⌘" : "Ctrl"} K
              </kbd>
            </button>
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
        <div key={nav} className="page-enter flex-1 overflow-auto px-6 py-6">
          {nav === "dashboard" && (
            <DashboardView stats={stats} writePulse={writePulse} hw={hw} user={props.user} />
          )}
          {nav === "lifecycle" && <LifecycleView />}
          {nav === "collections" && <CollectionsView onStatus={setStatus} />}
          {nav === "files" && <FilesView onStatus={setStatus} />}
          {nav === "api" && <ApiExplorerView />}
          {nav === "logs" && <LogsView />}
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
              start={scanStart}
              end={scanEnd}
              onChangePrefix={(s) => {
                setKeyPrefix(s);
                setScanAfter("");
              }}
              onChangeStart={(s) => {
                setScanStart(s);
                setScanAfter("");
              }}
              onChangeEnd={(s) => {
                setScanEnd(s);
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

      <CommandPalette open={paletteOpen} onOpenChange={setPaletteOpen} items={commands} />
    </div>
  );
}

/* ---------------------- Views ---------------------- */

function DashboardView(props: { stats: DBStats | null; writePulse: boolean; hw: bridgeNS.HardwareStatus | null; user: bridgeNS.User }) {
  const { stats, writePulse, hw, user } = props;
  return (
    <div className="animate-slideUp space-y-6">
      <DashboardHero user={user} stats={stats} hw={hw} />
      <div className="grid grid-cols-2 gap-3 md:grid-cols-4">
        {!stats ? (
          Array.from({ length: 8 }).map((_, i) => <StatSkeleton key={i} />)
        ) : (
          <>
            <NumStat label="Live Keys" value={stats.liveKeys} accent={writePulse} />
            <NumStat label="Tombstones" value={stats.tombstones} />
            <ByteStat label="Memtable" value={stats.memtableBytes} />
            <ByteStat label="WAL" value={stats.walBytes} />
            <NumStat label="SSTables" value={stats.ssTableCount} />
            <NumStat label="Total Keys" value={stats.keys} />
            <StatMono label="Data Dir" value={stats.dataDir} />
            <StatMono label="WAL Path" value={stats.walPath} />
          </>
        )}
      </div>

      {hw && <HardwareCard hw={hw} />}

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

function HardwareCard(props: { hw: bridgeNS.HardwareStatus }) {
  const { hw } = props;
  const [thresh, setThresh] = useState<bridgeNS.HardwareThresholds | null>(null);

  useEffect(() => {
    void (async () => {
      try {
        setThresh(await GetThresholds());
      } catch {
        // ignore
      }
    })();
  }, []);

  async function save(update: Partial<bridgeNS.HardwareThresholds>) {
    if (!thresh) return;
    const merged = { ...thresh, ...update } as bridgeNS.HardwareThresholds;
    try {
      const next = await SetThresholds(merged);
      setThresh(next);
    } catch {
      // ignore
    }
  }

  return (
    <div className="card card-pad">
      <div className="mb-3 flex items-center justify-between">
        <div>
          <div className="section-title">Hardware-Aware Compaction</div>
          <div className="section-sub">
            SolderDB skips heavy disk work when your machine is under stress.
          </div>
        </div>
        <span
          className={`chip chip-mono ${hw.throttled ? "border-amber-200 bg-amber-50 text-amber-800" : "chip-steel"}`}
        >
          <span className={`dot ${hw.throttled ? "" : "dot-idle"}`} />
          {hw.throttled ? `Throttled · ${hw.reason}` : "Healthy"}
        </span>
      </div>

      <div className="grid grid-cols-2 gap-3 md:grid-cols-4">
        <HwStat
          label="Power"
          value={hw.onBattery ? "Battery" : "AC"}
          tone={hw.onBattery ? "warn" : "ok"}
        />
        <HwStat
          label="Battery"
          value={hw.batteryKnown ? `${hw.batteryPct}%` : "—"}
          tone={hw.batteryKnown && hw.batteryPct < 25 ? "warn" : "ok"}
        />
        <HwStat
          label="CPU Temp"
          value={hw.cpuTempKnown ? `${hw.cpuTempC.toFixed(1)}°C` : "—"}
          tone={hw.cpuTempKnown && hw.cpuTempC > 80 ? "warn" : "ok"}
        />
        <HwStat label="Platform" value={hw.platform} tone="ok" />
      </div>

      {thresh && (
        <div className="mt-4 grid grid-cols-1 gap-3 rounded-lg border border-canvas-200 bg-canvas-100 p-4 md:grid-cols-3">
          <div>
            <label className="label">Min battery % (when unplugged)</label>
            <input
              type="number"
              min={0}
              max={100}
              value={thresh.minBatteryPct}
              onChange={(e) => void save({ minBatteryPct: Number(e.target.value) || 0 })}
              className="field mt-1"
            />
          </div>
          <div>
            <label className="label">Max CPU temp (°C)</label>
            <input
              type="number"
              min={0}
              max={120}
              value={thresh.maxCpuTempC}
              onChange={(e) => void save({ maxCpuTempC: Number(e.target.value) || 0 })}
              className="field mt-1"
            />
          </div>
          <div>
            <label className="label">Pause whenever on battery</label>
            <div className="mt-2">
              <label className="flex items-center gap-2 text-[12.5px] text-ink-700">
                <input
                  type="checkbox"
                  checked={thresh.pauseOnBattery}
                  onChange={(e) => void save({ pauseOnBattery: e.target.checked })}
                />
                Yes — never compact on battery
              </label>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

function HwStat(props: { label: string; value: string; tone: "ok" | "warn" }) {
  const color = props.tone === "warn" ? "text-warn" : "text-ink-900";
  return (
    <div className="rounded-lg border border-canvas-200 bg-white px-3 py-2">
      <div className="stat-label">{props.label}</div>
      <div className={`mt-1 font-mono text-[15px] font-semibold ${color}`}>{props.value}</div>
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
  start: string;
  end: string;
  onChangePrefix: (s: string) => void;
  onChangeStart: (s: string) => void;
  onChangeEnd: (s: string) => void;
  hasNext: boolean;
  onFirst: () => void;
  onNext: () => void;
  selectedKey: string;
  onSelect: (k: string) => void;
}) {
  return (
    <div className="animate-slideUp space-y-4">
      <div className="card card-pad">
        <div className="grid grid-cols-1 gap-3 md:grid-cols-3">
          <div>
            <label className="label">Prefix</label>
            <input
              value={props.prefix}
              onChange={(e) => props.onChangePrefix(e.target.value)}
              className="field mt-1"
              placeholder="user:"
              spellCheck={false}
            />
          </div>
          <div>
            <label className="label">Start (inclusive)</label>
            <input
              value={props.start}
              onChange={(e) => props.onChangeStart(e.target.value)}
              className="field mt-1"
              placeholder="user:001"
              spellCheck={false}
            />
          </div>
          <div>
            <label className="label">End (exclusive)</label>
            <input
              value={props.end}
              onChange={(e) => props.onChangeEnd(e.target.value)}
              className="field mt-1"
              placeholder="user:999"
              spellCheck={false}
            />
          </div>
        </div>
        <div className="mt-3 flex flex-wrap items-center gap-2">
          <button className="btn" onClick={props.onFirst}>
            First
          </button>
          <button className="btn" disabled={!props.hasNext} onClick={props.onNext}>
            Next →
          </button>
          <span className="chip ml-auto">
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
  const [list, setList] = useState<bridgeNS.SnapshotInfo[]>([]);

  const refresh = async () => {
    try {
      const out = await ListSnapshots();
      setList(out ?? []);
    } catch {
      // ignore
    }
  };

  useEffect(() => {
    void refresh();
  }, []);

  async function onCreate() {
    props.onSnapshot();
    setTimeout(() => void refresh(), 200);
  }

  return (
    <div className="animate-slideUp space-y-4">
      <div className="card card-pad">
        <div className="flex items-center justify-between">
          <div>
            <div className="section-title">Snapshots</div>
            <div className="section-sub">
              A snapshot is a consistent copy of the WAL + every SSTable. Stored under{" "}
              <span className="font-mono text-ink-700">{props.dataDir}\snapshots\</span>.
            </div>
          </div>
          <div className="flex gap-2">
            <button className="btn" onClick={() => void refresh()}>
              Refresh
            </button>
            <button className="btn btn-primary" onClick={() => void onCreate()}>
              ⎘ Create snapshot
            </button>
          </div>
        </div>
      </div>

      <div className="card overflow-hidden">
        <div className="border-b border-canvas-200 px-4 py-3 section-title">
          {list.length} previous snapshot{list.length === 1 ? "" : "s"}
        </div>
        {list.length === 0 ? (
          <div className="px-4 py-10 text-center text-[12px] text-ink-400">
            No snapshots yet — create one above.
          </div>
        ) : (
          <table className="w-full text-left text-[12.5px]">
            <thead>
              <tr className="border-b border-canvas-200 bg-canvas-100 text-[10.5px] uppercase tracking-wider text-ink-400">
                <th className="px-4 py-2.5 font-semibold">Name</th>
                <th className="px-4 py-2.5 font-semibold">Created</th>
                <th className="px-4 py-2.5 font-semibold">Size</th>
                <th className="px-4 py-2.5 font-semibold">Path</th>
              </tr>
            </thead>
            <tbody>
              {list.map((s) => (
                <tr key={s.path} className="border-b border-canvas-150 hover:bg-canvas-100">
                  <td className="px-4 py-2.5 font-mono text-ink-900">{s.name}</td>
                  <td className="px-4 py-2.5 text-[11.5px] text-ink-500">
                    {new Date(s.createdAt).toLocaleString()}
                  </td>
                  <td className="px-4 py-2.5 font-mono text-ink-700">{formatBytes(s.bytes)}</td>
                  <td className="px-4 py-2.5 font-mono text-[11px] text-ink-400" title={s.path}>
                    {s.path}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  );
}

function SettingsView(props: { stats: DBStats | null; apiAddr: string }) {
  return (
    <div className="animate-slideUp space-y-4">
      <div className="card card-pad">
        <div className="section-title">JavaScript SDK</div>
        <div className="section-sub">Drop-in client for any web or Node app pointing at this database.</div>
        <div className="mt-4 space-y-3">
          <div className="rounded-lg border border-canvas-200 bg-canvas-100 p-3 text-[12px]">
            <pre className="font-mono text-[11.5px] leading-relaxed text-ink-700">
{`npm install solderdb

import { SolderDB } from "solderdb";

const db = new SolderDB("${props.apiAddr || "http://localhost:8787"}");
await db.auth.login("you@example.com", "secret");

type Note = { title: string };
const notes = db.collection<Note>("notes");
await notes.create({ title: "hi" });

notes.subscribe(evt => console.log(evt.kind, evt.id));`}
            </pre>
          </div>
          <div className="text-[11px] text-ink-400">
            Source: <span className="font-mono">sdk/solderdb-js/</span> in the repo.
          </div>
        </div>
      </div>

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

function DashboardHero(props: { user: bridgeNS.User; stats: DBStats | null; hw: bridgeNS.HardwareStatus | null }) {
  const hour = new Date().getHours();
  const greeting = hour < 5 ? "Still up" : hour < 12 ? "Good morning" : hour < 18 ? "Good afternoon" : "Good evening";
  const name = props.user.email.split("@")[0];

  return (
    <div className="relative overflow-hidden rounded-2xl border border-canvas-200 bg-white p-6 shadow-card">
      {/* Decorative top edge */}
      <div
        className="pointer-events-none absolute -right-24 -top-24 h-56 w-56 rounded-full opacity-50 blur-3xl"
        style={{ background: "radial-gradient(circle, rgba(224,122,37,0.25) 0%, transparent 70%)" }}
      />
      <div className="relative flex items-start justify-between gap-6">
        <div>
          <div className="text-[12px] font-medium uppercase tracking-[0.14em] text-ink-400">
            {greeting}
          </div>
          <h1 className="mt-1 text-[26px] font-semibold tracking-tight text-ink-900">
            <span className="capitalize">{name}</span>
            <span className="ml-2 text-copper-500">·</span>
            <span className="ml-2 text-ink-500">{props.user.role}</span>
          </h1>
          <div className="mt-1.5 text-[13px] text-ink-500">
            {props.stats ? (
              <>
                <strong className="text-ink-900">{props.stats.liveKeys.toLocaleString()}</strong> live
                {" "}keys across{" "}
                <strong className="text-ink-900">{props.stats.ssTableCount}</strong> SSTable{props.stats.ssTableCount === 1 ? "" : "s"}.
              </>
            ) : (
              "Booting…"
            )}
          </div>
        </div>
        <div className="flex flex-col items-end gap-2">
          {props.hw && (
            <span
              className={`chip chip-mono ${
                props.hw.throttled
                  ? "border-amber-200 bg-amber-50 text-amber-800"
                  : "chip-steel"
              }`}
              title={props.hw.throttled ? props.hw.reason : "All systems nominal"}
            >
              <span className={`dot ${props.hw.throttled ? "" : "dot-idle"}`} />
              {props.hw.throttled ? "Throttled" : "Healthy"}
            </span>
          )}
          <span className="chip chip-mono">
            ⌘ K to search
          </span>
        </div>
      </div>
    </div>
  );
}

function NumStat(props: { label: string; value: number; accent?: boolean }) {
  return (
    <div className={`stat ${props.accent ? "animate-pulseCopper" : ""}`}>
      <div className="stat-label">{props.label}</div>
      <div className="stat-value">
        <CountUp value={props.value} />
      </div>
    </div>
  );
}

function ByteStat(props: { label: string; value: number }) {
  return (
    <div className="stat">
      <div className="stat-label">{props.label}</div>
      <div className="stat-value">
        <CountUp value={props.value} format={formatBytes} />
      </div>
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
