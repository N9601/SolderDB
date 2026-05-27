import { useEffect, useState } from "react";
import { ChangePassword } from "../wailsjs/go/bridge/AuthService";
import { bridge } from "../wailsjs/go/models";
import { useToast } from "../components/Toast";
import { applyTheme, loadTheme, saveTheme, watchSystem, type Theme } from "../lib/theme";
import { getToken } from "../lib/apiFetch";

type Props = {
  user: bridge.User;
  onSignOut: () => void;
};

export default function ProfileView({ user, onSignOut }: Props) {
  const toast = useToast();
  const [theme, setThemeState] = useState<Theme>(() => loadTheme());

  useEffect(() => {
    applyTheme(theme);
    saveTheme(theme);
  }, [theme]);

  useEffect(() => {
    if (theme !== "system") return;
    const unsub = watchSystem(() => applyTheme("system"));
    return unsub;
  }, [theme]);

  return (
    <div className="space-y-5">
      <ProfileHeader user={user} />

      <div className="grid grid-cols-1 gap-5 lg:grid-cols-2">
        <AccountCard user={user} />
        <AppearanceCard theme={theme} onChange={setThemeState} />
        <PasswordCard userId={user.id} onSuccess={() => toast.push("Password updated", "success")} />
        <SessionCard onSignOut={onSignOut} />
      </div>

      <DangerZone onSignOut={onSignOut} />
    </div>
  );
}

/* ---------------- Pieces ---------------- */

function ProfileHeader({ user }: { user: bridge.User }) {
  const handle = user.email.split("@")[0] ?? user.email;
  return (
    <div className="relative overflow-hidden rounded-2xl border border-canvas-200 bg-white p-6 shadow-card">
      <div
        className="pointer-events-none absolute -right-24 -top-24 h-60 w-60 rounded-full opacity-40 blur-3xl"
        style={{ background: "radial-gradient(circle, rgba(224,122,37,0.25) 0%, transparent 70%)" }}
      />
      <div className="relative flex items-center gap-5">
        <div className="flex h-16 w-16 items-center justify-center rounded-2xl bg-gradient-to-br from-copper-500 to-copper-700 text-[24px] font-semibold text-white shadow-copper">
          {user.email.charAt(0).toUpperCase()}
        </div>
        <div className="min-w-0">
          <div className="text-[12px] font-medium uppercase tracking-[0.14em] text-ink-400">Profile</div>
          <h1 className="mt-0.5 truncate text-[22px] font-semibold tracking-tight text-ink-900">{handle}</h1>
          <div className="mt-1 flex items-center gap-2 text-[12px] text-ink-500">
            <span className="font-mono">{user.email}</span>
            <span className="text-ink-300">·</span>
            <span className={`chip chip-mono ${user.role === "admin" ? "chip-copper" : "chip-steel"}`}>{user.role}</span>
          </div>
        </div>
      </div>
    </div>
  );
}

function AccountCard({ user }: { user: bridge.User }) {
  return (
    <div className="card card-pad">
      <div className="section-title">Account</div>
      <div className="section-sub">Read-only details about your user record.</div>
      <dl className="mt-4 space-y-3 text-[13px]">
        <Row label="User ID">
          <code className="text-[11.5px] text-ink-700 break-all">{user.id}</code>
        </Row>
        <Row label="Email">{user.email}</Row>
        <Row label="Role">
          <span className={`chip chip-mono ${user.role === "admin" ? "chip-copper" : "chip-steel"}`}>
            {user.role}
          </span>
        </Row>
        <Row label="Created">{fmtDate(user.created)}</Row>
        <Row label="Updated">{fmtDate(user.updated)}</Row>
      </dl>
    </div>
  );
}

function AppearanceCard({ theme, onChange }: { theme: Theme; onChange: (t: Theme) => void }) {
  const options: { id: Theme; label: string; hint: string; icon: React.ReactNode }[] = [
    {
      id: "light",
      label: "Light",
      hint: "Bright workspace",
      icon: (
        <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7">
          <circle cx="12" cy="12" r="4" />
          <path d="M12 2v2M12 20v2M2 12h2M20 12h2M4.93 4.93l1.41 1.41M17.66 17.66l1.41 1.41M4.93 19.07l1.41-1.41M17.66 6.34l1.41-1.41" />
        </svg>
      )
    },
    {
      id: "dark",
      label: "Dark",
      hint: "Easier at night",
      icon: (
        <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7">
          <path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z" />
        </svg>
      )
    },
    {
      id: "system",
      label: "System",
      hint: "Match your OS",
      icon: (
        <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7">
          <rect x="2" y="4" width="20" height="14" rx="2" />
          <line x1="8" y1="22" x2="16" y2="22" />
          <line x1="12" y1="18" x2="12" y2="22" />
        </svg>
      )
    }
  ];
  return (
    <div className="card card-pad">
      <div className="section-title">Appearance</div>
      <div className="section-sub">Persists across sessions on this machine.</div>
      <div className="mt-4 grid grid-cols-3 gap-2">
        {options.map((o) => (
          <button
            key={o.id}
            onClick={() => onChange(o.id)}
            className={`flex flex-col items-start gap-2 rounded-lg border p-3 text-left transition-all ${
              theme === o.id
                ? "border-copper-500 bg-copper-50 shadow-glow"
                : "border-canvas-200 bg-white hover:border-canvas-300 hover:bg-canvas-100"
            }`}
          >
            <span className={theme === o.id ? "text-copper-600" : "text-ink-500"}>{o.icon}</span>
            <div>
              <div className={`text-[13px] font-medium ${theme === o.id ? "text-ink-900" : "text-ink-700"}`}>
                {o.label}
              </div>
              <div className="text-[11px] text-ink-400">{o.hint}</div>
            </div>
          </button>
        ))}
      </div>
    </div>
  );
}

