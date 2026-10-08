import {
  FaCog, FaLayerGroup, FaHeartbeat, FaSignOutAlt, FaHome, FaChartLine, FaDownload, FaGlobe, FaKey,
} from "react-icons/fa";
import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Icon } from "../icons";
import { availableLanguages, type Lang } from "../i18n";
import type { Category } from "../api";

// activeKey is the highlighted item: "all", a category id, "health", "settings",
// or "" (nothing, e.g. on a detail page).
export function Sidebar({
  open = false,
  categories,
  activeKey,
  onHome,
  onSelectCategory,
  onShowHealth,
  onShowAuthenticator,
  onShowFinance,
  onShowSettings,
  onExport,
  onLogout,
  arabicEnabled,
  lang,
  onChangeLang,
  orgName,
}: {
  open?: boolean; // mobile drawer open state
  categories: Category[];
  activeKey: string;
  onHome: () => void;
  onSelectCategory: (id: string) => void; // "all" or a category id
  onShowHealth: () => void;
  onShowAuthenticator: () => void;
  onShowFinance: () => void;
  onShowSettings: () => void;
  onExport: () => void;
  onLogout: () => void;
  arabicEnabled: boolean;
  lang: string;
  onChangeLang: (lang: Lang) => void;
  orgName?: string;
}) {
  const { t } = useTranslation();
  const isAr = lang === "ar";
  // Show a category's Arabic name when viewing in Arabic and one exists.
  const catName = (c: Category) => (isAr && c.name_ar ? c.name_ar : c.name);
  const langs = availableLanguages(arabicEnabled);

  // Top/bottom fades on the category list, shown only when there's more to scroll.
  const catsRef = useRef<HTMLDivElement>(null);
  const [fade, setFade] = useState({ top: false, bottom: false });
  function updateFade() {
    const el = catsRef.current;
    if (!el) return;
    setFade({
      top: el.scrollTop > 4,
      bottom: el.scrollTop + el.clientHeight < el.scrollHeight - 4,
    });
  }
  useEffect(() => {
    updateFade();
    window.addEventListener("resize", updateFade);
    return () => window.removeEventListener("resize", updateFade);
  }, [categories]);

  return (
    <aside className={`sidebar ${open ? "open" : ""}`}>
      <div className="sidebar-head">
        <span className="sidebar-title">
          {orgName && <span className="sidebar-org">{orgName}</span>}
          {t("sidebar.appName")}
        </span>
      </div>

      <nav>
        <button
          className={`side-item ${activeKey === "home" ? "active" : ""}`}
          title={t("sidebar.home")}
          onClick={onHome}
        >
          <FaHome /> <span>{t("sidebar.home")}</span>
        </button>

        <button
          className={`side-item ${activeKey === "all" ? "active" : ""}`}
          title={t("sidebar.allAccounts")}
          onClick={() => onSelectCategory("all")}
        >
          <FaLayerGroup /> <span>{t("sidebar.allAccounts")}</span>
        </button>

        <div className={`side-cat-wrap ${fade.top ? "fade-top" : ""} ${fade.bottom ? "fade-bottom" : ""}`}>
          <div className="side-categories" ref={catsRef} onScroll={updateFade}>
            {categories.map((c) => (
              <button
                key={c.id}
                className={`side-item ${activeKey === c.id ? "active" : ""}`}
                title={catName(c)}
                onClick={() => onSelectCategory(c.id)}
              >
                <Icon name={c.icon} /> <span>{catName(c)}</span>
              </button>
            ))}

            {categories.length === 0 && (
              <p className="muted" style={{ padding: "8px 10px" }}>{t("sidebar.noCategories")}</p>
            )}
          </div>
        </div>
      </nav>

      <div className="sidebar-footer">
        <button className={`side-item ${activeKey === "health" ? "active" : ""}`} title={t("sidebar.health")} onClick={onShowHealth}>
          <FaHeartbeat /> <span>{t("sidebar.health")}</span>
        </button>
        <button className={`side-item ${activeKey === "authenticator" ? "active" : ""}`} title={t("sidebar.authenticator")} onClick={onShowAuthenticator}>
          <FaKey /> <span>{t("sidebar.authenticator")}</span>
        </button>
        <button className={`side-item ${activeKey === "finance" ? "active" : ""}`} title={t("sidebar.finance")} onClick={onShowFinance}>
          <FaChartLine /> <span>{t("sidebar.finance")}</span>
        </button>
        <button className={`side-item ${activeKey === "settings" ? "active" : ""}`} title={t("sidebar.settings")} onClick={onShowSettings}>
          <FaCog /> <span>{t("sidebar.settings")}</span>
        </button>
        <button className="side-item" title={t("sidebar.export")} onClick={onExport}>
          <FaDownload /> <span>{t("sidebar.export")}</span>
        </button>

        {/* Language switcher — only when Arabic support is activated */}
        {langs.length > 1 && (
          <div className="side-lang">
            <FaGlobe />
            <select dir="ltr" value={lang} onChange={(e) => onChangeLang(e.target.value as Lang)} title={t("sidebar.language")}>
              {langs.map((l) => <option key={l} value={l}>{t(`lang.${l}`)}</option>)}
            </select>
          </div>
        )}

        <button className="side-item" title={t("sidebar.signOut")} onClick={onLogout}>
          <FaSignOutAlt /> <span>{t("sidebar.signOut")}</span>
        </button>
      </div>
    </aside>
  );
}
