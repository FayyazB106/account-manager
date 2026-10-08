import { NavLink, Outlet } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { useLayout } from "../components/Layout";
import { Subnav } from "../components/Subnav";

// SettingsLayout gives the Settings area its own static text sub-navigation and
// forwards the shared data context to its sub-pages.
export function SettingsLayout() {
  const ctx = useLayout();
  const { t, i18n } = useTranslation();

  // Sub-nav items sorted alphabetically by their (localized) label.
  const items = [
    { to: "/settings/categories", label: t("settingsNav.categories") },
    { to: "/settings/admin", label: t("settingsNav.admin") },
    { to: "/settings/users", label: t("settingsNav.users") },
    { to: "/settings/configuration", label: t("settingsNav.configuration") },
    { to: "/settings/deployment", label: t("settingsNav.deployment") },
    { to: "/settings/audit", label: t("settingsNav.audit") },
  ].sort((a, b) => a.label.localeCompare(b.label, i18n.language));

  return (
    <div className="settings-wrap">
      <Subnav title={t("settingsNav.title")}>
        {items.map((it) => (
          <NavLink key={it.to} to={it.to} className={({ isActive }) => (isActive ? "active" : "")}>{it.label}</NavLink>
        ))}
      </Subnav>

      <div className="settings-content">
        <Outlet context={ctx} />
      </div>
    </div>
  );
}
