import { useEffect, useRef, useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { FaUpload } from "react-icons/fa";
import {
  addPayment,
  updatePayment,
  extendAccount,
  listAttachments,
  uploadAttachment,
  type Payment,
  type PaymentInput,
  type AttachmentMeta,
} from "../api";
import { useLayout } from "./Layout";
import { Modal } from "./Modal";
import { Field } from "./Field";
import { Button } from "./Button";
import { CircleButton } from "./CircleButton";

// PaymentForm is the add/edit modal for a single ledger payment. On save it calls
// onSaved(); the parent reloads the list.
//
// `markPaid` mode (from the account's "Mark as paid" button) opens the form
// prefilled from `defaults` so you can review/adjust, and on save ALSO advances
// the subscription's next-payment date.
export function PaymentForm({
  accountId,
  existing,
  markPaid = false,
  defaults,
  onSaved,
  onCancel,
}: {
  accountId: string;
  existing: Payment | null;
  markPaid?: boolean;
  defaults?: { amount?: number | null; currency?: string };
  onSaved: () => void;
  onCancel: () => void;
}) {
  const editing = existing !== null;
  const { t } = useTranslation();
  const { currencies } = useLayout();
  const activeCcys = currencies.filter((c) => c.active && (c.code === "USD" || c.rate > 0));

  const today = new Date().toISOString().slice(0, 10);
  const [paidOn, setPaidOn] = useState(existing ? existing.paid_on.slice(0, 10) : today);
  const [amount, setAmount] = useState(
    existing ? String(existing.amount) : defaults?.amount != null ? String(defaults.amount) : "",
  );
  const [currency, setCurrency] = useState(existing?.currency ?? defaults?.currency ?? "USD");
  const [note, setNote] = useState(existing?.note ?? "");
  const [attachmentId, setAttachmentId] = useState(existing?.attachment_id ?? "");
  const [attachments, setAttachments] = useState<AttachmentMeta[]>([]);
  const [uploading, setUploading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const fileRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    listAttachments(accountId).then(setAttachments).catch(() => {});
  }, [accountId]);

  // Upload a new invoice right here (for when there's nothing existing to link),
  // then select it.
  async function handleUpload(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0];
    if (!file) return;
    setUploading(true);
    setError(null);
    try {
      const res = await uploadAttachment(accountId, file);
      setAttachments(await listAttachments(accountId));
      setAttachmentId(res.id);
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setUploading(false);
      if (fileRef.current) fileRef.current.value = "";
    }
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    const amt = Number(amount);
    if (!amount || Number.isNaN(amt)) {
      setError(t("payments.amountRequired"));
      return;
    }
    const payload: PaymentInput = { paid_on: paidOn, amount: amt, currency };
    if (note.trim()) payload.note = note.trim();
    if (attachmentId) payload.attachment_id = attachmentId;
    try {
      if (editing) {
        await updatePayment(existing!.id, payload);
      } else {
        await addPayment(accountId, payload);
        // "Mark as paid": also roll the subscription's next-payment date forward.
        if (markPaid) await extendAccount(accountId);
      }
      onSaved();
    } catch (err) {
      setError((err as Error).message);
    }
  }

  return (
    <Modal onClose={onCancel}>
      <form className="add-form" onSubmit={handleSubmit}>
        <h1 style={{ fontSize: 16 }}>{editing ? t("payments.editTitle") : markPaid ? t("accounts.markPaid") : t("payments.addTitle")}</h1>
        <div className="grid">
          <Field label={t("payments.paidOn")}>
            <input type="date" value={paidOn} onChange={(e) => setPaidOn(e.target.value)} required />
          </Field>
          <Field label={t("payments.amountCurrency")}>
            <div className="amount-row">
              <select dir="ltr" aria-label={t("payments.currency")} value={currency} onChange={(e) => setCurrency(e.target.value)}>
                {activeCcys.map((c) => (
                  <option key={c.code} value={c.code}>{c.code}</option>
                ))}
                {/* Keep an out-of-list currency (e.g. one later deactivated) selectable. */}
                {!activeCcys.some((c) => c.code === currency) && <option value={currency}>{currency}</option>}
              </select>
              <input
                type="number"
                min={0}
                step="0.01"
                value={amount}
                onChange={(e) => setAmount(e.target.value)}
                required
              />
            </div>
          </Field>
          <Field label={t("payments.invoice")} full>
            <div className="invoice-row">
              <select value={attachmentId} onChange={(e) => setAttachmentId(e.target.value)}>
                <option value="">{t("payments.noInvoice")}</option>
                {attachments.map((a) => (
                  <option key={a.id} value={a.id}>{a.filename}</option>
                ))}
              </select>
              <CircleButton as="label" title={t("payments.uploadInvoice")} disabled={uploading}>
                <FaUpload size={15} />
                <input ref={fileRef} type="file" hidden onChange={handleUpload} disabled={uploading} />
              </CircleButton>
            </div>
          </Field>
          <Field label={t("payments.note")} full>
            <textarea value={note} onChange={(e) => setNote(e.target.value)} rows={2} placeholder={t("payments.notePlaceholder")} />
          </Field>
        </div>
        {error && <p className="error">{error}</p>}
        <div className="form-actions">
          <Button type="submit" variant={markPaid ? "ok" : undefined}>{editing ? t("accounts.saveChanges") : markPaid ? t("accounts.markPaid") : t("payments.save")}</Button>
          <Button type="button" variant="ghost" onClick={onCancel}>{t("common.cancel")}</Button>
        </div>
      </form>
    </Modal>
  );
}
