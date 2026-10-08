import { useMemo, useState } from "react";
import { useNavigate, useParams, useSearchParams } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { useLayout } from "../components/Layout";
import { Button } from "../components/Button";
import { CircleButton } from "../components/CircleButton";
import { AccountRow, AccountForm, rotationDueDate, paymentStatus } from "../account-ui";
import type { AccountListItem } from "../api";

type SortKey = "service" | "login" | "seats" | "status" | "next_payment" | "rotation";

function sortValue(a: AccountListItem, key: SortKey): string | number {
  switch (key) {
    case "service": return a.service.toLowerCase();
    case "login": return (a.login ?? "").toLowerCase();
    case "seats": return a.seats.length;
    case "status": return a.status;
    case "next_payment": return a.next_payment_date ?? "￿";
    case "rotation": return rotationDueDate(a.password_changed_at, a.rotate_every_days) ?? "￿";
  }
}

// A date-status filter value ("due" = overdue or soon) matches a PaymentStatus.
function statusMatches(filter: string, s: string): boolean {
  return filter === "due" ? s === "overdue" || s === "soon" : s === filter;
}

function textMatches(a: AccountListItem, query: string): boolean {
  // Each whitespace-separated term must match somewhere (AND, order-independent),
  // so "zaki adobe" finds the Adobe account whose user is "…Zaki…".
  const terms = query.trim().toLowerCase().split(/\s+/).filter(Boolean);
  if (terms.length === 0) return true;
  const hay = [a.service, a.login ?? "", ...a.seats, ...(a.seats_ar ?? []).map((s) => s ?? "")]
    .join(" ").toLowerCase();
  return terms.every((t) => hay.includes(t));
}

