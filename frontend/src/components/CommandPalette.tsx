import { useEffect, useMemo, useRef, useState } from "react";

export type CommandItem = {
  id: string;
  label: string;
  hint?: string;
  group: string;
  shortcut?: string;
  run: () => void;
};

type Props = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  items: CommandItem[];
};

export function CommandPalette({ open, onOpenChange, items }: Props) {
  const [query, setQuery] = useState("");
  const [active, setActive] = useState(0);
  const inputRef = useRef<HTMLInputElement | null>(null);

  useEffect(() => {
    if (open) {
      setQuery("");
      setActive(0);
      window.setTimeout(() => inputRef.current?.focus(), 0);
    }
  }, [open]);

  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      const cmd = e.metaKey || e.ctrlKey;
      if (cmd && e.key.toLowerCase() === "k") {
        e.preventDefault();
        onOpenChange(!open);
      } else if (e.key === "Escape" && open) {
        onOpenChange(false);
      }
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open, onOpenChange]);

  const filtered = useMemo(() => {
    if (!query.trim()) return items;
    const q = query.toLowerCase();
    return items
      .map((it) => ({ it, score: score(it, q) }))
      .filter((x) => x.score > 0)
      .sort((a, b) => b.score - a.score)
      .map((x) => x.it);
  }, [query, items]);

  useEffect(() => {
    if (active >= filtered.length) setActive(Math.max(0, filtered.length - 1));
  }, [filtered.length, active]);

  if (!open) return null;

  function runSelected() {
    const it = filtered[active];
    if (!it) return;
    onOpenChange(false);
    it.run();
  }

  function onInputKey(e: React.KeyboardEvent) {
    if (e.key === "ArrowDown") {
      e.preventDefault();
      setActive((a) => Math.min(filtered.length - 1, a + 1));
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      setActive((a) => Math.max(0, a - 1));
    } else if (e.key === "Enter") {
      e.preventDefault();
      runSelected();
    }
  }

  const grouped = groupBy(filtered, (it) => it.group);

  return (
    <div
      className="fixed inset-0 z-[120] flex items-start justify-center bg-ink-900/45 backdrop-blur-md p-4"
      style={{ paddingTop: "12vh" }}
      onClick={() => onOpenChange(false)}
    >
      <div
        className="w-full max-w-[640px] overflow-hidden rounded-2xl border border-canvas-200 bg-white shadow-cardHover animate-slideUp"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center gap-3 border-b border-canvas-200 px-4 py-3">
          <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" className="text-ink-400">
            <circle cx="11" cy="11" r="7" />
            <line x1="21" y1="21" x2="16.65" y2="16.65" />
          </svg>
          <input
            ref={inputRef}
            value={query}
            onChange={(e) => {
              setQuery(e.target.value);
              setActive(0);
            }}
            onKeyDown={onInputKey}
            placeholder="Jump to anywhere · type to search"
            className="w-full bg-transparent text-[14px] text-ink-900 outline-none placeholder:text-ink-300"
            spellCheck={false}
          />
          <kbd className="hidden font-mono text-[10px] text-ink-400 sm:inline">ESC</kbd>
        </div>

        <div className="max-h-[60vh] overflow-y-auto">
          {filtered.length === 0 ? (
            <div className="px-4 py-10 text-center text-[12.5px] text-ink-400">
              Nothing matches “{query}”.
            </div>
          ) : (
            Object.entries(grouped).map(([group, list]) => (
              <div key={group} className="py-1.5">
                <div className="px-4 py-1.5 text-[10px] font-semibold uppercase tracking-[0.12em] text-ink-400">
                  {group}
                </div>
                {list.map((it) => {
                  const idx = filtered.indexOf(it);
                  const isActive = idx === active;
                  return (
                    <button
                      key={it.id}
                      onMouseEnter={() => setActive(idx)}
                      onClick={() => {
                        onOpenChange(false);
                        it.run();
                      }}
                      className={`flex w-full items-center gap-3 border-l-2 px-4 py-2 text-left text-[13px] transition-colors ${
                        isActive
                          ? "border-copper-500 bg-copper-50 text-ink-900"
                          : "border-transparent text-ink-700 hover:bg-canvas-100"
                      }`}
                    >
                      <span className="flex-1 truncate">
                        {it.label}
                        {it.hint && (
                          <span className="ml-2 text-[11.5px] text-ink-400">{it.hint}</span>
                        )}
                      </span>
                      {it.shortcut && (
                        <kbd className="rounded border border-canvas-300 bg-canvas-100 px-1.5 py-0.5 font-mono text-[10px] text-ink-500">
                          {it.shortcut}
                        </kbd>
                      )}
                    </button>
                  );
                })}
              </div>
            ))
          )}
        </div>

        <div className="flex items-center justify-between border-t border-canvas-200 bg-canvas-50 px-4 py-2 text-[10.5px] text-ink-400">
          <span className="flex items-center gap-1">
            <Kbd>↑</Kbd>
            <Kbd>↓</Kbd>
            <span>navigate</span>
          </span>
          <span className="flex items-center gap-1">
            <Kbd>↵</Kbd>
            <span>select</span>
            <span className="mx-2 opacity-50">·</span>
            <Kbd>ESC</Kbd>
            <span>close</span>
          </span>
        </div>
      </div>
    </div>
  );
}

function Kbd({ children }: { children: React.ReactNode }) {
  return (
    <kbd className="rounded border border-canvas-300 bg-white px-1.5 py-0.5 font-mono text-[10px] text-ink-500">
      {children}
    </kbd>
  );
}

function score(it: CommandItem, q: string): number {
  const hay = (it.label + " " + (it.hint ?? "") + " " + it.group).toLowerCase();
  if (hay === q) return 1000;
  if (hay.startsWith(q)) return 500;
  if (hay.includes(q)) return 300;
  // Subsequence match (Cmd-K style).
  let i = 0;
  for (const ch of hay) {
    if (ch === q[i]) {
      i++;
      if (i === q.length) return 100 + q.length;
    }
  }
  return 0;
}

function groupBy<T>(items: T[], key: (t: T) => string): Record<string, T[]> {
  const out: Record<string, T[]> = {};
  for (const it of items) {
    const k = key(it);
    out[k] ??= [];
    out[k].push(it);
  }
  return out;
}
