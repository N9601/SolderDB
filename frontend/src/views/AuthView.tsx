import { useState } from "react";
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
    <div className="flex h-screen w-screen items-center justify-center bg-canvas-50 p-6">
      <div className="grid w-full max-w-4xl grid-cols-1 overflow-hidden rounded-2xl border border-canvas-200 bg-white shadow-cardHover lg:grid-cols-2">
        {/* Left: brand panel */}
        <aside className="hidden flex-col justify-between bg-gunmetal-900 p-10 text-canvas-200 lg:flex">
          <div>
            <Logo size={40} withWordmark variant="dark" />
            <div className="mt-10 max-w-[320px]">
              <div className="text-[20px] font-semibold text-white">Local-first.</div>
              <div className="text-[20px] font-semibold text-white">Built from scratch.</div>
              <div className="text-[20px] font-semibold text-copper-400">Yours to deploy anywhere.</div>
              <div className="mt-4 text-[13px] leading-relaxed text-canvas-300">
                SolderDB is a single-binary database — LSM engine, REST API, auth, collections — running on
                your machine, no servers required.
              </div>
            </div>
          </div>
          <div className="space-y-2 text-[11px] text-canvas-300">
            <Feature label="LSM engine · Memtable + WAL + SSTables" />
            <Feature label="CRC32C-checked log · bloom-filtered reads" />
            <Feature label="Collections + schema validation" />
            <Feature label="REST API on localhost:8787" />
          </div>
        </aside>

        {/* Right: form */}
        <section className="flex flex-col justify-center p-10">
          <div className="lg:hidden mb-6">
            <Logo size={28} withWordmark variant="light" />
          </div>

          <h1 className="text-[22px] font-semibold tracking-tight text-ink-900">
            {mode === "login" ? "Welcome back" : "Create your admin"}
          </h1>
          <p className="mt-1 text-[13px] text-ink-500">
            {mode === "login"
              ? "Sign in to your local database."
              : "The first account becomes the admin of this instance."}
          </p>

          <div className="mt-6 space-y-3">
            <div>
              <label className="label">Email</label>
              <input
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                className="field mt-1"
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
                className="field mt-1"
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
              className="btn btn-primary w-full justify-center"
              disabled={busy || !email || password.length < 8}
              onClick={() => void submit()}
            >
              {busy ? "…" : mode === "login" ? "Sign in" : "Create account"}
            </button>
          </div>

          <div className="mt-5 text-center text-[12px] text-ink-500">
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
