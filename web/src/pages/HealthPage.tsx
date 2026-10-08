import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router-dom";
import { AiOutlineSecurityScan } from "react-icons/ai";
import { FaFileExcel } from "react-icons/fa";
import { CircleButton } from "../components/CircleButton";
import { Badge } from "../components/Badge";
import { downloadSheet } from "./FinanceAccounts";
import { loadHealthCache, runHealthScan } from "../health-scan";
import { formatCrackTime } from "../strength";
import type { HealthEntry } from "../health";

function scoreBand(score: number): "weak" | "fair" | "good" {
  if (score < 50) return "weak";
  if (score < 80) return "fair";
  return "good";
}

// CountUp animates 0 → value once on mount (re-mount it via `key` to replay).
function CountUp({ value }: { value: number }) {
  const [n, setN] = useState(0);
  useEffect(() => {
    let raf = 0;
    const start = performance.now();
    const dur = 800;
    const tick = (now: number) => {
      const p = Math.min(1, (now - start) / dur);
      setN(Math.round((1 - (1 - p) * (1 - p)) * value)); // ease-out
      if (p < 1) raf = requestAnimationFrame(tick);
    };
    raf = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(raf);
  }, [value]);
  return <>{n}</>;
}

// Shimmering placeholder shown while a scan runs.
function HealthSkeleton() {
  return (
    <div className="health-skeleton" aria-hidden>
      <div className="hs-top">
        <span className="skeleton hs-score" />
        <div className="hs-summary">
          {Array.from({ length: 5 }, (_, i) => <span key={i} className="skeleton hs-pill" />)}
        </div>
      </div>
      <div className="hs-rows">
        {Array.from({ length: 8 }, (_, r) => (
          <div key={r} className="hs-row">
            {Array.from({ length: 6 }, (_, c) => <span key={c} className="skeleton" />)}
          </div>
        ))}
      </div>
    </div>
  );
}

type HKey = "service" | "strength" | "crack" | "reused" | "rotation" | "breached";
const strengthRank: Record<string, number> = { weak: 0, fair: 1, strong: 2, excellent: 3 };

function healthValue(e: HealthEntry, key: HKey): string | number {
  switch (key) {
    case "service": return e.service.toLowerCase();
    case "strength": return strengthRank[e.label] ?? 0;
    case "crack": return e.crackSeconds;
    case "reused": return e.reused ? 1 : 0;
    case "rotation": return e.rotateDue ? 1 : 0;
    case "breached": return e.pwned;
  }
}