function PasswordCard({ userId, onSuccess }: { userId: string; onSuccess: () => void }) {
  const [current, setCurrent] = useState("");
  const [next, setNext] = useState("");
  const [confirm, setConfirm] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  async function submit() {
    setError("");
    if (next.length < 8) {
      setError("New password must be at least 8 characters.");
      return;
    }
    if (next !== confirm) {
      setError("New password and confirmation don't match.");
      return;
    }
    setBusy(true);
    try {
      await ChangePassword(userId, current, next);
      setCurrent("");
      setNext("");
      setConfirm("");
      onSuccess();
    } catch (e) {
      setError(String(e).replace(/^Error:\s*/, ""));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="card card-pad">
      <div className="section-title">Change password</div>
      <div className="section-sub">8+ characters. Existing sessions stay signed in.</div>
      <div className="mt-4 space-y-3">
        <div>
          <label className="label">Current password</label>
          <input
            type="password"
            value={current}
            onChange={(e) => setCurrent(e.target.value)}
            className="field mt-1"
            autoComplete="current-password"
          />
        </div>
        <div>
          <label className="label">New password</label>
          <input
            type="password"
            value={next}
            onChange={(e) => setNext(e.target.value)}
            className="field mt-1"
            autoComplete="new-password"
          />
        </div>
        <div>
          <label className="label">Confirm new password</label>
          <input
            type="password"
            value={confirm}
            onChange={(e) => setConfirm(e.target.value)}
            className="field mt-1"
            autoComplete="new-password"
          />
        </div>
        {error && (
          <div className="rounded-md border border-red-200 bg-red-50 px-3 py-2 text-[12px] text-red-700">
            {error}
          </div>
        )}
        <div className="flex justify-end">
          <button
            className="btn btn-primary"
            disabled={busy || !current || !next || !confirm}
            onClick={() => void submit()}
          >
            {busy ? "Updating…" : "Update password"}
          </button>
        </div>
      </div>
    </div>
  );
}

function SessionCard({ onSignOut }: { onSignOut: () => void }) {
  const [revealed, setRevealed] = useState(false);
  const [copied, setCopied] = useState(false);
  const token = getToken();
  const masked = token ? token.slice(0, 12) + "…" + token.slice(-8) : "—";

  function copy() {
    if (!token) return;
    navigator.clipboard.writeText(token).then(() => {
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1500);
    });
  }

  return (
    <div className="card card-pad">
      <div className="section-title">Current session</div>
      <div className="section-sub">Bearer token for the local REST API. Treat it like a password.</div>
      <div className="mt-4 space-y-3">
        <div className="rounded-lg border border-canvas-200 bg-canvas-100 p-3">
          <div className="label">Token</div>
          <code className="mt-1 block break-all font-mono text-[11.5px] text-ink-700">
            {revealed ? token : masked}
          </code>
          <div className="mt-3 flex gap-2">
            <button className="btn btn-ghost" onClick={() => setRevealed((r) => !r)}>
              {revealed ? "Hide" : "Reveal"}
            </button>
            <button className="btn btn-ghost" onClick={copy}>
              {copied ? "Copied!" : "Copy"}
            </button>
          </div>
        </div>
        <button className="btn w-full justify-center" onClick={onSignOut}>
          Sign out of this device
        </button>
      </div>
    </div>
  );
}

function DangerZone({ onSignOut }: { onSignOut: () => void }) {
  return (
    <div className="card card-pad border-red-200 bg-red-50/40">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <div className="text-[13px] font-semibold text-red-700">Sign out</div>
          <div className="text-[12px] text-red-600/80">
            Clears the local session token. You&apos;ll need to log in again to access this database.
          </div>
        </div>
        <button className="btn btn-danger" onClick={onSignOut}>
          Sign out
        </button>
      </div>
    </div>
  );
}

/* ---------------- helpers ---------------- */

function Row({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex items-start justify-between gap-4">
      <dt className="w-28 flex-shrink-0 text-[11px] font-medium uppercase tracking-wider text-ink-400">
        {label}
      </dt>
      <dd className="flex-1 text-right text-ink-900">{children}</dd>
    </div>
  );
}

function fmtDate(iso: string): string {
  if (!iso) return "—";
  try {
    return new Date(iso).toLocaleString();
  } catch {
    return iso;
  }
}
