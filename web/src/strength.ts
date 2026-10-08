// strength.ts — the single source of truth for password strength across the app
// (admin passwords, account passwords, and the Vault Health audit).
//
// We judge against an OFFLINE attack on a leaked hash (~1e10 guesses/sec), which
// is the real threat for a vault — NOT zxcvbn's `score`, which is calibrated for
// throttled online attacks and over-rates things like "whatthefuckinghel".
import zxcvbn from "zxcvbn";

export type StrengthLabel = "weak" | "fair" | "strong" | "excellent";

// Thresholds are log10 of the guess count (offline crack time at 1e10/sec).
export function labelFromLog10(log10Guesses: number): StrengthLabel {
  if (log10Guesses < 13) return "weak"; // < ~17 minutes
  if (log10Guesses < 16) return "fair"; // < ~months
  if (log10Guesses < 19) return "strong"; // < ~decades
  return "excellent"; // centuries+
}

export interface Strength {
  label: StrengthLabel;
  log10: number;
  crackSeconds: number;
}

export function passwordStrength(pw: string): Strength {
  const r = zxcvbn(pw);
  const log10 = Math.log10(r.guesses);
  return {
    label: labelFromLog10(log10),
    log10,
    crackSeconds: Number(r.crack_times_seconds.offline_fast_hashing_1e10_per_second),
  };
}

type TFunc = (key: string, opts?: Record<string, unknown>) => string;

// Largest-unit buckets (ascending) for humanizing a duration in seconds.
const CRACK_UNITS: { secs: number; key: string }[] = [
  { secs: 1, key: "seconds" },
  { secs: 60, key: "minutes" },
  { secs: 3600, key: "hours" },
  { secs: 86400, key: "days" },
  { secs: 2592000, key: "months" },   // 30 days
  { secs: 31536000, key: "years" },   // 365 days
];

// Turn a crack-time in seconds into a localized, pluralized string.
export function formatCrackTime(t: TFunc, seconds: number): string {
  if (!isFinite(seconds) || seconds < 1) return t("crack.instant");
  if (seconds >= 31536000 * 100) return t("crack.centuries");
  let chosen = CRACK_UNITS[0];
  for (const u of CRACK_UNITS) if (seconds >= u.secs) chosen = u;
  const count = Math.round(seconds / chosen.secs);
  return t(`crack.${chosen.key}`, { count });
}
