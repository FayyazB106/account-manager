import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Modal } from "./Modal";
import { Button } from "./Button";

// The options, in the order shown: 7 days before ... 1 day before, then the due day (0).
const OPTIONS = [7, 6, 5, 4, 3, 2, 1, 0];

// ReminderDaysModal lets an admin choose which days a reminder email is sent on.
export function ReminderDaysModal({
  title,
  days,
  onSave,
  onCancel,
}: {
  title: string;
  days: number[];
  onSave: (days: number[]) => Promise<void>;
  onCancel: () => void;
}) {
  const { t } = useTranslation();
  const [picked, setPicked] = useState<Set<number>>(new Set(days));
  const [busy, setBusy] = useState(false);

  const label = (d: number) => (d === 0 ? t("report.lastDay") : t("report.daysBefore", { count: d }));

  function toggle(d: number) {
    setPicked((prev) => {
      const next = new Set(prev);
      if (next.has(d)) next.delete(d); else next.add(d);
      return next;
    });
  }

  async function save() {
    setBusy(true);
    try { await onSave(OPTIONS.filter((d) => picked.has(d))); }
    finally { setBusy(false); }
  }

  return (
    <Modal wide onClose={onCancel}>
      <div className="add-form">
        <h2 className="section-title" style={{ marginBottom: 0 }}>{title}</h2>
        <p style={{ margin: "4px 0 0" }}>{t("report.daysTitle")}</p>
        <p className="field-hint" style={{ margin: 0 }}>{t("report.daysHint")}</p>

        <div className="day-grid">
          {OPTIONS.map((d) => (
            <label key={d} className="day-opt">
              <span>{label(d)}</span>
              <input type="checkbox" className="round-check" checked={picked.has(d)} onChange={() => toggle(d)} />
            </label>
          ))}
        </div>
        {picked.size === 0 && <p className="error" style={{ margin: 0 }}>{t("report.daysNeedOne")}</p>}

        <div className="form-actions">
          <Button onClick={save} disabled={busy || picked.size === 0}>{t("common.save")}</Button>
          <Button variant="ghost" onClick={onCancel} disabled={busy}>{t("common.cancel")}</Button>
        </div>
      </div>
    </Modal>
  );
}
