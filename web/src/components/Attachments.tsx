import { useEffect, useRef, useState } from "react";
import {
  listAttachments,
  uploadAttachment,
  downloadAttachment,
  deleteAttachment,
  exportAttachments,
  type AttachmentMeta,
} from "../api";
import { useTranslation } from "react-i18next";
import { confirmDialog, errorAlert } from "../alerts";
import { FaDownload, FaTrash, FaUpload } from "react-icons/fa";
import { CircleButton } from "./CircleButton";
import { IconButton } from "./IconButton";
import { Button } from "./Button";

export function Attachments({ accountId }: { accountId: string }) {
  const { t } = useTranslation();
  const [files, setFiles] = useState<AttachmentMeta[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  async function refresh() {
    try {
      setFiles(await listAttachments(accountId));
    } catch (err) {
      setError((err as Error).message);
    }
  }

  useEffect(() => {
    refresh();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [accountId]);

  async function handleUpload(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0];
    if (!file) return;
    setBusy(true);
    setError(null);
    try {
      await uploadAttachment(accountId, file);
      if (inputRef.current) inputRef.current.value = "";
      await refresh();
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setBusy(false);
    }
  }

  async function handleDelete(f: AttachmentMeta) {
    const ok = await confirmDialog({
      title: t("attachments.deleteTitle", { name: f.filename }),
      text: t("attachments.deleteText"),
      confirmText: t("common.delete"),
      danger: true,
    });
    if (!ok) return;
    try {
      await deleteAttachment(f.id);
      await refresh();
    } catch (err) {
      setError((err as Error).message);
    }
  }

  return (
    <div className="attachments">
      <div className="att-head">
        <strong>{t("attachments.title")}</strong>
        <span style={{ display: "flex", gap: 8, alignItems: "center" }}>
          {files.length > 0 && (
            <Button variant="ghost" onClick={() => exportAttachments(accountId).catch((e) => errorAlert((e as Error).message))}>
              {t("attachments.exportAll")}
            </Button>
          )}
          <CircleButton as="label" title={t("attachments.uploadFile")} disabled={busy}>
            <FaUpload size={16} />
            <input ref={inputRef} type="file" onChange={handleUpload} hidden disabled={busy} />
          </CircleButton>
        </span>
      </div>

      {error && <p className="error">{error}</p>}

      {files.length === 0 ? (
        <p className="muted">{t("attachments.noFiles")}</p>
      ) : (
        <ul className="att-list">
          {files.map((f) => (
            <li key={f.id}>
              <span>{f.filename}</span>
              <span className="att-actions">
                <IconButton title={t("attachments.download")} onClick={() => downloadAttachment(f.id, f.filename)}><FaDownload /></IconButton>
                <IconButton danger title={t("common.delete")} onClick={() => handleDelete(f)}><FaTrash /></IconButton>
              </span>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
