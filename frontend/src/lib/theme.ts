// Theme management — light / dark / system. Persisted to localStorage.
// Applies to <html> via data-theme so CSS variables can switch.

export type Theme = "light" | "dark" | "system";

const KEY = "solderdb.theme";

export function loadTheme(): Theme {
  const v = window.localStorage.getItem(KEY);
  if (v === "light" || v === "dark" || v === "system") return v;
  return "system";
}

export function saveTheme(t: Theme): void {
  window.localStorage.setItem(KEY, t);
}

/** Resolve "system" against the OS preference; light/dark return as-is. */
export function effectiveTheme(t: Theme): "light" | "dark" {
  if (t === "system") {
    return window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
  }
  return t;
}

export function applyTheme(t: Theme): void {
  const eff = effectiveTheme(t);
  document.documentElement.setAttribute("data-theme", eff);
}

/** Subscribe to OS-level theme changes; only fires when current setting is "system". */
export function watchSystem(handler: () => void): () => void {
  const m = window.matchMedia("(prefers-color-scheme: dark)");
  const fn = () => handler();
  m.addEventListener("change", fn);
  return () => m.removeEventListener("change", fn);
}
