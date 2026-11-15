import { useRef, useState, type DragEvent } from "react";
import { useTranslation } from "react-i18next";
import { FaCloudUploadAlt } from "react-icons/fa";
import { Modal } from "./Modal";
import { Button } from "./Button";
import { readPaymentsFile, type PaymentRow, type RowError } from "../paymentsImport";

// ImportHistoryModal explains how to lay out a payment-history file (date, amount, optional
// currency and note), takes it by drag and drop or a file picker, shows what was found and
// which rows can't be read, and only imports when you press Import.
export function ImportHistoryModal({
  accountCurrency,
  onImport,
  onClose,
}: {
  accountCurrency: string;
  onImport: (rows: PaymentRow[]) => Promise<void>;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const inputRef = useRef<HTMLInputElement>(null);
  const [over, setOver] = useState(false);
  const [fileName, setFileName] = useState("");
  const [rows, setRows] = useState<PaymentRow[]>([]);
  const [rowErrors, setRowErrors] = useState<RowError[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function load(file: File | undefined) {
    if (!file) return;
    setError(null);
    setRows([]);
    setRowErrors([]);
    setFileName(file.name);
    try {
      const res = await readPaymentsFile(file);
      setRows(res.rows);
      setRowErrors(res.errors);
      if (res.rows.length === 0) setError(t("payments.importNothing"));
    } catch (e) {
      setError((e as Error).message);
    }
  }

  function onDrop(e: DragEvent) {
    e.preventDefault();
    setOver(false);
    load(e.dataTransfer.files?.[0]);
  }

  async function submit() {
    setBusy(true);
    setError(null);
    try { await onImport(rows); }
    catch (e) { setError((e as Error).message); setBusy(false); }
  }

  return (
    <Modal wide onClose={onClose}>
      <div className="add-form">
        <h2 className="section-title" style={{ marginBottom: 0 }}>{t("payments.importTitle")}</h2>
        <p className="muted" style={{ margin: "4px 0 0" }}>{t("payments.importIntro")}</p>

        {/* What the file should look like. */}
        <table className="import-sample" dir="ltr" aria-label={t("payments.importExample")}>
          <thead>
            <tr><th></th><th>A</th><th>B</th><th>C</th><th>D</th></tr>
          </thead>
          <tbody>
            <tr><th>1</th><td>2025-01-15</td><td>49.99</td><td>USD</td><td>January renewal</td></tr>
            <tr><th>2</th><td>2025-02-15</td><td>49.99</td><td></td><td></td></tr>
          </tbody>
        </table>
        <ul className="import-rules muted">
          <li>{t("payments.importColA")}</li>
          <li>{t("payments.importColB")}</li>
          <li>{t("payments.importColC", { currency: accountCurrency })}</li>
          <li>{t("payments.importColD")}</li>
          <li>{t("payments.importNoHeader")}</li>
        </ul>

        <div
          className={`import-drop ${over ? "over" : ""}`.trim()}
          role="button"
          tabIndex={0}
          onClick={() => inputRef.current?.click()}
          onKeyDown={(e) => { if (e.key === "Enter" || e.key === " ") { e.preventDefault(); inputRef.current?.click(); } }}
          onDragOver={(e) => { e.preventDefault(); setOver(true); }}
          onDragLeave={() => setOver(false)}
          onDrop={onDrop}
        >
          <FaCloudUploadAlt size={28} />
          {fileName ? (
            <>
              <strong>{fileName}</strong>
              {rows.length > 0 && <span>{t("payments.importFound", { count: rows.length })}</span>}
              <span className="muted">{t("users.importChange")}</span>
            </>
          ) : (
            <>
              <strong>{t("users.importDrop")}</strong>
              <span className="muted">{t("users.importTypes")}</span>
            </>
          )}
          <input ref={inputRef} type="file" accept=".xlsx,.xls,.csv" hidden onChange={(e) => { load(e.target.files?.[0]); e.target.value = ""; }} />
        </div>

        {rowErrors.length > 0 && (
          <div className="import-preview">
            <span className="danger-text">{t("payments.importSkipped", { count: rowErrors.length })}</span>
            <ul className="import-rules muted" style={{ margin: "4px 0 0" }}>
              {rowErrors.slice(0, 3).map((er) => (
                <li key={er.row}>{t("payments.importRowError", { row: er.row, reason: t(`payments.importReason_${er.reason}`) })}</li>
              ))}
              {rowErrors.length > 3 && <li>…</li>}
            </ul>
          </div>
        )}
        {error && <p className="error">{error}</p>}

        <div className="form-actions">
          <Button onClick={submit} disabled={busy || rows.length === 0}>{busy ? t("users.adding") : t("payments.importAction", { count: rows.length })}</Button>
          <Button variant="ghost" onClick={onClose} disabled={busy}>{t("common.cancel")}</Button>
        </div>
      </div>
    </Modal>
  );
}
