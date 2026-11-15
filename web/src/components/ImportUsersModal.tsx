import { useRef, useState, type DragEvent } from "react";
import { useTranslation } from "react-i18next";
import { FaCloudUploadAlt } from "react-icons/fa";
import { Modal } from "./Modal";
import { Button } from "./Button";
import { readPeopleFile, type ImportRow } from "../peopleImport";

// ImportUsersModal explains how to lay out the file (English name in column A, Arabic name in
// column B when Arabic support is on), takes it by drag and drop or a file picker, shows what
// was found, and only imports when you press Import.
export function ImportUsersModal({
  arabicEnabled,
  onImport,
  onClose,
}: {
  arabicEnabled: boolean;
  onImport: (rows: ImportRow[]) => Promise<void>;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const inputRef = useRef<HTMLInputElement>(null);
  const [over, setOver] = useState(false);
  const [fileName, setFileName] = useState("");
  const [rows, setRows] = useState<ImportRow[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function load(file: File | undefined) {
    if (!file) return;
    setError(null);
    setRows([]);
    setFileName(file.name);
    try {
      const found = await readPeopleFile(file, arabicEnabled);
      if (found.length === 0) setError(t("users.noNamesFound"));
      setRows(found);
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
        <h2 className="section-title" style={{ marginBottom: 0 }}>{t("users.importTitle")}</h2>
        <p className="muted" style={{ margin: "4px 0 0" }}>
          {arabicEnabled ? t("users.importIntroAr") : t("users.importIntro")}
        </p>

        {/* What the file should look like. */}
        <table className="import-sample" dir="ltr" aria-label={t("users.importExample")}>
          <thead>
            <tr>
              <th></th>
              <th>A</th>
              {arabicEnabled && <th>B</th>}
            </tr>
          </thead>
          <tbody>
            <tr>
              <th>1</th>
              <td>Lewis Hamilton</td>
              {arabicEnabled && <td dir="rtl">لويس هاميلتون</td>}
            </tr>
            <tr>
              <th>2</th>
              <td>Muhammad Ali</td>
              {arabicEnabled && <td dir="rtl">محمد علي</td>}
            </tr>
          </tbody>
        </table>
        <ul className="import-rules muted">
          <li>{t("users.importColA")}</li>
          {arabicEnabled && <li>{t("users.importColB")}</li>}
          <li>{t("users.importNoHeader")}</li>
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
              {rows.length > 0 && <span>{t("users.importFound", { count: rows.length })}</span>}
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

        {rows.length > 0 && (
          <p className="muted import-preview">
            {rows.slice(0, 3).map((r) => (arabicEnabled && r.name_ar ? `${r.name} · ${r.name_ar}` : r.name)).join("  |  ")}
            {rows.length > 3 ? "  …" : ""}
          </p>
        )}
        {error && <p className="error">{error}</p>}

        <div className="form-actions">
          <Button onClick={submit} disabled={busy || rows.length === 0}>{busy ? t("users.adding") : t("users.importAction", { count: rows.length })}</Button>
          <Button variant="ghost" onClick={onClose} disabled={busy}>{t("common.cancel")}</Button>
        </div>
      </div>
    </Modal>
  );
}
