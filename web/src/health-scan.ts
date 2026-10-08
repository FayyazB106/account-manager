// health-scan.ts — runs the Vault Health scan and caches the result. Shared by
// the Health page and the post-login background scan, so there's one code path.
//
// The cache holds labels/counts/service names only (no plaintext passwords).
import { listAccounts, revealAllSecrets } from "./api";
import { analyze, type HealthReport } from "./health";

const CACHE_KEY = "am_health_report";

export interface CachedHealth {
  report: HealthReport;
  ts: number;
}

export function loadHealthCache(): CachedHealth | null {
  try {
    const raw = localStorage.getItem(CACHE_KEY);
    return raw ? (JSON.parse(raw) as CachedHealth) : null;
  } catch {
    return null;
  }
}

// runHealthScan fetches all passwords in one bulk request (logged once as a
// Vault Health scan, not one reveal per account), analyzes them client-side,
// caches the report, and returns it.
export async function runHealthScan(): Promise<HealthReport> {
  const [accounts, secrets] = await Promise.all([listAccounts(), revealAllSecrets()]);
  const items = accounts.map((a) => ({
    id: a.id,
    service: a.service,
    password: secrets[a.id] ?? "",
    changedAt: a.password_changed_at,
    rotateEveryDays: a.rotate_every_days,
  }));
  const report = await analyze(items);
  const ts = Date.now();
  try {
    localStorage.setItem(CACHE_KEY, JSON.stringify({ report, ts }));
  } catch {
    /* ignore */
  }
  return report;
}
