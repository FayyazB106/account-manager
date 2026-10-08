// health.ts — Vault Health analysis, done CLIENT-SIDE after decryption.
//
// Two checks:
//   * weak    — realistic strength via the shared zxcvbn-based estimator
//   * reused  — the same password on more than one account
//
// Reuse is detected by hashing each password with SHA-256 (Web Crypto) and
// grouping equal hashes. We compare HASHES, never sending plaintext anywhere new
// and never storing plaintext — the same principle the backend uses.

import { passwordStrength, type StrengthLabel } from "./strength";
import { pwnedCount } from "./pwned";

// SHA-256 hex digest of a string, via the browser's Web Crypto API.
async function sha256Hex(text: string): Promise<string> {
  const bytes = new TextEncoder().encode(text);
  const digest = await crypto.subtle.digest("SHA-256", bytes);
  return [...new Uint8Array(digest)].map((b) => b.toString(16).padStart(2, "0")).join("");
}

export interface HealthEntry {
  id: string;
  service: string;
  crackSeconds: number;
  label: StrengthLabel;
  reused: boolean;
  rotateDue: boolean;
  pwned: number; // times seen in breaches; 0 = safe, -1 = check failed
}

export interface HealthReport {
  entries: HealthEntry[];
  weakCount: number;
  reusedCount: number;
  oldCount: number;
  pwnedCount: number;
  score: number; // 0..100
}

export interface HealthInput {
  id: string;
  service: string;
  password: string;
  changedAt?: string;   // ISO timestamp
  rotateEveryDays?: number;
}

// Is the password past its rotation policy date?
function isRotationDue(changedAt?: string, everyDays?: number): boolean {
  if (!changedAt || !everyDays) return false;
  const due = new Date(changedAt);
  due.setDate(due.getDate() + everyDays);
  return due.getTime() < Date.now();
}

// analyze takes the revealed passwords keyed by account and returns a report.
export async function analyze(items: HealthInput[]): Promise<HealthReport> {
  // Group account ids by password hash to find reuse.
  const byHash = new Map<string, string[]>();
  const hashes = new Map<string, string>();
  for (const it of items) {
    const h = await sha256Hex(it.password);
    hashes.set(it.id, h);
    byHash.set(h, [...(byHash.get(h) ?? []), it.id]);
  }

  const entries: HealthEntry[] = await Promise.all(
    items.map(async (it) => {
      const s = passwordStrength(it.password);
      const reused = (byHash.get(hashes.get(it.id)!)?.length ?? 1) > 1;
      const rotateDue = isRotationDue(it.changedAt, it.rotateEveryDays);
      const pwned = await pwnedCount(it.password);
      return { id: it.id, service: it.service, crackSeconds: s.crackSeconds, label: s.label, reused, rotateDue, pwned };
    })
  );

  const weakCount = entries.filter((e) => e.label === "weak").length;
  const reusedCount = entries.filter((e) => e.reused).length;
  const oldCount = entries.filter((e) => e.rotateDue).length;
  const breached = entries.filter((e) => e.pwned > 0).length;

  // Score: start at 100, penalize weak, reused, overdue, and breached entries.
  // Breaches are the most serious, so they carry the biggest penalty.
  const n = entries.length || 1;
  const penalty = (weakCount * 25 + reusedCount * 20 + oldCount * 15 + breached * 35) / n;
  const score = Math.max(0, Math.round(100 - penalty));

  return { entries, weakCount, reusedCount, oldCount, pwnedCount: breached, score };
}
