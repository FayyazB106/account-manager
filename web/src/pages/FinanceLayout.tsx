import { NavLink, Outlet } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { useLayout } from "../components/Layout";
import { Subnav } from "../components/Subnav";

export function FinanceLayout() {
  const ctx = useLayout();
  const { t } = useTranslation();
  return (
    <div className="settings-wrap">
      <Subnav title={t("sidebar.finance")}>
        <NavLink to="/finance/accounts" className={({ isActive }) => (isActive ? "active" : "")}>{t("finance.accounts")}</NavLink>
        <NavLink to="/finance/calendar" className={({ isActive }) => (isActive ? "active" : "")}>{t("finance.calendar")}</NavLink>
        <NavLink to="/finance/graph" className={({ isActive }) => (isActive ? "active" : "")}>{t("finance.graph")}</NavLink>
        <NavLink to="/finance/payments" className={({ isActive }) => (isActive ? "active" : "")}>{t("finance.payments")}</NavLink>
      </Subnav>
      <div className="settings-content">
        <Outlet context={ctx} />
      </div>
    </div>
  );
}
