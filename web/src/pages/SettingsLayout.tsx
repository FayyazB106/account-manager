import { useEffect } from "react";
import { NavLink, Outlet, useLocation } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { useLayout } from "../components/Layout";
import { Subnav } from "../components/Subnav";
import { SettingsSearchBox, SettingsSearchResults, useSettingsSearch } from "../components/SettingsSearch";

// SettingsLayout gives the Settings area its own static text sub-navigation and
// forwards the shared data context to its sub-pages.
export function SettingsLayout() {
  const ctx = useLayout();
  const { t, i18n } = useTranslation();
  const search = useSettingsSearch();

  // Sub-nav items sorted alphabetically by their (localized) label.
  const items = [
    { to: "/settings/categories", label: t("settingsNav.categories") },
    { to: "/settings/admin", label: t("settingsNav.admin") },
    { to: "/settings/users", label: t("settingsNav.users") },
    { to: "/settings/configuration", label: t("settingsNav.configuration") },
    { to: "/settings/deployment", label: t("settingsNav.deployment") },
    { to: "/settings/email", label: t("settingsNav.email") },
    { to: "/settings/audit", label: t("settingsNav.audit") },
  ].sort((a, b) => a.label.localeCompare(b.label, i18n.language));

  // Arriving from the settings search (…/page#s-section): scroll to that section and
  // flash it. The page may still be mounting, so look for it for a moment.
  const location = useLocation();
  useEffect(() => {
    const id = location.hash.slice(1);
    if (!id) return;
    let tries = 0;
    let timer: number | undefined;
    const find = () => {
      const el = document.getElementById(id);
      if (el) {
        el.scrollIntoView({ behavior: "smooth", block: "start" });
        el.classList.add("search-hit");
        window.setTimeout(() => el.classList.remove("search-hit"), 2000);
      } else if (++tries < 20) {
        timer = window.setTimeout(find, 50);
      }
    };
    find();
    return () => window.clearTimeout(timer);
  }, [location.pathname, location.hash, location.key]);

  return (
    <div className="settings-wrap">
      {/* Phone layout: the search gets its own row above the tab strip (hidden on desktop). */}
      <div className="settings-search-top">
        <div className="subnav-search"><SettingsSearchBox s={search} /></div>
        {search.searching && <SettingsSearchResults s={search} />}
      </div>

      <Subnav title={t("settingsNav.title")} className={search.searching ? "is-searching" : ""}>
        {/* Desktop layout: the box sits under the title and the results replace the links. */}
        <div className="subnav-search"><SettingsSearchBox s={search} /></div>
        {search.searching ? (
          <SettingsSearchResults s={search} />
        ) : (
          items.map((it) => (
            <NavLink key={it.to} to={it.to} className={({ isActive }) => (isActive ? "active" : "")}>{it.label}</NavLink>
          ))
        )}
      </Subnav>

      <div className="settings-content">
        <Outlet context={ctx} />
      </div>
    </div>
  );
}