export function AccountsPage() {
  const { accounts, categories, refresh, lang } = useLayout();
  const { t } = useTranslation();
  const { categoryId } = useParams();
  const navigate = useNavigate();
  const [params, setParams] = useSearchParams();

  const query = params.get("q") ?? "";
  const fStatus = params.get("status") ?? "";
  const fPayment = params.get("payment") ?? "";
  const fRotation = params.get("rotation") ?? "";
  const fPayDate = params.get("paydate") ?? "";   // exact next-payment day (from tiles)
  const fRotDate = params.get("rotdate") ?? "";   // exact next-rotation day (from tiles)

  // Update one filter param in the URL, dropping it when cleared.
  function setParam(key: string, value: string) {
    const next = new URLSearchParams(params);
    if (value) next.set(key, value); else next.delete(key);
    setParams(next, { replace: true });
  }

  const [showAdd, setShowAdd] = useState(false);
  const [sortKey, setSortKey] = useState<SortKey>("service");
  const [sortDir, setSortDir] = useState<"asc" | "desc">("asc");

  const cat = categoryId ? categories.find((c) => c.id === categoryId) : undefined;
  const title = categoryId
    ? (cat ? (lang === "ar" && cat.name_ar ? cat.name_ar : cat.name) : t("accounts.title"))
    : t("sidebar.allAccounts");

  const filtered = useMemo(() => {
    let list = categoryId ? accounts.filter((a) => a.category_id === categoryId) : accounts;
    if (fStatus) list = list.filter((a) => a.status === fStatus);
    if (fPayment) list = list.filter((a) => statusMatches(fPayment, paymentStatus(a.next_payment_date)));
    if (fRotation) list = list.filter((a) => statusMatches(fRotation, paymentStatus(rotationDueDate(a.password_changed_at, a.rotate_every_days) ?? undefined)));
    if (fPayDate) list = list.filter((a) => (a.next_payment_date ?? "").slice(0, 10) === fPayDate);
    if (fRotDate) list = list.filter((a) => (rotationDueDate(a.password_changed_at, a.rotate_every_days) ?? "") === fRotDate);
    if (query.trim()) list = list.filter((a) => textMatches(a, query));
    return list;
  }, [accounts, categoryId, query, fStatus, fPayment, fRotation, fPayDate, fRotDate]);

  const visible = useMemo(() => {
    const dir = sortDir === "asc" ? 1 : -1;
    return [...filtered].sort((a, b) => {
      if (sortKey !== "status") {
        const ai = a.status === "active" ? 0 : 1;
        const bi = b.status === "active" ? 0 : 1;
        if (ai !== bi) return ai - bi;
      }
      const va = sortValue(a, sortKey), vb = sortValue(b, sortKey);
      if (va < vb) return -1 * dir;
      if (va > vb) return 1 * dir;
      return 0;
    });
  }, [filtered, sortKey, sortDir]);

  function toggleSort(key: SortKey) {
    if (sortKey === key) setSortDir((d) => (d === "asc" ? "desc" : "asc"));
    else { setSortKey(key); setSortDir("asc"); }
  }

  function SortHead({ k, label, center }: { k: SortKey; label: string; center?: boolean }) {
    return (
      <th className={`sortable ${center ? "center" : ""}`.trim()} onClick={() => toggleSort(k)}>
        {label}{sortKey === k && <span className="sort-arrow">{sortDir === "asc" ? "▲" : "▼"}</span>}
      </th>
    );
  }

  const hasFilter = !!(query.trim() || fStatus || fPayment || fRotation || fPayDate || fRotDate);

  return (
    <>
      <header className="topbar">
        <h1>{title}</h1>
        <CircleButton open={showAdd} title={showAdd ? t("common.close") : t("accounts.add")} onClick={() => setShowAdd((v) => !v)}>
          +
        </CircleButton>
      </header>

      <div className="filter-bar">
        <input
          className="search-input"
          type="search"
          value={query}
          onChange={(e) => setParam("q", e.target.value)}
          placeholder={t("accounts.search")}
        />
        <select dir="ltr" value={fStatus} onChange={(e) => setParam("status", e.target.value)}>
          <option value="">{t("accounts.fStatus")}: {t("accounts.filterAll")}</option>
          <option value="active">{t("accounts.active")}</option>
          <option value="inactive">{t("accounts.inactive")}</option>
        </select>
        <select dir="ltr" value={fPayment} onChange={(e) => setParam("payment", e.target.value)}>
          <option value="">{t("accounts.fPayment")}: {t("accounts.filterAll")}</option>
          <option value="soon">{t("accounts.optSoon")}</option>
          <option value="overdue">{t("accounts.optOverdue")}</option>
        </select>
        <select dir="ltr" value={fRotation} onChange={(e) => setParam("rotation", e.target.value)}>
          <option value="">{t("accounts.fRotation")}: {t("accounts.filterAll")}</option>
          <option value="soon">{t("accounts.optSoon")}</option>
          <option value="overdue">{t("accounts.optOverdue")}</option>
        </select>
        {hasFilter && (
          <Button variant="ghost" className="clear-filters" onClick={() => setParams({}, { replace: true })}>
            {t("accounts.clear")}
          </Button>
        )}
      </div>

      {showAdd && (
        <AccountForm
          existing={null}
          categories={categories}
          defaultCategoryId={categoryId}
          onSaved={() => { setShowAdd(false); refresh(); }}
          onCancel={() => setShowAdd(false)}
        />
      )}

      <div className="table-scroll">
      <table className="accounts">
        <colgroup>
          <col style={{ width: "15%" }} />
          <col style={{ width: "18%" }} />
          <col style={{ width: "12%" }} />
          <col style={{ width: "6%" }} />
          <col style={{ width: "13%" }} />
          <col style={{ width: "9%" }} />
          <col style={{ width: "11%" }} />
          <col style={{ width: "10%" }} />
          <col style={{ width: "6%" }} />
        </colgroup>
        <thead>
          <tr>
            <SortHead k="service" label={t("accounts.service")} />
            <SortHead k="login" label={t("accounts.login")} />
            <th>{t("accounts.password")}</th>
            <SortHead k="seats" label={t("accounts.seats")} />
            <th>{t("accounts.users")}</th>
            <SortHead k="status" label={t("accounts.status")} center />
            <SortHead k="next_payment" label={t("accounts.nextPaymentShort")} />
            <SortHead k="rotation" label={t("accounts.nextRotationCol")} />
            <th></th>
          </tr>
        </thead>
        <tbody>
          {visible.map((a) => (
            <AccountRow key={a.id} account={a} onOpen={() => navigate(`/account/${a.id}`)} />
          ))}
          {visible.length === 0 && (
            <tr><td colSpan={9} className="muted">{hasFilter ? t("accounts.noMatches") : t("accounts.none")}</td></tr>
          )}
        </tbody>
      </table>
      </div>
    </>
  );
}
