import { useCallback, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { FaPlus, FaEdit, FaTrash, FaFileInvoiceDollar } from "react-icons/fa";
import {
  listAccountPayments,
  deletePayment,
  downloadAttachment,
  type Payment,
} from "../api";
import { useLayout } from "./Layout";
import { confirmDialog, errorAlert } from "../alerts";
import { fmtMoney, amountVariants, toUSD } from "../money";
import { CircleButton } from "./CircleButton";
import { IconButton } from "./IconButton";
import { PaymentForm } from "./PaymentForm";
import { Pagination, usePagination } from "./Pagination";

// PaymentHistory is the per-account ledger section: a table of recorded payments
// (auto-logged on renewal + manual entries) with a total, plus add/edit/delete.
export function PaymentHistory({ accountId, reloadKey = 0 }: { accountId: string; reloadKey?: number }) {
  const { t } = useTranslation();
  const { currencies } = useLayout();
  const [payments, setPayments] = useState<Payment[]>([]);
  const [adding, setAdding] = useState(false);
  const [editing, setEditing] = useState<Payment | null>(null);
  const pager = usePagination(payments, accountId);

  const refresh = useCallback(() => {
    listAccountPayments(accountId).then(setPayments).catch(() => {});
  }, [accountId]);

  // reloadKey lets the parent force a refetch (e.g. after "Mark as paid" auto-logs
  // a payment) even though accountId hasn't changed.
  useEffect(() => { refresh(); }, [refresh, reloadKey]);

  async function handleDelete(p: Payment) {
    const ok = await confirmDialog({
      title: t("payments.deleteTitle"),
      text: t("payments.deleteText"),
      confirmText: t("common.delete"),
      danger: true,
    });
    if (!ok) return;
    try {
      await deletePayment(p.id);
      refresh();
    } catch (err) {
      errorAlert((err as Error).message);
    }
  }

  const totalUSD = payments.reduce((sum, p) => sum + toUSD(p.amount, p.currency, currencies), 0);

  return (
    <div className="card">
      <div className="att-head">
        <strong>{t("payments.title")}</strong>
        <CircleButton title={t("payments.addTitle")} onClick={() => setAdding(true)}><FaPlus size={15} /></CircleButton>
      </div>

      {payments.length === 0 ? (
        <p className="muted">{t("payments.none")}</p>
      ) : (
        <div className="table-scroll">
          <table className="accounts payments-table">
            <thead>
              <tr>
                <th>{t("payments.paidOn")}</th>
                <th>{t("payments.amount")}</th>
                <th>{t("payments.invoice")}</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              {pager.items.map((p) => (
                <tr key={p.id}>
                  <td className="nowrap"><span dir="ltr">{p.paid_on.slice(0, 10)}</span></td>
                  <td className="nowrap"><span dir="ltr">{fmtMoney(p.currency, p.amount)}</span></td>
                  <td className="wrap-cell">
                    {p.attachment_id && p.attachment_name ? (
                      <button
                        className="link"
                        onClick={() => downloadAttachment(p.attachment_id!, p.attachment_name!).catch((e) => errorAlert((e as Error).message))}
                      >
                        <FaFileInvoiceDollar style={{ verticalAlign: "-2px", marginInlineEnd: 4 }} />{p.attachment_name}
                      </button>
                    ) : "—"}
                  </td>
                  <td className="nowrap" style={{ textAlign: "end" }}>
                    <span className="att-actions">
                      <IconButton title={t("common.edit")} onClick={() => setEditing(p)}><FaEdit /></IconButton>
                      <IconButton danger title={t("common.delete")} onClick={() => handleDelete(p)}><FaTrash /></IconButton>
                    </span>
                  </td>
                </tr>
              ))}
            </tbody>
            <tfoot>
              <tr className="total-row">
                <td>{t("payments.total")}</td>
                <td colSpan={3}><strong dir="ltr">{amountVariants(totalUSD, currencies)}</strong></td>
              </tr>
            </tfoot>
          </table>
        </div>
      )}
      <Pagination p={pager} />

      {/* Notes, shown beneath the table so the rows stay scannable. */}
      {payments.some((p) => p.note) && (
        <ul className="pay-notes">
          {payments.filter((p) => p.note).map((p) => (
            <li key={p.id}>
              <span className="muted" dir="ltr">{p.paid_on.slice(0, 10)}</span> — {p.note}
            </li>
          ))}
        </ul>
      )}

      {adding && (
        <PaymentForm
          accountId={accountId}
          existing={null}
          onSaved={() => { setAdding(false); refresh(); }}
          onCancel={() => setAdding(false)}
        />
      )}
      {editing && (
        <PaymentForm
          accountId={accountId}
          existing={editing}
          onSaved={() => { setEditing(null); refresh(); }}
          onCancel={() => setEditing(null)}
        />
      )}
    </div>
  );
}
