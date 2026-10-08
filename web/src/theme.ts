// Theme helpers.
//
// Colors are read live from the CSS tokens (:root in index.css) so JS-driven
// colors — chiefly SweetAlert2 dialog options — automatically follow the active
// theme (dark by default, light when data-theme="light"). Access these at call
// time (e.g. inside a Swal.fire) so they reflect the current theme.

function cssVar(name: string): string {
  if (typeof document === "undefined") return "";
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim();
}

export const theme = {
  get bg() { return cssVar("--bg"); },
  get surface() { return cssVar("--surface"); },
  get surface2() { return cssVar("--surface-2"); },
  get border() { return cssVar("--border"); },
  get borderStrong() { return cssVar("--border-strong"); },
  get text() { return cssVar("--text"); },
  get muted() { return cssVar("--muted"); },
  get accent() { return cssVar("--accent"); },
  get accentHover() { return cssVar("--accent-hover"); },
  get success() { return cssVar("--success"); },
  get warn() { return cssVar("--warn"); },
  get danger() { return cssVar("--danger"); },
};

// ---- Theme mode (per-user, persisted in localStorage) --------------------
export type ThemeMode = "dark" | "light";
const THEME_KEY = "theme-mode";

export function getThemeMode(): ThemeMode {
  try {
    return localStorage.getItem(THEME_KEY) === "light" ? "light" : "dark";
  } catch {
    return "dark";
  }
}

// applyThemeMode sets the <html data-theme> attribute (which drives the CSS
// token overrides) and remembers the choice for this user/browser.
export function applyThemeMode(mode: ThemeMode): void {
  document.documentElement.setAttribute("data-theme", mode);
  try {
    localStorage.setItem(THEME_KEY, mode);
  } catch {
    /* private mode / storage disabled — theme still applies for this session */
  }
}
