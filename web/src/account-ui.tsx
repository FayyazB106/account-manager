import { useEffect, useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import {
  createAccount,
  updateAccount,
  setAccountStatus,
  deleteAccount,
  listPeople,
  type AccountListItem,
  type NewAccount,
  type Category,
  type Currency,
  type Person,
} from "./api";
import { CopyText } from "./components/CopyText";
import { Button } from "./components/Button";
import { StatusBadge } from "./components/Badge";
import { Modal } from "./components/Modal";
import { Field } from "./components/Field";
import { SecretReveal } from "./components/SecretReveal";
import { AccountTOTP } from "./components/AccountTOTP";
import { PaymentForm } from "./components/PaymentForm";
import { IconButton } from "./components/IconButton";
import { Icon } from "./icons";
import { confirmDialog, errorAlert, toast } from "./alerts";
import { PasswordField } from "./components/PasswordField";
import { Combobox } from "./components/Combobox";
import { useLayout } from "./components/Layout";
import { localCategoryName } from "./categoryName";
import { amountVariants } from "./money";
import { FaChevronRight, FaTrash } from "react-icons/fa";

// --- Helpers ---------------------------------------------------------------

type TFunc = (key: string, opts?: Record<string, unknown>) => string;

export function billingLabel(t: TFunc, months: number, amount?: number | null): string {
  if (!months) return amount && amount > 0 ? t("accounts.oneTime") : t("accounts.free");
  if (months === 1) return t("accounts.monthly");
  if (months === 3) return t("accounts.quarterly");
  if (months === 6) return t("accounts.sixMonths");
  if (months === 12) return t("accounts.annual");
  return t("accounts.everyNMonths", { n: months });
}

function namedUsers(seats: string[]): string {
  return seats.map((s) => s.trim()).filter(Boolean).join(", ");
}

// Ensure a URL has a protocol so the link isn't treated as a relative path.
function withProtocol(url: string): string {
  return /^https?:\/\//i.test(url) ? url : `https://${url}`;
}

const DUE_SOON_DAYS = 7;
type PaymentStatus = "none" | "overdue" | "soon" | "ok";

export function paymentStatus(date?: string): PaymentStatus {
  if (!date) return "none";
  // date may be "YYYY-MM-DD" or a full ISO timestamp — normalize to the day.
  const due = new Date(date.slice(0, 10) + "T00:00:00");
  const today = new Date();
  today.setHours(0, 0, 0, 0);
  const days = Math.round((due.getTime() - today.getTime()) / 86_400_000);
  if (days < 0) return "overdue";
  if (days <= DUE_SOON_DAYS) return "soon";
  return "ok";
}

function daysUntil(date: string): number {
  const due = new Date(date.slice(0, 10) + "T00:00:00");
  const today = new Date();
  today.setHours(0, 0, 0, 0);
  return Math.round((due.getTime() - today.getTime()) / 86_400_000);
}

export function rotationDueDate(changedAt: string, everyDays?: number): string | null {
  if (!everyDays) return null;
  const d = new Date(changedAt);
  d.setDate(d.getDate() + everyDays);
  return d.toISOString().slice(0, 10);
}

function rotationStatus(changedAt: string, everyDays?: number): PaymentStatus {
  const due = rotationDueDate(changedAt, everyDays);
  return due ? paymentStatus(due) : "none";
}

// --- Row -------------------------------------------------------------------

export function AccountRow({ account, onOpen }: { account: AccountListItem; onOpen: () => void }) {
  const { t } = useTranslation();
  const { lang, categories: allCategories } = useLayout();
  const catName = localCategoryName(allCategories, account.category_id, account.category_name, lang);
  const isActive = account.status === "active";
  const lapsed =
    account.billing_period_months > 0 &&
    paymentStatus(account.next_payment_date) === "overdue";
  const seats = lang === "ar"
    ? account.seats.map((s, i) => (account.seats_ar?.[i] ? account.seats_ar[i]! : s))
    : account.seats;
  const rotateDue = rotationDueDate(account.password_changed_at, account.rotate_every_days) ?? undefined;

  return (
    <tr className={isActive ? "" : "inactive-row"}>
      <td>
        <span className="svc">
          {account.category_name && (
            <span className="cat-tag" title={catName}>
              <Icon name={account.category_icon} size={13} />
            </span>
          )}
          {account.service}
        </span>
      </td>
      <td className="nowrap">{account.login ? <CopyText value={account.login} /> : "—"}</td>
      <td><SecretReveal accountId={account.id} /></td>
      <td>{account.seats.length > 0 ? account.seats.length : "—"}</td>
      <td className="wrap-cell">{namedUsers(seats) || "—"}</td>
      <td className="center">
        {lapsed && isActive
          ? <span className="status-dot status-lapsed" title={t("accounts.subLapsedTitle")} aria-label={t("accounts.subLapsed")} />
          : <StatusBadge status={account.status} />}
      </td>
      <td><PaymentCell date={account.next_payment_date} /></td>
      <td><PaymentCell date={rotateDue} /></td>
      <td className="open-cell">
        <button className="open-btn" title={t("accounts.open")} onClick={onOpen}><FaChevronRight size={14} /></button>
      </td>
    </tr>
  );
}

// --- General tab (account info + 2FA) --------------------------------------
// The account detail page is a sidebar layout (see AccountLayout); this is its
// first tab. Password history, payments, and attachments are their own tabs.

export function AccountGeneral({
  account,
  categories,
  currencies,
  onBack,
  onChanged,
}: {
  account: AccountListItem;
  categories: Category[];
  currencies: Currency[];
  onBack: () => void;
  onChanged: () => void;
}) {
  const [editing, setEditing] = useState(false);
  const [markingPaid, setMarkingPaid] = useState(false);
  const isActive = account.status === "active";
  const { t } = useTranslation();
  const { lang, categories: allCategories } = useLayout();
  const catName = localCategoryName(allCategories, account.category_id, account.category_name, lang);
  const seatLabel = (i: number) => (lang === "ar" && account.seats_ar?.[i] ? account.seats_ar[i]! : account.seats[i]);

  async function setStatus(status: "active" | "inactive") {
    if (status === "inactive") {
      const ok = await confirmDialog({
        title: t("accounts.deactivateTitle", { service: account.service }),
        text: t("accounts.deactivateText"),
        confirmText: t("accounts.deactivate"),
        warn: true,
      });
      if (!ok) return;
    }
    try {
      await setAccountStatus(account.id, status);
      toast(t(status === "active" ? "accounts.activated" : "accounts.deactivated"));
      onChanged();
    }
    catch (err) { errorAlert((err as Error).message); }
  }

  async function remove() {
    const ok = await confirmDialog({
      title: t("accounts.deleteTitle", { service: account.service }),
      text: t("accounts.deleteText"),
      confirmText: t("common.delete"),
      danger: true,
    });
    if (!ok) return;
    try { await deleteAccount(account.id); onChanged(); onBack(); }
    catch (err) { errorAlert((err as Error).message); }
  }

  const rotateDue = account.rotate_every_days
    ? rotationDueDate(account.password_changed_at, account.rotate_every_days)
    : null;

  return (
    <div>
      <div className="card detail">
        <div className="detail-head">
          <div>
            <h1>
              {account.category_icon && <span className="cat-tag"><Icon name={account.category_icon} size={20} /></span>}
              {account.service}
            </h1>
            {account.website && (
              <a className="website-link" href={withProtocol(account.website)} target="_blank" rel="noreferrer">
                {account.website}
              </a>
            )}
          </div>
          <StatusBadge status={account.status} />
        </div>

        <dl className="fields">
          <div><dt>{t("accounts.category")}</dt><dd>{account.category_name ? catName : t("accounts.uncategorizedPlain")}</dd></div>
          <div><dt>{t("accounts.billing")}</dt><dd>{billingLabel(t, account.billing_period_months, account.amount)}</dd></div>
          <div><dt>{t("accounts.login")}</dt><dd>{account.login ? <CopyText value={account.login} /> : "—"}</dd></div>
          <div><dt>{t("accounts.password")}</dt><dd><SecretReveal accountId={account.id} /></dd></div>
          {account.breach_count != null && (
            <div>
              <dt>{t("accounts.breachStatus")}</dt>
              <dd>{account.breach_count > 0
                ? <span className="danger-text">{t("accounts.breachFound", { n: account.breach_count.toLocaleString() })}</span>
                : <span className="muted">{t("accounts.breachNone")}</span>}</dd>
            </div>
          )}
          <div><dt>{t("accounts.amount")}</dt><dd>{account.amount != null ? amountVariants(account.amount, currencies) : "—"}</dd></div>
          <div>
            <dt>{t("accounts.nextPaymentShort")}</dt>
            <dd>{account.next_payment_date ? <PaymentCell date={account.next_payment_date} /> : "—"}</dd>
          </div>
          <div>
            <dt>{t("accounts.passwordRotation")}</dt>
            <dd>{account.rotate_every_days ? t("accounts.rotationEvery", { days: account.rotate_every_days, date: rotateDue }) : "—"}</dd>
          </div>
          <div><dt>{t("accounts.passwordChangedLabel")}</dt><dd>{account.password_changed_at.slice(0, 10)}</dd></div>
          <div className="full">
            <dt>{t("accounts.seats")} {account.seats.length > 0 && `(${account.seats.length})`}</dt>
            <dd>
              {account.seats.length === 0 ? "—" : (
                <ul className="seat-list">
                  {account.seats.map((_s, i) => {
                    const label = seatLabel(i);
                    return <li key={i}>{label ? label : <span className="muted">{t("accounts.seatUnnamed", { n: i + 1 })}</span>}</li>;
                  })}
                </ul>
              )}
            </dd>
          </div>
          <div className="full"><dt>{t("accounts.notes")}</dt><dd>{account.notes ? account.notes : "—"}</dd></div>
        </dl>

        <div className="detail-actions">
          <div className="detail-actions-group">
            <Button onClick={() => setEditing(true)}>{t("common.edit")}</Button>
            {isActive && account.next_payment_date && account.billing_period_months > 0 && (
              <Button variant="ok" onClick={() => setMarkingPaid(true)}>{t("accounts.markPaid")}</Button>
            )}
          </div>
          <div className="detail-actions-group">
            {isActive ? (
              <Button variant="warn" onClick={() => setStatus("inactive")}>{t("accounts.deactivate")}</Button>
            ) : (
              <>
                <Button variant="ok" onClick={() => setStatus("active")}>{t("accounts.activate")}</Button>
                <Button variant="danger" onClick={remove}>{t("common.delete")}</Button>
              </>
            )}
          </div>
        </div>
      </div>

      <AccountTOTP accountId={account.id} configured={account.has_totp ?? false} onChanged={onChanged} />

      {editing && (
        <AccountForm
          existing={account}
          categories={categories}
          onSaved={() => { setEditing(false); onChanged(); }}
          onCancel={() => setEditing(false)}
        />
      )}
      {markingPaid && (
        <PaymentForm
          accountId={account.id}
          existing={null}
          markPaid
          defaults={{ amount: account.amount }}
          onSaved={() => { setMarkingPaid(false); toast(t("accounts.markedPaid")); onChanged(); }}
          onCancel={() => setMarkingPaid(false)}
        />
      )}
    </div>
  );
}

// --- Small display pieces --------------------------------------------------

function PaymentCell({ date }: { date?: string }) {
  const status = paymentStatus(date);
  if (status === "none") return <span className="muted">—</span>;
  const d = date!.slice(0, 10);
  // Color conveys overdue / due-soon; no extra suffix text needed.
  return <span className={`pay pay-${status}`}>{d}</span>;
}

export function DueSoonBanner({ accounts }: { accounts: AccountListItem[] }) {
  const { t } = useTranslation();
  const flagged = accounts
    .filter((a) => {
      const s = paymentStatus(a.next_payment_date);
      return s === "overdue" || s === "soon";
    })
    .sort((a, b) => daysUntil(a.next_payment_date!) - daysUntil(b.next_payment_date!));
  if (flagged.length === 0) return null;
  return (
    <div className="banner">
      <strong>{t("accounts.paymentsNeedAttention", { count: flagged.length })}</strong>{" "}
      {flagged.map((a) => (
        <span key={a.id} className={`chip pay-${paymentStatus(a.next_payment_date)}`}>
          {a.service} · {a.next_payment_date}
        </span>
      ))}
    </div>
  );
}

export function RotationBanner({ accounts }: { accounts: AccountListItem[] }) {
  const { t } = useTranslation();
  const flagged = accounts.filter((a) => {
    const s = rotationStatus(a.password_changed_at, a.rotate_every_days);
    return s === "overdue" || s === "soon";
  });
  if (flagged.length === 0) return null;
  return (
    <div className="banner rotate">
      <strong>{t("accounts.passwordsDueRotation", { count: flagged.length })}</strong>{" "}
      {flagged.map((a) => (
        <span key={a.id} className={`chip pay-${rotationStatus(a.password_changed_at, a.rotate_every_days)}`}>
          {a.service}
        </span>
      ))}
    </div>
  );
}

// --- Create/edit form (modal) ----------------------------------------------

export function AccountForm({
  existing,
  categories,
  defaultCategoryId,
  onSaved,
  onCancel,
}: {
  existing: AccountListItem | null;
  categories: Category[];
  defaultCategoryId?: string;
  onSaved: () => void;
  onCancel: () => void;
}) {
  const editing = existing !== null;
  const { t } = useTranslation();
  const { arabicEnabled, lang } = useLayout();

  const [form, setForm] = useState<NewAccount>({
    category_id: existing?.category_id ?? defaultCategoryId ?? "",
    service: existing?.service ?? "",
    website: existing?.website ?? "",
    login: existing?.login ?? "",
    password: "",
    billing_period_months: existing?.billing_period_months ?? 0,
    amount: existing?.amount,
    next_payment_date: existing?.next_payment_date?.slice(0, 10) ?? "",
    rotate_every_days: existing?.rotate_every_days,
    notes: existing?.notes ?? "",
    seats: existing?.seats ?? [],
    seats_ar: existing?.seats_ar ?? [],
  });
  const [error, setError] = useState<string | null>(null);
  const [people, setPeople] = useState<Person[]>([]);
  // Billing period as free text so you can clear it and type "0" freely.
  const [billing, setBilling] = useState(existing ? String(existing.billing_period_months) : "");
  const billingNum = billing === "" ? 0 : Number(billing);

  useEffect(() => {
    listPeople().then(setPeople).catch(() => {});
  }, []);

  // One searchable list of people; each option is searchable by BOTH the English
  // and Arabic name, shows both, and stores the English name as the seat value.
  const seatOptions = people.map((p) => ({
    value: p.name,
    label: !arabicEnabled || !p.name_ar ? p.name : lang === "ar" ? `${p.name_ar} · ${p.name}` : `${p.name} · ${p.name_ar}`,
    search: `${p.name} ${p.name_ar ?? ""}`,
  }));

  // Picking/typing a seat: match a known person (by English or Arabic name) and
  // store both names; free text stores just what was typed.
  function onSeat(i: number, v: string) {
    // Only match a person for non-empty input — otherwise "" matches the first
    // person whose (name_ar ?? "") is "", refilling a field you just cleared.
    const p = v ? people.find((x) => x.name === v || x.name_ar === v) : undefined;
    setForm((f) => {
      const seats = [...(f.seats ?? [])];
      const seatsAr = [...(f.seats_ar ?? [])];
      while (seatsAr.length < seats.length) seatsAr.push(null);
      seats[i] = p ? p.name : v;
      // Free text has no Arabic pairing — clear any stale one so a cleared field
      // doesn't fall back to a previous person's name via seatDisplay().
      seatsAr[i] = p ? (p.name_ar ?? null) : null;
      return { ...f, seats, seats_ar: seatsAr };
    });
  }

  // Display value for a seat: prefer the English name; fall back to Arabic.
  const seatDisplay = (i: number) => (form.seats ?? [])[i] || (form.seats_ar ?? [])[i] || "";

  function set<K extends keyof NewAccount>(key: K, value: NewAccount[K]) {
    setForm((f) => ({ ...f, [key]: value }));
  }


  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    try {
      const payload: NewAccount = { ...form, billing_period_months: billingNum };
      if (!payload.website) delete payload.website;
      if (!payload.login) delete payload.login;
      if (!payload.next_payment_date) delete payload.next_payment_date;
      if (!payload.category_id) payload.category_id = null;
      if (editing && !payload.password) delete (payload as Partial<NewAccount>).password;

      if (editing) await updateAccount(existing!.id, payload);
      else await createAccount(payload);
      onSaved();
    } catch (err) {
      setError((err as Error).message);
    }
  }

  return (
    <Modal wide onClose={onCancel}>
    <form className="add-form" onSubmit={handleSubmit}>
      <h1 style={{ fontSize: 16 }}>{editing ? t("accounts.editPrefix", { service: existing!.service }) : t("accounts.newAccount")}</h1>
      <div className="grid">
        <Field label={t("accounts.category")}>
          <select value={form.category_id ?? ""} onChange={(e) => set("category_id", e.target.value)}>
            <option value="">{t("accounts.uncategorized")}</option>
            {categories.map((c) => (
              <option key={c.id} value={c.id}>{lang === "ar" && c.name_ar ? c.name_ar : c.name}</option>
            ))}
          </select>
        </Field>
        <Field label={t("accounts.service")}>
          <input value={form.service} onChange={(e) => set("service", e.target.value)} required />
        </Field>
        <Field label={t("accounts.websiteOptional")}>
          <input value={form.website} onChange={(e) => set("website", e.target.value)} placeholder={t("accounts.websitePlaceholder")} />
        </Field>
        <Field label={t("accounts.loginEmail")}>
          <input value={form.login} onChange={(e) => set("login", e.target.value)} />
        </Field>
        <Field label={t("accounts.billingPeriod")} hint={billingLabel(t, billingNum, form.amount)}>
          <input
            type="text"
            inputMode="numeric"
            placeholder="0"
            value={billing}
            onChange={(e) => setBilling(e.target.value.replace(/[^0-9]/g, ""))}
          />
        </Field>
        <Field label={t("accounts.amountOptional")}>
          <input
            type="number"
            min={0}
            step="0.01"
            placeholder={t("accounts.amountPlaceholder")}
            value={form.amount ?? ""}
            onChange={(e) => set("amount", e.target.value ? Number(e.target.value) : null)}
          />
        </Field>
        <Field label={t("accounts.nextPaymentShort")}>
          <input type="date" value={form.next_payment_date} onChange={(e) => set("next_payment_date", e.target.value)} />
        </Field>
        <Field label={t("accounts.rotateEvery")}>
          <input
            type="number"
            min={1}
            placeholder={t("accounts.rotatePlaceholder")}
            value={form.rotate_every_days ?? ""}
            onChange={(e) => set("rotate_every_days", e.target.value ? Number(e.target.value) : undefined)}
          />
        </Field>
        <PasswordField
          label={<>{t("accounts.password")} {editing && <span className="muted">{t("accounts.passwordKeep")}</span>}</>}
          value={form.password}
          onChange={(v) => set("password", v)}
          required={!editing}
          full
        />
        <Field label={`${t("accounts.seats")}${form.seats?.length ? ` (${form.seats.length})` : ""} — ${t("common.optional")}`} full>
          <div className="seats">
            {(form.seats ?? []).map((_, i) => (
              <div className="seat-row" key={i}>
                <Combobox
                  value={seatDisplay(i)}
                  options={seatOptions}
                  placeholder={t("accounts.seatPick", { n: i + 1 })}
                  onChange={(v) => onSeat(i, v)}
                />
                <IconButton danger title={t("accounts.removeSeat")} onClick={() => {
                  set("seats", (form.seats ?? []).filter((_, j) => j !== i));
                  set("seats_ar", (form.seats_ar ?? []).filter((_, j) => j !== i));
                }}>
                  <FaTrash />
                </IconButton>
              </div>
            ))}
            <Button type="button" variant="ghost" onClick={() => { set("seats", [...(form.seats ?? []), ""]); set("seats_ar", [...(form.seats_ar ?? []), null]); }}>
              {t("accounts.addSeat")}
            </Button>
          </div>
        </Field>
        <Field label={t("accounts.notes")} full>
          <textarea value={form.notes ?? ""} onChange={(e) => set("notes", e.target.value)} rows={3} placeholder={t("accounts.notesPlaceholder")} />
        </Field>
      </div>
      {error && <p className="error">{error}</p>}
      <div className="form-actions">
        <Button type="submit">{editing ? t("accounts.saveChanges") : t("accounts.saveAccount")}</Button>
        <Button type="button" variant="ghost" onClick={onCancel}>{t("common.cancel")}</Button>
      </div>
    </form>
    </Modal>
  );
}
