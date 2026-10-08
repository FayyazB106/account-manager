import { NavLink, Outlet, useNavigate, useOutletContext, useParams } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { useLayout, type LayoutCtx } from "../components/Layout";
import { Subnav } from "../components/Subnav";
import { AccountGeneral } from "../account-ui";
import { PasswordHistory } from "../components/PasswordHistory";
import { PaymentHistory } from "../components/PaymentHistory";
import { Attachments } from "../components/Attachments";
import type { AccountListItem } from "../api";

// The account detail page is a sidebar layout (like Settings/Finance): a General
// tab (info + 2FA) plus Password history, Payments, and Attachments tabs.

// AccountOutletCtx is the shared layout context extended with the resolved account,
// passed down to each tab via the Outlet.
export interface AccountOutletCtx extends LayoutCtx {
  account: AccountListItem;
}
export function useAccountCtx() {
  return useOutletContext<AccountOutletCtx>();
}

export function AccountLayout() {
  const { id } = useParams();
  const { t } = useTranslation();
  const navigate = useNavigate();
  const ctx = useLayout();
  const account = ctx.accounts.find((a) => a.id === id) ?? null;

  if (!account) {
    return (
      <>
        <button className="link back" onClick={() => navigate("/")}>‹ {t("common.back")}</button>
        <p className="muted">{t("accounts.notFound")}</p>
      </>
    );
  }

  return (
    <div className="account-detail">
      <button className="link back" onClick={() => navigate(-1)}>‹ {t("common.back")}</button>
      <div className="settings-wrap">
        <Subnav title={account.service}>
          {/* replace: switching tabs shouldn't pile up history, so Back returns to
              the list/table you came from rather than walking back through tabs. */}
          <NavLink end replace to={`/account/${account.id}/general`} className={({ isActive }) => (isActive ? "active" : "")}>{t("accountNav.general")}</NavLink>
          <NavLink replace to={`/account/${account.id}/passwords`} className={({ isActive }) => (isActive ? "active" : "")}>{t("accountNav.passwords")}</NavLink>
          <NavLink replace to={`/account/${account.id}/payments`} className={({ isActive }) => (isActive ? "active" : "")}>{t("accountNav.payments")}</NavLink>
          <NavLink replace to={`/account/${account.id}/attachments`} className={({ isActive }) => (isActive ? "active" : "")}>{t("accountNav.attachments")}</NavLink>
        </Subnav>
        <div className="settings-content">
          <Outlet context={{ ...ctx, account } satisfies AccountOutletCtx} />
        </div>
      </div>
    </div>
  );
}

// --- Tabs ------------------------------------------------------------------

export function AccountGeneralTab() {
  const navigate = useNavigate();
  const { account, categories, currencies, refresh } = useAccountCtx();
  return (
    <AccountGeneral
      account={account}
      categories={categories}
      currencies={currencies}
      onBack={() => navigate("/")}
      onChanged={refresh}
    />
  );
}

export function AccountPasswordsTab() {
  const { account } = useAccountCtx();
  return <PasswordHistory accountId={account.id} />;
}

export function AccountPaymentsTab() {
  const { account } = useAccountCtx();
  return <PaymentHistory accountId={account.id} />;
}

export function AccountAttachmentsTab() {
  const { account } = useAccountCtx();
  return (
    <div className="card">
      <Attachments accountId={account.id} />
    </div>
  );
}
