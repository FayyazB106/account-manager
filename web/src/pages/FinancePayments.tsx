import { useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import { useTranslation } from "react-i18next";
import * as XLSX from "xlsx";
import JSZip from "jszip";
import { useLayout } from "../components/Layout";
import { localCategoryName } from "../categoryName";
import { billingLabel } from "../account-ui";
import { fmtMoney, toUSD } from "../money";
import { downloadSheet } from "./FinanceAccounts";
import { errorAlert } from "../alerts";
import { Button } from "../components/Button";
import { CircleButton } from "../components/CircleButton";
import { Icon } from "../icons";
import { FaFileExcel, FaFileArchive } from "react-icons/fa";
import { listAllPayments, downloadAttachment, fetchAttachmentBlob, type PaymentListItem } from "../api";

type SortKey = "paid_on" | "service" | "amount";

export function FinancePayments() {
  const { currencies, lang, categories } = useLayout();
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [payments, setPayments] = useState<PaymentListItem[]>([]);
  const [ccy, setCcy] = useState("USD");
  const [sortKey, setSortKey] = useState<SortKey>("paid_on");
  const [sortDir, setSortDir] = useState<"asc" | "desc">("desc");
  const [catFilter, setCatFilter] = useState("all");
  const [billingFilter, setBillingFilter] = useState("all");
  const [userFilter, setUserFilter] = useState("all");
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    listAllPayments().then(setPayments).catch(() => {});
  }, []);

  const seatNames = (p: PaymentListItem) =>
    lang === "ar" ? p.seats.map((s, i) => (p.seats_ar?.[i] ? p.seats_ar[i]! : s)) : p.seats;

  // Filter options derived from all payments so each dropdown offers every value.
  const catOptions = useMemo(() => {
    const m = new Map<string, string>();
    for (const p of payments) if (p.category_id) m.set(p.category_id, localCategoryName(categories, p.category_id, p.category_name || p.category_id, lang));
    return [...m.entries()].map(([id, name]) => ({ id, name })).sort((x, y) => x.name.localeCompare(y.name));
  }, [payments, categories, lang]);
  const billingOptions = useMemo(() => {
    const s = new Set<number>();
    for (const p of payments) s.add(p.billing_period_months);
    return [...s].sort((x, y) => x - y);
  }, [payments]);
  const userOptions = useMemo(() => {
    const m = new Map<string, string>();
    for (const p of payments)
      p.seats.forEach((s, i) => {
        const en = s.trim();
        if (!en) return;
        const loc = lang === "ar" && p.seats_ar?.[i] ? p.seats_ar[i]! : s;
        if (!m.has(en)) m.set(en, loc);
      });
    return [...m.entries()].map(([en, loc]) => ({ en, loc })).sort((x, y) => x.loc.localeCompare(y.loc));
  }, [payments, lang]);

  const hasFilter =
    catFilter !== "all" || billingFilter !== "all" || userFilter !== "all" || from !== "" || to !== "";
  function clearFilters() {
    setCatFilter("all"); setBillingFilter("all"); setUserFilter("all"); setFrom(""); setTo("");
  }

  const activeCcys = currencies.filter((c) => c.active && (c.code === "USD" || c.rate > 0));
  const fx = ccy === "USD" ? 1 : (currencies.find((c) => c.code === ccy)?.rate ?? 1);
  // Convert a payment's native amount into the chosen display currency (via USD).
  const inDisplay = (p: PaymentListItem) => toUSD(p.amount, p.currency, currencies) * fx;
  const money = (v: number) => fmtMoney(ccy, v);

  const rows = useMemo(() => {
    const list = payments.filter((p) => {
      const day = p.paid_on.slice(0, 10);
      return (
        (catFilter === "all" || p.category_id === catFilter) &&
        (billingFilter === "all" || p.billing_period_months === Number(billingFilter)) &&
        (userFilter === "all" || p.seats.some((s) => s.trim() === userFilter)) &&
        (from === "" || day >= from) &&
        (to === "" || day <= to)
      );
    });
    const dir = sortDir === "asc" ? 1 : -1;
    const val = (p: PaymentListItem): string | number => {
      switch (sortKey) {
        case "paid_on": return p.paid_on.slice(0, 10);
        case "service": return p.service.toLowerCase();
        case "amount": return toUSD(p.amount, p.currency, currencies);
      }
    };
    return [...list].sort((a, b) => {
      const va = val(a), vb = val(b);
      if (va < vb) return -1 * dir;
      if (va > vb) return 1 * dir;
      return 0;
    });
  }, [payments, catFilter, billingFilter, userFilter, from, to, sortKey, sortDir, currencies]);

  const total = rows.reduce((sum, p) => sum + inDisplay(p), 0);

  // Build the array-of-arrays the Excel sheet is made from (shared by the plain
  // export and the with-invoices zip, so both show exactly what's filtered).
  // Each active currency gets its own "Amount (CODE)" column (the payment converted
  // into that currency), plus a "Paid in" column naming the currency actually used.
  function buildAoA(): (string | number)[][] {
    const money2 = (v: number) => Number(v.toFixed(2));
    const amountIn = (p: PaymentListItem, code: string, rate: number) =>
      money2(toUSD(p.amount, p.currency, currencies) * (code === "USD" ? 1 : rate));
    const header = [
      t("payments.paidOn"), t("finance.service"), t("accounts.category"), t("finance.users"),
      ...activeCcys.map((c) => `${t("payments.amount")} (${c.code})`),
      t("payments.paidIn"), t("payments.note"), t("payments.invoice"),
    ];
    const data = rows.map((p) => [
      p.paid_on.slice(0, 10),
      p.service,
      p.category_name ? localCategoryName(categories, p.category_id, p.category_name, lang) : t("accounts.uncategorizedPlain"),
      seatNames(p).map((s) => s.trim()).filter(Boolean).join(", "),
      ...activeCcys.map((c) => amountIn(p, c.code, c.rate)),
      p.currency,
      p.note ?? "",
      p.attachment_name ?? "",
    ]);
    const totals = activeCcys.map((c) =>
      money2(rows.reduce((s, p) => s + toUSD(p.amount, p.currency, currencies) * (c.code === "USD" ? 1 : c.rate), 0)),
    );
    const totalRow = [t("payments.total"), "", "", "", ...totals, "", "", ""];
    return [header, ...data, totalRow];
  }

  function exportExcel() {
    downloadSheet(buildAoA(), "Payments", "payments.xlsx");
  }

  // Rows that have a linked invoice — the files bundled into the zip export.
  const invoiceRows = rows.filter((p) => p.attachment_id && p.attachment_name);

  // Export a .zip: the same Excel sheet plus an invoices/ folder with each linked
  // invoice file (decrypted). De-duplicates repeated filenames.
  async function exportZip() {
    setBusy(true);
    try {
      const wb = XLSX.utils.book_new();
      XLSX.utils.book_append_sheet(wb, XLSX.utils.aoa_to_sheet(buildAoA()), "Payments");
      const xlsx = XLSX.write(wb, { bookType: "xlsx", type: "array" }) as ArrayBuffer;

      const zip = new JSZip();
      zip.file("payments.xlsx", xlsx);
      const inv = zip.folder("invoices")!;

      // One fetch per unique attachment id; keep names unique within the folder.
      const seen = new Map<string, number>();
      const uniqueIds = [...new Set(invoiceRows.map((p) => p.attachment_id!))];
      await Promise.all(uniqueIds.map(async (id) => {
        const row = invoiceRows.find((p) => p.attachment_id === id)!;
        const blob = await fetchAttachmentBlob(id);
        let name = row.attachment_name!;
        const n = seen.get(name) ?? 0;
        seen.set(name, n + 1);
        if (n > 0) {
          const dot = name.lastIndexOf(".");
          name = dot > 0 ? `${name.slice(0, dot)} (${n})${name.slice(dot)}` : `${name} (${n})`;
        }
        inv.file(name, blob);
      }));

      const out = await zip.generateAsync({ type: "blob" });
      const url = URL.createObjectURL(out);
      const a = document.createElement("a");
      a.href = url;
      a.download = "payments.zip";
      a.click();
      URL.revokeObjectURL(url);
    } catch (err) {
      errorAlert((err as Error).message);
    } finally {
      setBusy(false);
    }
  }

  function toggleSort(key: SortKey) {
    if (sortKey === key) setSortDir((d) => (d === "asc" ? "desc" : "asc"));
    else { setSortKey(key); setSortDir(key === "paid_on" ? "desc" : "asc"); }
  }
  function SortHead({ k, label }: { k: SortKey; label: string }) {
    return (
      <th className="sortable" onClick={() => toggleSort(k)}>
        {label}{sortKey === k && <span className="sort-arrow">{sortDir === "asc" ? "▲" : "▼"}</span>}
      </th>
    );
  }

  return (
    <div className="card">
      <div className="topbar fin-topbar" style={{ marginBottom: 12 }}>
        <h2 className="section-title" style={{ margin: 0 }}>{t("finance.payments")}</h2>
        <div className="fin-topbar-actions" style={{ display: "flex", gap: 10, alignItems: "center" }}>
          {activeCcys.length > 1 && (
            <select dir="ltr" value={ccy} onChange={(e) => setCcy(e.target.value)}>
              {activeCcys.map((c) => <option key={c.code} value={c.code}>{c.code}</option>)}
            </select>
          )}
          {rows.length > 0 && (
            <CircleButton variant="ok" title={t("export.exportBtn")} onClick={exportExcel}><FaFileExcel /></CircleButton>
          )}
          {invoiceRows.length > 0 && (
            <CircleButton title={t("payments.exportWithInvoices")} disabled={busy} onClick={exportZip}><FaFileArchive /></CircleButton>
          )}
        </div>
      </div>

      <div className="filter-bar fin-filter-bar" style={{ marginBottom: 12 }}>
        <select dir={lang === "ar" ? "rtl" : "ltr"} value={catFilter} onChange={(e) => setCatFilter(e.target.value)}>
          <option value="all">{t("finance.allCategories")}</option>
          {catOptions.map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}
        </select>
        <select dir="ltr" value={billingFilter} onChange={(e) => setBillingFilter(e.target.value)}>
          <option value="all">{t("finance.allBilling")}</option>
          {billingOptions.map((m) => <option key={m} value={m}>{billingLabel(t, m)}</option>)}
        </select>
        <select dir={lang === "ar" ? "rtl" : "ltr"} value={userFilter} onChange={(e) => setUserFilter(e.target.value)}>
          <option value="all">{t("finance.allUsers")}</option>
          {userOptions.map((u) => <option key={u.en} value={u.en}>{u.loc}</option>)}
        </select>
        <input type="date" dir="ltr" value={from} onChange={(e) => setFrom(e.target.value)} title={t("payments.from")} aria-label={t("payments.from")} />
        <input type="date" dir="ltr" value={to} onChange={(e) => setTo(e.target.value)} title={t("payments.to")} aria-label={t("payments.to")} />
        {hasFilter && (
          <Button variant="ghost" className="clear-filters" onClick={clearFilters}>{t("finance.clearFilters")}</Button>
        )}
      </div>

      <div className="table-scroll">
        <table className="accounts fin-accounts">
          <thead>
            <tr>
              <SortHead k="paid_on" label={t("payments.paidOn")} />
              <SortHead k="service" label={t("finance.service")} />
              <th>{t("finance.users")}</th>
              <SortHead k="amount" label={`${t("payments.amount")} (${ccy})`} />
              <th>{t("payments.invoice")}</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((p) => (
              <tr key={p.id} className="clickable-row" onClick={() => navigate(`/account/${p.account_id}`)}>
                <td className="nowrap"><span dir="ltr">{p.paid_on.slice(0, 10)}</span></td>
                <td>
                  <span className="svc">
                    {p.category_name && <span className="cat-tag"><Icon name={p.category_icon} size={13} /></span>}
                    {p.service}
                  </span>
                </td>
                <td className="wrap-cell">{seatNames(p).map((s) => s.trim()).filter(Boolean).join(", ") || "—"}</td>
                <td className="nowrap">
                  <span dir="ltr">{money(inDisplay(p))}</span>
                  {p.currency !== ccy && <span className="muted" dir="ltr" style={{ fontSize: 11, display: "block" }}>{fmtMoney(p.currency, p.amount)}</span>}
                </td>
                <td className="wrap-cell">
                  {p.attachment_id && p.attachment_name ? (
                    <button
                      className="link"
                      onClick={(e) => { e.stopPropagation(); downloadAttachment(p.attachment_id!, p.attachment_name!).catch((err) => errorAlert((err as Error).message)); }}
                    >
                      {p.attachment_name}
                    </button>
                  ) : "—"}
                </td>
              </tr>
            ))}
            {rows.length === 0 && (
              <tr><td colSpan={5} className="muted">{hasFilter ? t("accounts.noMatches") : t("payments.none")}</td></tr>
            )}
          </tbody>
          <tfoot>
            <tr className="total-row">
              <td colSpan={3}>{t("payments.total")}</td>
              <td colSpan={2}><strong dir="ltr" style={{ display: "inline-block" }}>{money(total)}</strong></td>
            </tr>
          </tfoot>
        </table>
      </div>
    </div>
  );
}
