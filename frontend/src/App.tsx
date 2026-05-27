import { useEffect, useRef, useState } from "react";
import {
  Compact,
  Delete,
  Get,
  GetStats,
  Scan,
  Set as SetKV,
  Snapshot
} from "./wailsjs/go/bridge/DBService";
import { bridge } from "./wailsjs/go/models";

type DBStats = bridge.Stats;

type Row = {
  key: string;
  preview: string;
  loading: boolean;
};

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

function isJSONLike(s: string): boolean {
  const t = s.trim();
  if (t.length < 2) return false;
  const first = t[0];
  const last = t[t.length - 1];
  if (!((first === "{" && last === "}") || (first === "[" && last === "]"))) return false;
  try {
    JSON.parse(t);
    return true;
  } catch {
    return false;
  }
}

export default function App() {
  const [key, setKey] = useState<string>("");
  const [value, setValue] = useState<string>("");
  const [readValue, setReadValue] = useState<string>("");
  const [status, setStatus] = useState<string>("READY");
  const [stats, setStats] = useState<DBStats | null>(null);

  const [keyPrefix, setKeyPrefix] = useState<string>("");
  const [rows, setRows] = useState<Row[]>([]);
  const [scanAfter, setScanAfter] = useState<string>("");
  const [scanNextAfter, setScanNextAfter] = useState<string>("");
  const [selectedKey, setSelectedKey] = useState<string>("");

  const [writeLed, setWriteLed] = useState<boolean>(false);
  const ledTimer = useRef<number | null>(null);

  function flashWriteLed() {
    setWriteLed(true);
    if (ledTimer.current !== null) {
      window.clearTimeout(ledTimer.current);
    }
    ledTimer.current = window.setTimeout(() => setWriteLed(false), 350);
  }

  async function refreshStats() {
    try {
      const st = await GetStats();
      setStats(st);
    } catch (e) {
      setStatus(`STATS ERR: ${String(e)}`);
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
      const initial: Row[] = keys.map((k) => ({ key: k, preview: "", loading: true }));
      setRows(initial);
      // Fetch previews in parallel but bounded.
      const previews = await Promise.all(keys.map((k) => Get(k).catch(() => "")));
      setRows(keys.map((k, i) => ({ key: k, preview: truncate(previews[i] ?? "", PREVIEW_BYTES), loading: false })));
      setStatus("READY");
    } catch (e) {
      setStatus(`SCAN ERR: ${String(e)}`);
    }
  }

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
      setStatus("KEY REQUIRED");
      return;
    }
    try {
      const v = await Get(key);
      setReadValue(v);
      setStatus(v ? "READ OK" : "KEY NOT FOUND");
    } catch (e) {
      setStatus(`GET ERR: ${String(e)}`);
    }
  }

  async function onSet() {
    if (!key) {
      setStatus("KEY REQUIRED");
      return;
    }
    try {
      await SetKV(key, value);
      flashWriteLed();
      setStatus(`SET ${key}`);
      await Promise.all([refreshStats(), refreshKeys()]);
    } catch (e) {
      setStatus(`SET ERR: ${String(e)}`);
    }
  }

  async function onDelete() {
    if (!key) {
      setStatus("KEY REQUIRED");
      return;
    }
    try {
      await Delete(key);
      flashWriteLed();
      setStatus(`DEL ${key}`);
      await Promise.all([refreshStats(), refreshKeys()]);
    } catch (e) {
      setStatus(`DEL ERR: ${String(e)}`);
    }
  }

  async function onCompact() {
    try {
      setStatus("COMPACTING…");
      await Compact();
      setStatus("COMPACTION DONE");
      await Promise.all([refreshStats(), refreshKeys()]);
    } catch (e) {
      setStatus(`COMPACT ERR: ${String(e)}`);
    }
  }

  async function onSnapshot() {
    try {
      setStatus("SNAPSHOTTING…");
      const path = await Snapshot();
      setStatus(`SNAPSHOT → ${path}`);
    } catch (e) {
      setStatus(`SNAPSHOT ERR: ${String(e)}`);
    }
  }

  function onFormatValue() {
    const { formatted, ok } = tryFormatJSON(value);
    if (ok) {
      setValue(formatted);
      setStatus("JSON FORMATTED");
    } else {
      setStatus("NOT VALID JSON");
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

  return (
    <div className="min-h-screen grid-overlay">
      <div className="mx-auto max-w-6xl px-6 py-6">
        <Header writeLed={writeLed} status={status} />

        <section className="mt-5">
          <PanelHeader title="Engine Telemetry" right={<span className="chip">LSM · Memtable + WAL + SSTables</span>} />
          <div className="panel p-4">
            <div className="grid grid-cols-2 gap-3 md:grid-cols-4">
              <Metric label="Live Keys" value={stats ? String(stats.liveKeys) : "—"} />
              <Metric label="Tombstones" value={stats ? String(stats.tombstones) : "—"} />
              <Metric label="Memtable" value={stats ? formatBytes(stats.memtableBytes) : "—"} />
              <Metric label="WAL" value={stats ? formatBytes(stats.walBytes) : "—"} />
              <Metric label="SSTables" value={stats ? String(stats.ssTableCount) : "—"} />
              <Metric label="Total Keys" value={stats ? String(stats.keys) : "—"} />
              <MetricMono label="Data Dir" value={stats?.dataDir ?? "—"} />
              <MetricMono label="WAL Path" value={stats?.walPath ?? "—"} />
            </div>
            <div className="mt-4 flex flex-wrap gap-2">
              <button className="btn" onClick={() => void onCompact()}>
                ⚙ Compact SSTables
              </button>
              <button className="btn" onClick={() => void onSnapshot()}>
                ⎘ Snapshot
              </button>
              <button className="btn" onClick={() => void refreshStats()}>
                ↻ Refresh
              </button>
            </div>
          </div>
        </section>

        <div className="mt-5 grid grid-cols-1 gap-5 lg:grid-cols-5">
          <section className="lg:col-span-2">
            <PanelHeader title="Console" right={<span className="chip">SET · GET · DEL</span>} />
            <div className="panel space-y-3 p-4">
              <div>
                <label className="label">Key</label>
                <input
                  value={key}
                  onChange={(e) => setKey(e.target.value)}
                  className="field mt-1"
                  placeholder="e.g. user:123"
                  spellCheck={false}
                />
              </div>
              <div>
                <div className="flex items-center justify-between">
                  <label className="label">Value</label>
                  {isJSONLike(value) && (
                    <span className="chip">
                      <span className="led" /> JSON
                    </span>
                  )}
                </div>
                <textarea
                  value={value}
                  onChange={(e) => setValue(e.target.value)}
                  className="field mt-1"
                  placeholder="JSON, text, or any string"
                  rows={5}
                  spellCheck={false}
                />
                <div className="mt-1 flex justify-end">
                  <button className="btn" onClick={onFormatValue}>
                    {"{ }"} Format JSON
                  </button>
                </div>
              </div>
              <div className="flex flex-wrap gap-2">
                <button className="btn btn-primary" onClick={() => void onSet()}>
                  ▶ Set
                </button>
                <button className="btn" onClick={() => void onGet()}>
                  ◐ Get
                </button>
                <button className="btn btn-danger" onClick={() => void onDelete()}>
                  ✕ Delete
                </button>
              </div>
              <div>
                <div className="flex items-center justify-between">
                  <label className="label">Read Result</label>
                  {isJSONLike(readValue) && (
                    <span className="chip">
                      <span className="led" /> JSON
                    </span>
                  )}
                </div>
                <pre className="field mt-1 max-h-40 overflow-auto whitespace-pre-wrap">
                  {readValue ? (isJSONLike(readValue) ? tryFormatJSON(readValue).formatted : readValue) : "—"}
                </pre>
              </div>
            </div>
          </section>

          <section className="lg:col-span-3">
            <PanelHeader
              title="Data Browser"
              right={
                <span className="chip">
                  {rows.length} {rows.length === 1 ? "key" : "keys"}
                </span>
              }
            />
            <div className="panel p-4">
              <div className="flex flex-wrap items-end gap-2">
                <div className="flex-1 min-w-[200px]">
                  <label className="label">Prefix Filter</label>
                  <input
                    value={keyPrefix}
                    onChange={(e) => {
                      setKeyPrefix(e.target.value);
                      setScanAfter("");
                    }}
                    className="field mt-1"
                    placeholder="e.g. user:"
                    spellCheck={false}
                  />
                </div>
                <button
                  className="btn"
                  onClick={() => {
                    setScanAfter("");
                  }}
                >
                  ⤒ First
                </button>
                <button
                  className="btn"
                  disabled={!scanNextAfter}
                  onClick={() => {
                    if (scanNextAfter) setScanAfter(scanNextAfter);
                  }}
                >
                  Next ▶
                </button>
              </div>

              <div className="mt-3 overflow-hidden rounded-sm border border-gunmetal-700">
                <div className="grid grid-cols-12 gap-2 border-b border-gunmetal-700 bg-gunmetal-850 px-3 py-2">
                  <div className="label col-span-4">Key</div>
                  <div className="label col-span-8">Value Preview</div>
                </div>
                <div className="max-h-[420px] overflow-auto">
                  {rows.length === 0 ? (
                    <div className="px-3 py-6 text-center text-sm text-silver-400">
                      No keys match this filter.
                    </div>
                  ) : (
                    rows.map((r) => {
                      const isSelected = r.key === selectedKey;
                      return (
                        <button
                          key={r.key}
                          onClick={() => selectRow(r.key)}
                          className={`grid w-full grid-cols-12 gap-2 border-b border-gunmetal-800 px-3 py-2 text-left transition-colors hover:bg-gunmetal-850 ${
                            isSelected ? "bg-gunmetal-800" : ""
                          }`}
                        >
                          <div className="col-span-4 truncate font-mono text-xs text-copper-300">
                            {r.key}
                          </div>
                          <div className="col-span-8 truncate font-mono text-xs text-silver-200">
                            {r.loading ? <span className="text-silver-400">…</span> : r.preview || <span className="text-silver-400">∅</span>}
                          </div>
                        </button>
                      );
                    })
                  )}
                </div>
              </div>

              <div className="mt-2 text-[10px] uppercase tracking-widest text-silver-400">
                Page size {PAGE_SIZE} · Preview {PREVIEW_BYTES}b · Tombstones hidden
              </div>
            </div>
          </section>
        </div>

        <footer className="mt-6 flex items-center justify-between text-[10px] uppercase tracking-widest text-silver-400">
          <span>SolderDB · Local-First LSM Engine</span>
          <span className="font-mono">v0.1.0</span>
        </footer>
      </div>
    </div>
  );
}

function Header(props: { writeLed: boolean; status: string }) {
  return (
    <header className="panel flex items-center justify-between gap-4 px-4 py-3">
      <div className="flex items-center gap-3">
        <div className="flex h-9 w-9 items-center justify-center rounded-sm border border-copper-600 bg-gunmetal-950 text-copper-400 shadow-copper-glow">
          <span className="font-mono text-sm font-bold">⚡</span>
        </div>
        <div>
          <div className="font-mono text-base font-semibold tracking-wide text-silver-50">
            SOLDER<span className="text-copper-400">DB</span>
          </div>
          <div className="text-[10px] uppercase tracking-widest text-silver-400">
            Flux &amp; Iron · LSM Storage Engine
          </div>
        </div>
      </div>
      <div className="flex items-center gap-3">
        <div className="flex items-center gap-2">
          <span className={`led ${props.writeLed ? "" : "led-idle"}`} />
          <span className="font-mono text-[10px] uppercase tracking-widest text-silver-300">
            {props.writeLed ? "WRITE" : "IDLE"}
          </span>
        </div>
        <div className="chip">{props.status}</div>
      </div>
    </header>
  );
}

function PanelHeader(props: { title: string; right?: React.ReactNode }) {
  return (
    <div className="mb-2 flex items-center justify-between">
      <h2 className="font-mono text-[11px] font-semibold uppercase tracking-[0.2em] text-silver-200">
        ▍ {props.title}
      </h2>
      {props.right ?? null}
    </div>
  );
}

function Metric(props: { label: string; value: string }) {
  return (
    <div className="rounded-sm border border-gunmetal-700 bg-gunmetal-950 px-3 py-2">
      <div className="label">{props.label}</div>
      <div className="metric-value mt-1">{props.value}</div>
    </div>
  );
}

function MetricMono(props: { label: string; value: string }) {
  return (
    <div className="rounded-sm border border-gunmetal-700 bg-gunmetal-950 px-3 py-2">
      <div className="label">{props.label}</div>
      <div className="metric-value-mono mt-1">{props.value}</div>
    </div>
  );
}
