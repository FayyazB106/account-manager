import { useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import { useTranslation } from "react-i18next";
import * as XLSX from "xlsx";
import { useLayout } from "../components/Layout";
import { localCategoryName } from "../categoryName";
import { billingLabel } from "../account-ui";
import { fmtMoney, withUsdAmounts } from "../money";
import { Button } from "../components/Button";
import { CircleButton } from "../components/CircleButton";
import { Icon } from "../icons";
import { FaFileExcel } from "react-icons/fa";
import type { AccountListItem } from "../api";

// Write an array-of-arrays as an .xlsx and trigger a download.
export function downloadSheet(aoa: (string | number)[][], sheetName: string, fileName: string) {
  const ws = XLSX.utils.aoa_to_sheet(aoa);
  const wb = XLSX.utils.book_new();
  XLSX.utils.book_append_sheet(wb, ws, sheetName);
  XLSX.writeFile(wb, fileName);
}

// Average monthly cost: the per-period amount spread over its billing months.
function avgMonthly(a: AccountListItem): number {
  return a.billing_period_months > 0 && a.amount ? a.amount / a.billing_period_months : 0;
}
function namedUsers(seats: string[]): string {
  return seats.map((s) => s.trim()).filter(Boolean).join(", ");
}

type SortKey = "service" | "login" | "seats" | "cost";

export function FinanceAccounts() {
  const { accounts: rawAccounts, currencies, lang, categories } = useLayout();
  // Every account has its own currency; the maths below (average cost, totals, the
  // per-currency columns) needs USD, so normalise once here.
  const accounts = useMemo(() => withUsdAmounts(rawAccounts, currencies), [rawAccounts, currencies]);
  const ownCurrency = useMemo(() => new Map(rawAccounts.map((a) => [a.id, a.currency || "USD"])), [rawAccounts]);
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [mode, setMode] = useState<"monthly" | "yearly">("monthly");
  const [ccy, setCcy] = useState("USD");
  const [sortKey, setSortKey] = useState<SortKey>("service");
  const [sortDir, setSortDir] = useState<"asc" | "desc">("asc");
  const [catFilter, setCatFilter] = useState("all");
  const [billingFilter, setBillingFilter] = useState("all");
  const [userFilter, setUserFilter] = useState("all");
  const seatNames = (a: AccountListItem) =>
    lang === "ar" ? a.seats.map((s, i) => (a.seats_ar?.[i] ? a.seats_ar[i]! : s)) : a.seats;

  // Base set: active, recurring subscriptions (Finance ignores inactive/free/one-time).
  const baseList = useMemo(
    () => accounts.filter((a) => a.status === "active" && a.billing_period_months > 0),
    [accounts],
  );

  // Filter options, derived from the base set so a dropdown always offers every
  // value that exists (independent of the other active filters).
  const catOptions = useMemo(() => {
    const m = new Map<string, string>();
    for (const a of baseList) if (a.category_id) m.set(a.category_id, localCategoryName(categories, a.category_id, a.category_name || a.category_id, lang));
    return [...m.entries()].map(([id, name]) => ({ id, name })).sort((x, y) => x.name.localeCompare(y.name));
  }, [baseList, categories, lang]);
  const billingOptions = useMemo(() => {
    const s = new Set<number>();
    for (const a of baseList) s.add(a.billing_period_months);
    return [...s].sort((x, y) => x - y);
  }, [baseList]);
  const userOptions = useMemo(() => {
    const m = new Map<string, string>(); // canonical (EN) name → localized display
    for (const a of baseList)
      a.seats.forEach((s, i) => {
        const en = s.trim();
        if (!en) return;
        const loc = lang === "ar" && a.seats_ar?.[i] ? a.seats_ar[i]! : s;
        if (!m.has(en)) m.set(en, loc);
      });
    return [...m.entries()].map(([en, loc]) => ({ en, loc })).sort((x, y) => x.loc.localeCompare(y.loc));
  }, [baseList, lang]);

  const hasFilter = catFilter !== "all" || billingFilter !== "all" || userFilter !== "all";
  function clearFilters() { setCatFilter("all"); setBillingFilter("all"); setUserFilter("all"); }

  const activeCcys = currencies.filter((c) => c.active && (c.code === "USD" || c.rate > 0));
  const fx = ccy === "USD" ? 1 : (currencies.find((c) => c.code === ccy)?.rate ?? 1);
  const money = (v: number) => fmtMoney(ccy, v * fx);

  const factor = mode === "yearly" ? 12 : 1;
  const rate = (a: AccountListItem) => avgMonthly(a) * factor;
  const rateLabel = mode === "yearly" ? t("finance.avgAnnual") : t("finance.avgMonthly");

  // Apply the category / billing / user filters, then sort. The total, footer,
  // and Excel export all derive from `rows`, so they follow the filters.
  const rows = useMemo(() => {
    const list = baseList.filter((a) =>
      (catFilter === "all" || a.category_id === catFilter) &&
      (billingFilter === "all" || a.billing_period_months === Number(billingFilter)) &&
      (userFilter === "all" || a.seats.some((s) => s.trim() === userFilter)),
    );
    const dir = sortDir === "asc" ? 1 : -1;
    const val = (a: AccountListItem): string | number => {
      switch (sortKey) {
        case "service": return a.service.toLowerCase();
        case "login": return (a.login ?? "").toLowerCase();
        case "seats": return a.seats.length;
        case "cost": return avgMonthly(a);
      }
    };
    return [...list].sort((a, b) => {
      const va = val(a), vb = val(b);
      if (va < vb) return -1 * dir;
      if (va > vb) return 1 * dir;
      return 0;
    });
  }, [baseList, catFilter, billingFilter, userFilter, sortKey, sortDir]);

  const total = rows.reduce((sum, a) => sum + rate(a), 0);
  const anyApprox = rows.some((a) =>
    rate(a) > 0 &&
    !((mode === "monthly" && a.billing_period_months === 1) || (mode === "yearly" && a.billing_period_months === 12))
  );

  // Export all currencies at once (not just the on-screen one), mirroring the
  // Payments export: an Amount column and an Avg-Cost column per active currency,
  // plus a "Paid in" column naming the account's own currency; conv() converts the
  // (USD-normalised) value into each active currency.
  function exportExcel() {
    const conv = (usd: number, code: string, r: number) => Number((usd * (code === "USD" ? 1 : r)).toFixed(2));
    const header = [
      t("finance.service"), t("accounts.category"), t("finance.login"), t("finance.seats"), t("finance.users"),
      ...activeCcys.map((c) => `${t("accounts.amount")} (${c.code})`),
      t("payments.paidIn"),
      t("finance.billing"),
      ...activeCcys.map((c) => `${rateLabel} (${c.code})`),
    ];
    const data = rows.map((a) => [
      a.service,
      a.category_name ? localCategoryName(categories, a.category_id, a.category_name, lang) : t("accounts.uncategorizedPlain"),
      a.login ?? "",
      a.seats.length,
      namedUsers(seatNames(a)),
      ...activeCcys.map((c) => (a.amount != null ? conv(a.amount, c.code, c.rate) : "")),
      ownCurrency.get(a.id) ?? "USD",
      billingLabel(t, a.billing_period_months, a.amount),
      ...activeCcys.map((c) => conv(rate(a), c.code, c.rate)),
    ]);
    const totalRow = [
      mode === "yearly" ? t("finance.totalAnnual") : t("finance.totalMonthly"),
      "", "", "", "",
      ...activeCcys.map(() => ""),
      "",
      "",
      ...activeCcys.map((c) => conv(total, c.code, c.rate)),
    ];
    downloadSheet([header, ...data, totalRow], t("finance.accounts"), `accounts-${mode}.xlsx`);
  }

  function toggleSort(key: SortKey) {
    if (sortKey === key) setSortDir((d) => (d === "asc" ? "desc" : "asc"));
    else { setSortKey(key); setSortDir("asc"); }
  }
  function SortHead({ k, label }: { k: SortKey; label: string }) {
    return (
      <th className="sortable" onClick={() => toggleSort(k)}>
        {label}{sortKey === k && <span className="sort-arrow">{sortDir === "asc" ? "▲" : "▼"}</span>}
      </th>
    );
  }

  // A cell is "exact" only when the billing period matches the displayed period
  // (a monthly plan shown monthly, or an annual plan shown yearly). Otherwise
  // it's derived, so we prefix "~".
  function rateCell(a: AccountListItem): string {
    const v = rate(a);
    if (v <= 0) return "—";
    const exact = (mode === "monthly" && a.billing_period_months === 1) ||
      (mode === "yearly" && a.billing_period_months === 12);
    return `${exact ? "" : "~"}${money(v)}`;
  }

  return (
    <div className="card">
      <div className="topbar fin-topbar" style={{ marginBottom: 12 }}>
        <h2 className="section-title" style={{ margin: 0 }}>{t("finance.accounts")}</h2>
        <div className="fin-topbar-actions" style={{ display: "flex", gap: 10, alignItems: "center" }}>
          {activeCcys.length > 1 && (
            <select dir="ltr" value={ccy} onChange={(e) => setCcy(e.target.value)}>
              {activeCcys.map((c) => <option key={c.code} value={c.code}>{c.code}</option>)}
            </select>
          )}
          <Button variant="ghost" onClick={() => setMode((m) => (m === "monthly" ? "yearly" : "monthly"))}>
            {mode === "monthly" ? t("finance.switchToYearly") : t("finance.switchToMonthly")}
          </Button>
          <CircleButton variant="ok" title={t("export.exportBtn")} onClick={exportExcel}><FaFileExcel /></CircleButton>
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
        {hasFilter && (
          <Button variant="ghost" className="clear-filters" onClick={clearFilters}>{t("finance.clearFilters")}</Button>
        )}
      </div>

      <div className="table-scroll">
      <table className="accounts fin-accounts">
        <colgroup>
          <col style={{ width: "22%" }} />
          <col style={{ width: "22%" }} />
          <col style={{ width: "8%" }} />
          <col style={{ width: "18%" }} />
          <col style={{ width: "16%" }} />
          <col style={{ width: "16%" }} />
        </colgroup>
        <thead>
          <tr>
            <SortHead k="service" label={t("finance.service")} />
            <SortHead k="login" label={t("finance.login")} />
            <SortHead k="seats" label={t("finance.seats")} />
            <th>{t("finance.users")}</th>
            <th>{t("finance.billing")}</th>
            <SortHead k="cost" label={rateLabel} />
          </tr>
        </thead>
        <tbody>
          {rows.map((a) => (
            <tr key={a.id} className="clickable-row" onClick={() => navigate(`/account/${a.id}`)}>
              <td>
                <span className="svc">
                  {a.category_name && <span className="cat-tag"><Icon name={a.category_icon} size={13} /></span>}
                  {a.service}
                </span>
              </td>
              <td className="nowrap">{a.login ?? "—"}</td>
              <td>{a.seats.length > 0 ? a.seats.length : "—"}</td>
              <td className="wrap-cell">{namedUsers(seatNames(a)) || "—"}</td>
              <td><span dir="ltr">{a.amount != null ? `${money(a.amount)} · ${billingLabel(t, a.billing_period_months, a.amount)}` : billingLabel(t, a.billing_period_months)}</span></td>
              <td><span dir="ltr">{rateCell(a)}</span></td>
            </tr>
          ))}
          {rows.length === 0 && (
            <tr><td colSpan={6} className="muted">{hasFilter ? t("accounts.noMatches") : t("accounts.none")}</td></tr>
          )}
        </tbody>
        <tfoot>
          <tr className="total-row">
            <td colSpan={5}>{mode === "yearly" ? t("finance.totalAnnual") : t("finance.totalMonthly")}</td>
            <td><strong dir="ltr" style={{ display: "inline-block" }}>{anyApprox ? "~" : ""}{money(total)}</strong></td>
          </tr>
        </tfoot>
      </table>
      </div>
    </div>
  );
}