export function HealthPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const cached = loadHealthCache();
  const [report, setReport] = useState(cached?.report ?? null);
  const [lastRun, setLastRun] = useState<number | null>(cached?.ts ?? null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [sortKey, setSortKey] = useState<HKey>("service");
  const [sortDir, setSortDir] = useState<"asc" | "desc">("asc");

  function toggleSort(key: HKey) {
    if (sortKey === key) setSortDir((d) => (d === "asc" ? "desc" : "asc"));
    else { setSortKey(key); setSortDir("asc"); }
  }
  function SortHead({ k, label }: { k: HKey; label: string }) {
    return (
      <th className="sortable" onClick={() => toggleSort(k)}>
        {label}{sortKey === k && <span className="sort-arrow">{sortDir === "asc" ? "▲" : "▼"}</span>}
      </th>
    );
  }
  const sortedEntries = report
    ? [...report.entries].sort((a, b) => {
        const va = healthValue(a, sortKey), vb = healthValue(b, sortKey);
        const dir = sortDir === "asc" ? 1 : -1;
        return va < vb ? -dir : va > vb ? dir : 0;
      })
    : [];

  // Export the audit table exactly as shown (current sort).
  function exportExcel() {
    const header = [
      t("health.colService"), t("health.colStrength"), t("health.colCrackTime"),
      t("health.colReused"), t("health.colRotation"), t("health.colBreached"),
    ];
    const data = sortedEntries.map((e) => [
      e.service,
      t(`strength.${e.label}`),
      formatCrackTime(t, e.crackSeconds),
      e.reused ? t("health.reusedBadge") : "—",
      e.rotateDue ? t("health.overdueBadge") : "—",
      e.pwned > 0 ? t("health.seenTimes", { count: e.pwned.toLocaleString() }) : e.pwned === 0 ? "—" : t("health.na"),
    ]);
    downloadSheet([header, ...data], t("health.title"), "vault-health.xlsx");
  }

  async function scan() {
    setBusy(true);
    setError(null);
    try {
      const rep = await runHealthScan();
      setReport(rep);
      setLastRun(Date.now());
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setBusy(false);
    }
  }

  return (
      <div className="card">
        <div className="topbar" style={{ marginBottom: 16 }}>
          <div>
            <h1>{t("health.title")}</h1>
            <p className="muted" style={{ margin: "4px 0 0" }}>
              {busy ? t("health.scanning") : lastRun ? t("health.lastScan", { when: new Date(lastRun).toLocaleString() }) : t("health.neverScanned")}
            </p>
          </div>
          <div style={{ display: "flex", gap: 10, alignItems: "center" }}>
            {report && <CircleButton variant="ok" title={t("export.exportBtn")} onClick={exportExcel}><FaFileExcel /></CircleButton>}
            <CircleButton onClick={scan} disabled={busy} title={t("health.scanVault")}>
              <AiOutlineSecurityScan />
            </CircleButton>
          </div>
        </div>

        {error && <p className="error">{error}</p>}

        {busy ? <HealthSkeleton /> : report ? (
          <>
            <div className="score-row">
              <div className={`score score-${scoreBand(report.score)}`}><CountUp key={lastRun ?? 0} value={report.score} /><span>/100</span></div>
              <div className="score-detail">
                <div><strong><CountUp key={lastRun ?? 0} value={report.weakCount} /></strong> {t("health.weakLabel")}</div>
                <div><strong><CountUp key={lastRun ?? 0} value={report.reusedCount} /></strong> {t("health.reusedLabel")}</div>
                <div><strong><CountUp key={lastRun ?? 0} value={report.oldCount} /></strong> {t("health.dueToRotate")}</div>
                <div><strong className={report.pwnedCount > 0 ? "danger-text" : ""}><CountUp key={lastRun ?? 0} value={report.pwnedCount} /></strong> {t("health.breachedLabel")}</div>
                <div><strong><CountUp key={lastRun ?? 0} value={report.entries.length} /></strong> {t("health.total")}</div>
              </div>
            </div>

            <div className="table-scroll">
            <table className="accounts health-report">
              <thead>
                <tr>
                  <SortHead k="service" label={t("health.colService")} />
                  <SortHead k="strength" label={t("health.colStrength")} />
                  <SortHead k="crack" label={t("health.colCrackTime")} />
                  <SortHead k="reused" label={t("health.colReused")} />
                  <SortHead k="rotation" label={t("health.colRotation")} />
                  <SortHead k="breached" label={t("health.colBreached")} />
                </tr>
              </thead>
              <tbody>
                {sortedEntries.map((e) => (
                  <tr key={e.id} className="clickable-row" onClick={() => navigate(`/account/${e.id}`)}>
                    <td>{e.service}</td>
                    <td><Badge tone={e.label}>{t(`strength.${e.label}`)}</Badge></td>
                    <td>~{formatCrackTime(t, e.crackSeconds)}</td>
                    <td>{e.reused ? <Badge tone="weak">{t("health.reusedBadge")}</Badge> : "—"}</td>
                    <td>{e.rotateDue ? <Badge tone="fair">{t("health.overdueBadge")}</Badge> : "—"}</td>
                    <td>
                      {e.pwned > 0
                        ? <Badge tone="weak">{t("health.seenTimes", { count: e.pwned.toLocaleString() })}</Badge>
                        : e.pwned === 0 ? "—" : <span className="muted">{t("health.na")}</span>}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
            </div>
            <p className="muted">
              {t("health.analyzedNotePre")}{" "}
              <a href="https://haveibeenpwned.com" target="_blank" rel="noreferrer">Have I Been Pwned</a>{" "}
              {t("health.analyzedNotePost")}
            </p>
          </>
        ) : (
          !busy && <p className="muted">{t("health.emptyHint")}</p>
        )}
      </div>
  );
}
