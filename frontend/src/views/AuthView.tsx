import { useEffect, useState } from "react";
import { Login, Register } from "../wailsjs/go/bridge/AuthService";
import { bridge } from "../wailsjs/go/models";
import { Logo } from "../components/Logo";

type Mode = "login" | "register";

export default function AuthView(props: { onSignedIn: (sess: bridge.Session) => void }) {
  const [mode, setMode] = useState<Mode>("login");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string>("");
  const [busy, setBusy] = useState(false);
  const [mounted, setMounted] = useState(false);

  useEffect(() => {
    // Defer mount-class so transitions fire on initial render
    const t = window.setTimeout(() => setMounted(true), 16);
    return () => window.clearTimeout(t);
  }, []);

  async function submit() {
    setError("");
    setBusy(true);
    try {
      const sess = mode === "login" ? await Login(email, password) : await Register(email, password);
      props.onSignedIn(sess);
    } catch (e) {
      setError(String(e).replace(/^Error:\s*/, ""));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="relative flex h-screen w-screen items-center justify-center overflow-hidden bg-canvas-50 p-6">
      {/* Animated background */}
      <BackgroundOrbs />
      <GridOverlay />

      <div
        className={`relative grid w-full max-w-[1040px] grid-cols-1 overflow-hidden rounded-[20px] border border-canvas-200 bg-white/95 shadow-cardHover backdrop-blur-md transition-all duration-500 lg:grid-cols-[1.05fr_1fr] ${
          mounted ? "translate-y-0 opacity-100" : "translate-y-2 opacity-0"
        }`}
      >
        {/* Left: brand panel */}
        <aside className="relative hidden flex-col justify-between overflow-hidden bg-gunmetal-900 p-10 text-canvas-200 lg:flex">
          {/* Decorative inside-panel glow */}
          <div
            className="pointer-events-none absolute -right-32 -top-32 h-72 w-72 rounded-full opacity-40 blur-3xl"
            style={{ background: "radial-gradient(circle, rgba(224,122,37,0.55) 0%, transparent 70%)" }}
          />
          <div
            className="pointer-events-none absolute -bottom-24 -left-16 h-64 w-64 rounded-full opacity-25 blur-3xl"
            style={{ background: "radial-gradient(circle, rgba(91,107,138,0.6) 0%, transparent 70%)" }}
          />
          <div className="relative">
            <Logo size={44} withWordmark variant="dark" />
            <div className="mt-12 max-w-[340px]">
              <div className="text-[28px] font-semibold leading-[1.15] tracking-tight text-white">
                Local-first.
                <br />
                Built from scratch.
                <br />
                <span className="text-copper-400">Yours to deploy anywhere.</span>
              </div>
              <div className="mt-5 text-[13px] leading-relaxed text-canvas-300">
                SolderDB is a single binary — LSM engine, REST API, auth, collections, files, realtime —
                running on your machine. No servers required.
              </div>
            </div>
          </div>

          <div className="relative space-y-2.5 text-[11.5px] text-canvas-300">
            <Feature label="LSM engine · Memtable + WAL + SSTables" />
            <Feature label="CRC32C-checked log · bloom-filtered reads" />
            <Feature label="Hardware-aware compaction" />
            <Feature label="REST + WebSocket realtime on localhost:8787" />
            <Feature label="JS & Go SDKs · CLI · Postman-style explorer" />
          </div>
        </aside>

        {/* Right: form */}
        <section className="flex flex-col justify-center px-10 py-12 lg:px-12">
          <div className="lg:hidden mb-6">
            <Logo size={28} withWordmark variant="light" />
          </div>

          <h1 className="text-[26px] font-semibold tracking-tight text-ink-900">
            {mode === "login" ? "Welcome back" : "Create your admin"}
          </h1>
          <p className="mt-1.5 text-[13.5px] text-ink-500">
            {mode === "login"
              ? "Sign in to your local database."
              : "The first account becomes the admin of this instance."}
          </p>

          <div className="mt-7 space-y-3.5">
            <div>
              <label className="label">Email</label>
              <input
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                className="field mt-1.5"
                placeholder="you@example.com"
                autoComplete="username"
                spellCheck={false}
              />
            </div>
            <div>
              <label className="label">Password</label>
              <input
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Enter") void submit();
                }}
                className="field mt-1.5"
                placeholder="at least 8 characters"
                autoComplete={mode === "login" ? "current-password" : "new-password"}
              />
            </div>

            {error && (
              <div className="rounded-md border border-red-200 bg-red-50 px-3 py-2 text-[12px] text-red-700">
                {error}
              </div>
            )}

            <button
              className="btn btn-primary w-full justify-center py-2.5 text-[13px]"
              disabled={busy || !email || password.length < 8}
              onClick={() => void submit()}
            >
              {busy ? (
                <span className="inline-flex items-center gap-2">
                  <Spinner /> Working…
                </span>
              ) : mode === "login" ? (
                "Sign in"
              ) : (
                "Create account"
              )}
            </button>
          </div>

          <div className="mt-6 text-center text-[12.5px] text-ink-500">
            {mode === "login" ? (
              <>
                Don&apos;t have an account?{" "}
                <button className="text-copper-600 hover:underline" onClick={() => setMode("register")}>
                  Register
                </button>
              </>
            ) : (
              <>
                Already have one?{" "}
                <button className="text-copper-600 hover:underline" onClick={() => setMode("login")}>
                  Sign in
                </button>
              </>
            )}
          </div>

          <div className="mt-8 flex items-center justify-center gap-2 text-[10.5px] uppercase tracking-[0.15em] text-ink-300">
            <span className="dot dot-idle" /> running locally · nothing leaves this machine
          </div>
        </section>
      </div>
    </div>
  );
}

function Feature({ label }: { label: string }) {
  return (
    <div className="flex items-center gap-2">
      <span className="dot" />
      <span>{label}</span>
    </div>
  );
}

function Spinner() {
  return (
    <svg className="animate-spin" width="14" height="14" viewBox="0 0 24 24" fill="none">
      <circle cx="12" cy="12" r="9" stroke="currentColor" strokeOpacity="0.25" strokeWidth="2.5" />
      <path d="M21 12a9 9 0 0 1-9 9" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" />
    </svg>
  );
}

function BackgroundOrbs() {
  return (
    <>
      <div
        className="pointer-events-none absolute -left-32 top-12 h-[420px] w-[420px] rounded-full opacity-50 blur-3xl"
        style={{ background: "radial-gradient(circle, rgba(224,122,37,0.18) 0%, transparent 70%)" }}
      />
      <div
        className="pointer-events-none absolute -right-24 bottom-0 h-[460px] w-[460px] rounded-full opacity-40 blur-3xl"
        style={{ background: "radial-gradient(circle, rgba(91,107,138,0.18) 0%, transparent 70%)" }}
      />
    </>
  );
}

function GridOverlay() {
  return (
    <div
      className="pointer-events-none absolute inset-0 opacity-[0.35]"
      style={{
        backgroundImage:
          "linear-gradient(rgba(15,17,21,0.04) 1px, transparent 1px), linear-gradient(90deg, rgba(15,17,21,0.04) 1px, transparent 1px)",
        backgroundSize: "32px 32px",
        maskImage: "radial-gradient(ellipse at center, black 30%, transparent 75%)",
        WebkitMaskImage: "radial-gradient(ellipse at center, black 30%, transparent 75%)"
      }}
    />
  );
}
