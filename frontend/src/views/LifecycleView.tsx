import { useCallback, useEffect, useRef, useState } from "react";
import { GetAPIAddr, GetStats } from "../wailsjs/go/bridge/DBService";
import { bridge } from "../wailsjs/go/models";
import { withAuthQuery } from "../lib/apiFetch";

type DBStats = bridge.Stats;

type Spark = { id: number; kind: "write" | "delete" | "flush" | "compact" };

const POLL_MS = 250;
const MAX_SPARKS = 8;

export default function LifecycleView() {
  const [stats, setStats] = useState<DBStats | null>(null);
  const [sparks, setSparks] = useState<Spark[]>([]);
  const [lastSSTCount, setLastSSTCount] = useState<number | null>(null);
  const [flashFlush, setFlashFlush] = useState(false);
  const [flashCompact, setFlashCompact] = useState(false);
  const nextSparkId = useRef(1);
  const lastSSTCountRef = useRef<number | null>(null);

  const tick = useCallback(async () => {
    try {
      const s = await GetStats();
      const prev = lastSSTCountRef.current;
      if (prev !== null) {
        if (s.ssTableCount > prev) {
          fireSpark("flush");
          setFlashFlush(true);
          window.setTimeout(() => setFlashFlush(false), 700);
        } else if (s.ssTableCount < prev) {
          fireSpark("compact");
          setFlashCompact(true);
          window.setTimeout(() => setFlashCompact(false), 900);
        }
      }
      lastSSTCountRef.current = s.ssTableCount;
      setLastSSTCount(s.ssTableCount);
      setStats(s);
    } catch {
      // ignore
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    void tick();
    const id = window.setInterval(() => void tick(), POLL_MS);
    return () => window.clearInterval(id);
  }, [tick]);

  // Subscribe to record events for write/delete sparks.
  useEffect(() => {
    let cancelled = false;
    void (async () => {
      try {
        const addr = await GetAPIAddr();
        if (!addr || cancelled) return;
        const es = new EventSource(withAuthQuery(`${addr}/api/realtime?topic=coll`));
        es.addEventListener("create", () => fireSpark("write"));
        es.addEventListener("update", () => fireSpark("write"));
        es.addEventListener("delete", () => fireSpark("delete"));
        const onClose = () => es.close();
        return onClose;
      } catch {
        // ignore
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  function fireSpark(kind: Spark["kind"]) {
    const id = nextSparkId.current++;
    setSparks((prev) => [...prev, { id, kind }].slice(-MAX_SPARKS));
    window.setTimeout(() => {
      setSparks((prev) => prev.filter((s) => s.id !== id));
    }, 1100);
  }

  const memBytes = stats?.memtableBytes ?? 0;
  const threshold = stats?.flushThresholdBytes ?? 1024 * 1024;
  const memPct = Math.min(100, (memBytes / threshold) * 100);
  const walBytes = stats?.walBytes ?? 0;
  const walPct = Math.min(100, (walBytes / threshold) * 100);
  const ssSizes = stats?.ssTableSizes ?? [];

  return (
    <div className="animate-slideUp space-y-5">
      <div className="card card-pad">
        <div className="flex items-center justify-between">
          <div>
            <div className="section-title">Data Lifecycle</div>
            <div className="section-sub">
              A live view of the LSM tree — Memtable filling up, WAL appending, SSTables flushing & compacting.
            </div>
          </div>
          <div className="flex gap-1.5">
            <Legend tone="copper">Memtable</Legend>
            <Legend tone="steel">WAL</Legend>
            <Legend tone="ink">SSTables</Legend>
          </div>
        </div>
      </div>

      {/* Memtable + WAL */}
      <div className="grid grid-cols-1 gap-5 lg:grid-cols-2">
        <Cell
          title="MEMTABLE (in-memory)"
          subtitle={`${fmt(memBytes)} / ${fmt(threshold)} · flush at 100%`}
          flash={flashFlush}
        >
          <FillBar pct={memPct} tone="copper" pulses={sparks.filter((s) => s.kind === "write" || s.kind === "delete")} threshold />
          <div className="mt-2 text-[11px] text-ink-400">
            Writes land here first. When the bar hits the threshold, the entire memtable
            is frozen, sorted, and flushed to disk as a new SSTable.
          </div>
        </Cell>

        <Cell title="WAL (append-only log)" subtitle={`${fmt(walBytes)} on disk · CRC32C per record`}>
          <FillBar pct={walPct} tone="steel" pulses={sparks.filter((s) => s.kind === "write")} />
          <div className="mt-2 text-[11px] text-ink-400">
            Every write is appended to <span className="font-mono">wal.bin</span> and fsync&apos;d before
            the in-memory map is updated. Replayed on startup for crash recovery.
          </div>
        </Cell>
      </div>

      {/* Flush arrow */}
      <div className="flex justify-center">
        <div className={`rounded-full border px-4 py-1 text-[11px] font-mono uppercase tracking-wider transition-all ${
          flashFlush ? "border-copper-500 bg-copper-50 text-copper-700 shadow-copper" : "border-canvas-200 bg-white text-ink-400"
        }`}>
          ↓ flush ↓
        </div>
      </div>

      {/* SSTables row */}
      <Cell
        title={`SSTABLES (immutable, on disk) · ${ssSizes.length} file${ssSizes.length === 1 ? "" : "s"}`}
        subtitle="Sorted, bloom-filtered, never modified after writing. Compaction merges them into fewer, larger files."
        flash={flashCompact}
      >
        <SSTableLane sizes={ssSizes} flashCompact={flashCompact} />
      </Cell>

      {/* Live spark feed */}
      <div className="card card-pad">
        <div className="flex items-center justify-between">
          <div className="section-title">Recent events</div>
          <span className="chip">{lastSSTCount ?? 0} SSTables</span>
        </div>
        <div className="mt-3 flex flex-wrap gap-2 min-h-[28px]">
          {sparks.length === 0 ? (
            <span className="text-[11.5px] text-ink-400">Waiting for activity…</span>
          ) : (
            sparks.map((s) => (
              <span
                key={s.id}
                className={`chip chip-mono animate-pulseCopper ${
                  s.kind === "flush" ? "border-copper-200 bg-copper-50 text-copper-700" :
                  s.kind === "compact" ? "border-steel-100 bg-steel-100 text-steel-700" :
                  s.kind === "delete" ? "border-red-200 bg-red-50 text-red-700" :
                  "chip-copper"
                }`}
              >
                {s.kind.toUpperCase()}
              </span>
            ))
          )}
        </div>
      </div>
    </div>
  );
}

/* ---------------- pieces ---------------- */

function Cell(props: { title: string; subtitle?: string; flash?: boolean; children: React.ReactNode }) {
  return (
    <div className={`card card-pad transition-shadow ${props.flash ? "shadow-copper" : ""}`}>
      <div className="mb-3">
        <div className="text-[11px] font-semibold uppercase tracking-widest text-ink-500">{props.title}</div>
        {props.subtitle && <div className="mt-0.5 text-[11.5px] text-ink-400">{props.subtitle}</div>}
      </div>
      {props.children}
    </div>
  );
}

function FillBar(props: { pct: number; tone: "copper" | "steel"; pulses?: Spark[]; threshold?: boolean }) {
  const grad =
    props.tone === "copper"
      ? "linear-gradient(90deg, #e07a25 0%, #ec9a4b 80%, #f4b87a 100%)"
      : "linear-gradient(90deg, #5b6b8a 0%, #7c8aa3 80%, #a9b6c8 100%)";
  const glow = props.tone === "copper" ? "rgba(224,122,37,0.45)" : "rgba(91,107,138,0.35)";

  return (
    <div className="relative h-7 w-full overflow-hidden rounded-md border border-canvas-200 bg-canvas-100">
      <div
        className="h-full transition-[width] duration-300 ease-out"
        style={{
          width: `${props.pct}%`,
          background: grad,
          boxShadow: `0 0 16px ${glow}`
        }}
      />
      <div
        className="absolute inset-y-0 left-0 px-2 text-[11px] font-mono leading-7 text-ink-900"
        style={{ mixBlendMode: "difference" as const, color: "white" }}
      >
        {props.pct.toFixed(0)}%
      </div>
      {props.threshold && (
        <div className="pointer-events-none absolute inset-y-0 right-0 w-px bg-copper-700" />
      )}
      {(props.pulses ?? []).slice(-4).map((s, i) => (
        <div
          key={s.id}
          className="pointer-events-none absolute top-1/2 h-2 w-2 -translate-y-1/2 rounded-full bg-white shadow"
          style={{
            left: `calc(${props.pct}% - 10px)`,
            transform: `translate(${-i * 6}px, -50%)`,
            opacity: 1 - i * 0.2,
            transition: "opacity 200ms"
          }}
        />
      ))}
    </div>
  );
}

function SSTableLane(props: { sizes: number[]; flashCompact: boolean }) {
  const max = Math.max(1, ...props.sizes);
  if (props.sizes.length === 0) {
    return (
      <div className="flex items-center justify-center rounded-md border border-dashed border-canvas-300 bg-canvas-100 py-10 text-[12px] text-ink-400">
        No SSTables on disk yet — keep writing to trigger a flush.
      </div>
    );
  }
  return (
    <div className={`flex gap-2 overflow-x-auto py-2 ${props.flashCompact ? "animate-pulseCopper" : ""}`}>
      {props.sizes.map((sz, i) => {
        const h = 28 + Math.round((sz / max) * 56);
        return (
          <div
            key={i}
            className="group flex min-w-[80px] flex-col items-center"
            title={`SSTable #${i + 1} — ${fmt(sz)}`}
          >
            <div
              className="w-[64px] rounded-md border border-gunmetal-700 bg-gradient-to-b from-gunmetal-700 to-gunmetal-900 shadow-sm transition-all duration-300"
              style={{ height: h }}
            />
            <div className="mt-1.5 text-[10px] font-mono text-ink-400">{fmt(sz)}</div>
            <div className="text-[9px] font-mono uppercase tracking-wider text-ink-300">#{i + 1}</div>
          </div>
        );
      })}
    </div>
  );
}

function Legend({ tone, children }: { tone: "copper" | "steel" | "ink"; children: React.ReactNode }) {
  const cls =
    tone === "copper" ? "chip-copper" :
    tone === "steel"  ? "chip-steel"  :
                        "";
  return <span className={`chip chip-mono ${cls}`}>{children}</span>;
}

function fmt(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return "0 B";
  const units = ["B", "KB", "MB", "GB"];
  let v = bytes;
  let i = 0;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return `${v.toFixed(i === 0 ? 0 : 1)} ${units[i]}`;
}
