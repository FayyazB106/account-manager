import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { FaEye, FaEyeSlash } from "react-icons/fa";
import { listPasswordHistory, revealPasswordHistory, type PasswordHistoryEntry } from "../api";
import { errorAlert } from "../alerts";
import { CopyText } from "./CopyText";
import { IconButton } from "./IconButton";
import { Pagination, usePagination } from "./Pagination";

// PasswordHistory shows an account's previous passwords (newest first), each
// reveal-on-click and click-to-copy. Renders nothing until there's history.
export function PasswordHistory({ accountId }: { accountId: string }) {
  const { t, i18n } = useTranslation();
  const [entries, setEntries] = useState<PasswordHistoryEntry[]>([]);
  const [loaded, setLoaded] = useState(false);
  const pager = usePagination(entries, accountId);

  useEffect(() => {
    let alive = true;
    listPasswordHistory(accountId)
      .then((e) => { if (alive) setEntries(e); })
      .catch(() => {})
      .finally(() => { if (alive) setLoaded(true); });
    return () => { alive = false; };
  }, [accountId]);

  if (!loaded) return null;

  if (entries.length === 0) {
    return (
      <div className="card">
        <h2 className="section-title">{t("history.title")}</h2>
        <p className="muted" style={{ marginBottom: 0 }}>{t("history.none")}</p>
      </div>
    );
  }

  return (
    <div className="card">
      <h2 className="section-title">{t("history.title")}</h2>
      <p className="muted" style={{ marginTop: 0 }}>{t("history.desc")}</p>
      <ul className="pw-history-list">
        {pager.items.map((e) => (
          <li key={e.id}>
            <span dir="ltr" className="muted pw-history-when">{new Date(e.changed_at).toLocaleString(i18n.language)}</span>
            <HistoryReveal accountId={accountId} historyId={e.id} />
          </li>
        ))}
      </ul>
      <Pagination p={pager} />
    </div>
  );
}

function HistoryReveal({ accountId, historyId }: { accountId: string; historyId: string }) {
  const { t } = useTranslation();
  const [revealed, setRevealed] = useState<string | null>(null);

  async function toggle() {
    if (revealed) { setRevealed(null); return; }
    try {
      setRevealed(await revealPasswordHistory(accountId, historyId));
    } catch (err) {
      errorAlert((err as Error).message);
    }
  }

  return (
    <span className="secret">
      <IconButton title={revealed ? t("common.hide") : t("common.reveal")} onClick={toggle}>
        {revealed ? <FaEyeSlash /> : <FaEye />}
      </IconButton>
      {revealed ? <CopyText value={revealed} mono /> : <span className="muted">••••••••</span>}
    </span>
  );
}
