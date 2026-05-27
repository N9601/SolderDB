import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from "react";

type Tone = "info" | "success" | "warn" | "error";

type Toast = {
  id: number;
  text: string;
  tone: Tone;
  detail?: string;
};

type ToastContext = {
  push: (text: string, tone?: Tone, detail?: string) => void;
};

const Ctx = createContext<ToastContext | null>(null);

export function ToastProvider({ children }: { children: React.ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([]);
  const nextId = useRef(1);

  const push = useCallback((text: string, tone: Tone = "info", detail?: string) => {
    const id = nextId.current++;
    setToasts((prev) => [...prev, { id, text, tone, detail }]);
    window.setTimeout(() => {
      setToasts((prev) => prev.filter((t) => t.id !== id));
    }, tone === "error" ? 6000 : 3200);
  }, []);

  const value = useMemo<ToastContext>(() => ({ push }), [push]);

  return (
    <Ctx.Provider value={value}>
      {children}
      <div className="pointer-events-none fixed bottom-6 right-6 z-[100] flex flex-col-reverse gap-2">
        {toasts.map((t) => (
          <ToastChip key={t.id} toast={t} onDismiss={() => setToasts((prev) => prev.filter((x) => x.id !== t.id))} />
        ))}
      </div>
    </Ctx.Provider>
  );
}

export function useToast(): ToastContext {
  const ctx = useContext(Ctx);
  if (!ctx) throw new Error("useToast must be used inside <ToastProvider>");
  return ctx;
}

function ToastChip({ toast, onDismiss }: { toast: Toast; onDismiss: () => void }) {
  const cls =
    toast.tone === "success" ? "border-emerald-200 bg-white text-emerald-800" :
    toast.tone === "warn"    ? "border-amber-200  bg-white text-amber-800" :
    toast.tone === "error"   ? "border-red-200    bg-white text-red-800" :
                                "border-canvas-200 bg-white text-ink-700";

  const dot =
    toast.tone === "success" ? "bg-emerald-500" :
    toast.tone === "warn"    ? "bg-amber-500" :
    toast.tone === "error"   ? "bg-red-500" :
                                "bg-copper-500";

  return (
    <div
      className={`pointer-events-auto flex min-w-[260px] max-w-[420px] items-start gap-3 rounded-xl border px-3.5 py-3 shadow-cardHover backdrop-blur-sm animate-slideUp ${cls}`}
      role="status"
    >
      <span className={`mt-1.5 inline-block h-2 w-2 flex-shrink-0 rounded-full ${dot}`} />
      <div className="flex-1 min-w-0">
        <div className="text-[12.5px] font-medium leading-snug">{toast.text}</div>
        {toast.detail && (
          <div className="mt-0.5 truncate font-mono text-[11px] text-ink-400" title={toast.detail}>
            {toast.detail}
          </div>
        )}
      </div>
      <button
        onClick={onDismiss}
        className="flex-shrink-0 text-ink-300 transition-colors hover:text-ink-700"
        aria-label="Dismiss"
      >
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
          <line x1="6" y1="6" x2="18" y2="18" />
          <line x1="18" y1="6" x2="6" y2="18" />
        </svg>
      </button>
    </div>
  );
}

/** Hook a toast on every change of a status string. Lets existing code keep
 *  calling setStatus(...) while users get toast-style feedback automatically. */
export function useStatusToast(status: string) {
  const { push } = useToast();
  const last = useRef("");
  useEffect(() => {
    if (!status || status === last.current) return;
    last.current = status;
    const lower = status.toLowerCase();
    const tone: Tone =
      lower.includes("error") || lower.includes("fail") ? "error" :
      lower.includes("throttle") || lower.includes("warn") ? "warn" :
      lower.includes("ready") ? "info" :
      "success";
    push(status, tone);
  }, [status, push]);
}
