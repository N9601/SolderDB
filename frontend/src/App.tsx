import { useEffect, useState } from "react";
import {
  Compact,
  Delete,
  Get,
  GetStats,
  Scan,
  Set as SetKV
} from "./wailsjs/go/bridge/DBService";
import { bridge } from "./wailsjs/go/models";

type DBStats = bridge.Stats;

type KVRow = {
  key: string;
  value: string;
};

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

export default function App() {
  const [key, setKey] = useState<string>("");
  const [value, setValue] = useState<string>("");
  const [readValue, setReadValue] = useState<string>("");
  const [status, setStatus] = useState<string>("");
  const [stats, setStats] = useState<DBStats | null>(null);
  const [recent, setRecent] = useState<KVRow[]>([]);
  const [keyPrefix, setKeyPrefix] = useState<string>("");
  const [keys, setKeys] = useState<string[]>([]);
  const [selectedKey, setSelectedKey] = useState<string>("");
  const [scanAfter, setScanAfter] = useState<string>("");
  const [scanNextAfter, setScanNextAfter] = useState<string>("");

  async function refreshStats() {
    const st = await GetStats();
    setStats(st);
  }

  async function refreshKeys() {
    const res = await Scan({ prefix: keyPrefix, after: scanAfter, limit: 200 } as bridge.ScanOptions);
    setKeys(res.keys);
    setScanNextAfter(res.nextAfter);
  }

  useEffect(() => {
    void refreshStats();
    void refreshKeys();
    const id = window.setInterval(() => {
      void refreshStats();
    }, 1000);
    return () => window.clearInterval(id);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    void refreshKeys();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [keyPrefix, scanAfter]);

  async function onGet() {
    setStatus("");
    const v = await Get(key);
    setReadValue(v);
    setStatus(v ? "OK" : "Key not found");
  }

  async function onSet() {
    setStatus("");
    await SetKV(key, value);
    setRecent((prev) => [{ key, value }, ...prev].slice(0, 20));
    await refreshStats();
    await refreshKeys();
    setStatus("Saved");
  }

  async function onDelete() {
    setStatus("");
    await Delete(key);
    await refreshStats();
    await refreshKeys();
    setStatus("Deleted (tombstone)");
  }

  async function onCompact() {
    setStatus("Compacting…");
    await Compact();
    await refreshStats();
    await refreshKeys();
    setStatus("Compaction complete");
  }

  return (
    <div className="min-h-screen bg-zinc-950 text-zinc-100">
      <div className="mx-auto max-w-5xl px-6 py-8">
        <header className="mb-6">
          <h1 className="text-2xl font-semibold tracking-tight">SolderDB</h1>
          <p className="mt-1 text-sm text-zinc-400">
            Local-first LSM engine (Memtable + WAL). Desktop admin GUI via Wails.
          </p>
        </header>

        <section className="mb-6 rounded-xl border border-zinc-800 bg-zinc-900/30 p-4">
          <h2 className="text-sm font-medium text-zinc-200">Metrics</h2>
          <div className="mt-3 grid grid-cols-2 gap-3 md:grid-cols-4">
            <Metric label="Keys" value={stats ? String(stats.keys) : "—"} />
            <Metric label="Live keys" value={stats ? String(stats.liveKeys) : "—"} />
            <Metric label="Tombstones" value={stats ? String(stats.tombstones) : "—"} />
            <Metric label="Memtable" value={stats ? formatBytes(stats.memtableBytes) : "—"} />
            <Metric label="WAL size" value={stats ? formatBytes(stats.walBytes) : "—"} />
            <Metric label="SSTables" value={stats ? String(stats.ssTableCount) : "—"} />
            <Metric label="Data dir" value={stats ? stats.dataDir : "—"} />
            <Metric label="WAL path" value={stats ? stats.walPath : "—"} />
            <Metric label="Status" value={status || "—"} />
          </div>
          <div className="mt-3">
            <button
              onClick={() => void onCompact()}
              className="rounded-lg border border-zinc-800 bg-zinc-950 px-3 py-2 text-sm"
            >
              Compact SSTables
            </button>
          </div>
        </section>

        <section className="mb-6 rounded-xl border border-zinc-800 bg-zinc-900/30 p-4">
          <h2 className="text-sm font-medium text-zinc-200">Operations</h2>

          <div className="mt-3 space-y-3">
            <div>
              <label className="text-xs text-zinc-400">Key</label>
              <input
                value={key}
                onChange={(e) => setKey(e.target.value)}
                className="mt-1 w-full rounded-lg border border-zinc-800 bg-zinc-950 px-3 py-2 text-sm outline-none"
                placeholder="e.g. user:123"
              />
            </div>

            <div>
              <label className="text-xs text-zinc-400">Value (for Set)</label>
              <textarea
                value={value}
                onChange={(e) => setValue(e.target.value)}
                className="mt-1 w-full rounded-lg border border-zinc-800 bg-zinc-950 px-3 py-2 text-sm outline-none"
                placeholder="JSON, text, or any string"
                rows={4}
              />
            </div>

            <div className="flex flex-wrap gap-2">
              <button
                onClick={() => void onGet()}
                className="rounded-lg border border-zinc-800 bg-zinc-950 px-3 py-2 text-sm"
              >
                Get
              </button>
              <button
                onClick={() => void onSet()}
                className="rounded-lg border border-zinc-800 bg-zinc-950 px-3 py-2 text-sm"
              >
                Set
              </button>
              <button
                onClick={() => void onDelete()}
                className="rounded-lg border border-zinc-800 bg-zinc-950 px-3 py-2 text-sm"
              >
                Delete
              </button>
            </div>

            <div>
              <label className="text-xs text-zinc-400">Read result</label>
              <div className="mt-1 rounded-lg border border-zinc-800 bg-zinc-950 px-3 py-2 text-sm text-zinc-200">
                {readValue || "—"}
              </div>
            </div>
          </div>
        </section>

        <section className="mb-6 rounded-xl border border-zinc-800 bg-zinc-900/30 p-4">
          <h2 className="text-sm font-medium text-zinc-200">Data browser</h2>

          <div className="mt-3 space-y-3">
            <div>
              <label className="text-xs text-zinc-400">Key prefix filter</label>
              <input
                value={keyPrefix}
                onChange={(e) => setKeyPrefix(e.target.value)}
                className="mt-1 w-full rounded-lg border border-zinc-800 bg-zinc-950 px-3 py-2 text-sm outline-none"
                placeholder="e.g. user:"
              />
              <div className="mt-2 text-xs text-zinc-500">
                Showing up to 200 keys per page. Tombstoned keys are hidden.
              </div>
            </div>

            <div className="rounded-lg border border-zinc-800 bg-zinc-950">
              {keys.length === 0 ? (
                <div className="px-3 py-2 text-sm text-zinc-500">No keys found.</div>
              ) : (
                <div className="max-h-56 overflow-auto">
                  {keys.map((k) => (
                    <button
                      key={k}
                      onClick={() => {
                        setSelectedKey(k);
                        setKey(k);
                        void (async () => {
                          const v = await Get(k);
                          setReadValue(v);
                        })();
                      }}
                      className="block w-full border-b border-zinc-900 px-3 py-2 text-left text-sm hover:bg-zinc-900/40"
                    >
                      <div className="truncate">{k}</div>
                    </button>
                  ))}
                </div>
              )}
            </div>

            <div className="flex flex-wrap gap-2">
              <button
                onClick={() => {
                  setScanAfter("");
                  setSelectedKey("");
                }}
                className="rounded-lg border border-zinc-800 bg-zinc-950 px-3 py-2 text-sm"
              >
                First page
              </button>
              <button
                onClick={() => {
                  if (!scanNextAfter) return;
                  setScanAfter(scanNextAfter);
                }}
                disabled={!scanNextAfter}
                className="rounded-lg border border-zinc-800 bg-zinc-950 px-3 py-2 text-sm disabled:opacity-50"
              >
                Next page
              </button>
            </div>

            <div className="text-xs text-zinc-500">
              Selected: <span className="text-zinc-300">{selectedKey || "—"}</span>
            </div>
          </div>
        </section>

        <section className="rounded-xl border border-zinc-800 bg-zinc-900/30 p-4">
          <h2 className="text-sm font-medium text-zinc-200">Recent writes</h2>
          <div className="mt-3 space-y-2">
            {recent.length === 0 ? (
              <div className="text-sm text-zinc-500">No recent writes yet.</div>
            ) : (
              recent.map((r, idx) => (
                <div
                  key={`${r.key}:${idx}`}
                  className="rounded-lg border border-zinc-800 bg-zinc-950 px-3 py-2"
                >
                  <div className="text-xs text-zinc-400">{r.key}</div>
                  <div className="mt-1 whitespace-pre-wrap text-sm text-zinc-200">{r.value}</div>
                </div>
              ))
            )}
          </div>
        </section>
      </div>
    </div>
  );
}

function Metric(props: { label: string; value: string }) {
  return (
    <div className="rounded-lg border border-zinc-800 bg-zinc-950 px-3 py-2">
      <div className="text-xs text-zinc-500">{props.label}</div>
      <div className="mt-1 truncate text-sm text-zinc-100">{props.value}</div>
    </div>
  );
}
