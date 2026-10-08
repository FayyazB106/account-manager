import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { listAudit, type AuditEntry } from "../api";
import { toast } from "../alerts";
import { Button } from "../components/Button";

const PAGE = 100;

// Top-level action groups (the backend filters by action prefix).
const GROUPS = ["auth", "account", "attachment", "admin", "profile", "mfa", "category", "person", "settings", "backup", "vault", "breach", "rates", "system"];

// A compact, sortable timestamp for the log lines (local time).
function fmtTime(iso: string): string {
  const d = new Date(iso);
  const p = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`;
}

// One plain, copiable line: timestamp, actor, action, target, ip, result, and
// an optional detail (e.g. "via 2FA", "scheduled / 15 checked / 0 skipped").
function toLine(e: AuditEntry): string {
  const cols = [fmtTime(e.at), e.actor_email || "-", e.action, e.target || "-", e.ip || "-", e.result];
  if (e.detail) cols.push(e.detail);
  return cols.join(", ");
}

export function AuditSettings() {
  const { t } = useTranslation();
  const [entries, setEntries] = useState<AuditEntry[]>([]);
  const [total, setTotal] = useState(0);
  const [actor, setActor] = useState("");
  const [group, setGroup] = useState("");
  const [offset, setOffset] = useState(0);
  const [loading, setLoading] = useState(true);
  const logRef = useRef<HTMLPreElement>(null);

  useEffect(() => {
    let alive = true;
    setLoading(true);
    listAudit({ actor: actor.trim(), action: group, limit: PAGE, offset })
      .then((r) => { if (alive) { setEntries(r.entries); setTotal(r.total); } })
      .catch(() => {})
      .finally(() => { if (alive) setLoading(false); });
    return () => { alive = false; };
  }, [actor, group, offset]);

  // API returns newest-first; show oldest-first so the latest is at the bottom.
  const ordered = [...entries].reverse();
  const text = ordered.map(toLine).join("\n");

  // Keep the newest line in view when the page (re)loads.
  useEffect(() => {
    if (logRef.current) logRef.current.scrollTop = logRef.current.scrollHeight;
  }, [text]);

  function changeActor(v: string) { setActor(v); setOffset(0); }
  function changeGroup(v: string) { setGroup(v); setOffset(0); }

  // Export every matching entry (across all pages, oldest-first) as one .txt.
  // The backend caps a page at 500, so fetch in chunks until we have them all.
  async function exportAll() {
    try {
      const CHUNK = 500;
      const collected: AuditEntry[] = [];
      let off = 0;
      let count = Infinity;
      while (collected.length < count) {
        const r = await listAudit({ actor: actor.trim(), action: group, limit: CHUNK, offset: off });
        count = r.total;
        collected.push(...r.entries);
        if (r.entries.length < CHUNK) break;
        off += CHUNK;
      }
      // API pages are newest-first; reverse the whole set to oldest-first.
      const all = collected.reverse().map(toLine).join("\n");
      const blob = new Blob([all], { type: "text/plain;charset=utf-8" });
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      const stamp = fmtTime(new Date().toISOString()).replace(/[: ]/g, "-");
      a.href = url;
      a.download = `audit-log-${stamp}.txt`;
      a.click();
      URL.revokeObjectURL(url);
    } catch { toast(t("common.error")); }
  }

  const from = total === 0 ? 0 : offset + 1;
  const to = Math.min(offset + PAGE, total);

  return (
    <div className="card">
      <div className="topbar" style={{ marginBottom: 12 }}>
        <h2 className="section-title" style={{ margin: 0 }}>{t("audit.title")}</h2>
        <Button variant="ghost" onClick={exportAll} disabled={total === 0}>{t("audit.export")}</Button>
      </div>

      <div className="audit-filters">
        <input placeholder={t("audit.filterActor")} value={actor} onChange={(e) => changeActor(e.target.value)} dir="ltr" />
        <select dir="ltr" value={group} onChange={(e) => changeGroup(e.target.value)}>
          <option value="">{t("audit.allActions")}</option>
          {GROUPS.map((g) => <option key={g} value={g}>{t(`audit.group.${g}`)}</option>)}
        </select>
      </div>

      <pre className="audit-terminal" ref={logRef} dir="ltr">{loading && entries.length === 0 ? t("common.loading") : text || t("audit.empty")}</pre>

      <div className="audit-pager">
        <span className="muted">{t("audit.showing", { from, to, total })}</span>
        <div>
          <Button variant="ghost" disabled={to >= total} onClick={() => setOffset(offset + PAGE)}>{t("audit.older")}</Button>
          <Button variant="ghost" disabled={offset === 0} onClick={() => setOffset(Math.max(0, offset - PAGE))}>{t("audit.newer")}</Button>
        </div>
      </div>
    </div>
  );
}
