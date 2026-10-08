import { useEffect, useState } from "react";
import { Outlet, useLocation, useNavigate, useOutletContext } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { listAccounts, listCategories, getSettings, listCurrencies, logout, exportAccounts, getProfile, type AccountListItem, type Category, type Currency } from "../api";
import { Sidebar } from "./Sidebar";
import { UpdateBanner } from "./UpdateBanner";
import { errorAlert, toast } from "../alerts";
import { applyLanguage, availableLanguages, storedLang, type Lang } from "../i18n";
import { theme } from "../theme";
import { FaBars } from "react-icons/fa";
import Swal from "sweetalert2";

// Shared data passed to every page via the router Outlet.
export interface LayoutCtx {
  accounts: AccountListItem[];
  categories: Category[];
  currencies: Currency[];
  weekStart: number;
  arabicEnabled: boolean; // whether Arabic support is activated in Configuration
  clipboardClear: number; // seconds before a copied secret is wiped (0 = off)
  lang: string;           // current UI language ("en" | "ar")
  displayName: string;    // signed-in admin's name (localized), for greetings
  refresh: () => void;
  refreshCategories: () => void;
  reloadSettings: () => void;
}

// useLayout gives pages access to the shared accounts/categories + refreshers.
export function useLayout() {
  return useOutletContext<LayoutCtx>();
}

export function Layout() {
  const [accounts, setAccounts] = useState<AccountListItem[]>([]);
  const [categories, setCategories] = useState<Category[]>([]);
  const [weekStart, setWeekStart] = useState(0);
  const [currencies, setCurrencies] = useState<Currency[]>([]);
  const [arabicEnabled, setArabicEnabled] = useState(false);
  const [clipboardClear, setClipboardClear] = useState(20);
  const [orgName, setOrgName] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [menuOpen, setMenuOpen] = useState(false); // mobile drawer
  const { t, i18n } = useTranslation();

  const navigate = useNavigate();
  const location = useLocation();

  // Close the mobile drawer whenever the route changes (i.e. after navigating).
  useEffect(() => { setMenuOpen(false); }, [location.pathname]);

  // The signed-in admin's name, localized, for the greeting in the top bar.
  useEffect(() => {
    getProfile()
      .then((p) => setDisplayName(i18n.language === "ar" && p.name_ar ? p.name_ar : p.name))
      .catch(() => {});
  }, [i18n.language]);

  async function refresh() {
    try { setAccounts(await listAccounts()); } catch { /* pages surface their own errors */ }
  }
  async function refreshCategories() {
    try { setCategories(await listCategories()); } catch { /* ignore */ }
  }
  async function reloadSettings() {
    try {
      const s = await getSettings();
      setWeekStart(parseInt(s.week_start ?? "0", 10) || 0);
      const enabled = s.lang_arabic_enabled === "1";
      setArabicEnabled(enabled);
      const cc = parseInt(s.clipboard_clear_seconds ?? "20", 10);
      setClipboardClear(Number.isFinite(cc) && cc >= 0 ? cc : 20);
      setOrgName(s.org_name ?? "");
      // If Arabic was turned off but this user had it selected, fall back to English.
      const allowed = availableLanguages(enabled);
      if (!allowed.includes(storedLang())) applyLanguage("en");
    } catch { /* ignore */ }
    try { setCurrencies(await listCurrencies()); } catch { /* ignore */ }
  }

  useEffect(() => {
    refresh();
    refreshCategories();
    reloadSettings();
  }, []);

  // Which sidebar item is highlighted, derived from the URL.
  const path = location.pathname;
  const activeKey =
    path === "/" ? "home" :
    path.startsWith("/accounts") ? "all" :
    path.startsWith("/c/") ? path.split("/")[2] :
    path.startsWith("/health") ? "health" :
    path.startsWith("/authenticator") ? "authenticator" :
    path.startsWith("/finance") ? "finance" :
    path.startsWith("/settings") ? "settings" :
    "";

  function handleLogout() {
    logout();
    navigate("/login");
  }

  async function handleExport() {
    const r = await Swal.fire({
      background: theme.surface,
      color: theme.text,
      icon: "question",
      title: t("export.title"),
      html: `<p style="margin:0 0 14px">${t("export.text")}</p>
        <div class="inline-check">
          <input type="checkbox" id="exp-attach">
          <span>${t("export.includeAttachments")}</span>
        </div>`,
      showCancelButton: true,
      confirmButtonText: t("export.exportBtn"),
      cancelButtonText: t("common.cancel"),
      confirmButtonColor: theme.accent,
      cancelButtonColor: theme.borderStrong,
      preConfirm: () => ({ attach: (document.getElementById("exp-attach") as HTMLInputElement | null)?.checked ?? false }),
    });
    if (!r.isConfirmed) return;
    try {
      await exportAccounts(r.value?.attach === true);
      toast(t("export.done"));
    } catch (err) {
      errorAlert((err as Error).message);
    }
  }

  function changeLang(lang: Lang) { applyLanguage(lang); }

  const ctx: LayoutCtx = { accounts, categories, currencies, weekStart, arabicEnabled, clipboardClear, lang: i18n.language, displayName, refresh, refreshCategories, reloadSettings };

  return (
    <div className="layout">
      <div className="mobile-topbar">
        <button className="hamburger" onClick={() => setMenuOpen(true)} aria-label={t("sidebar.menu")}>
          <FaBars />
        </button>
        <span className="mobile-title">{displayName ? t("home.greeting", { name: displayName }) : (orgName || t("sidebar.appName"))}</span>
      </div>
      {menuOpen && <div className="sidebar-backdrop" onClick={() => setMenuOpen(false)} />}
      <Sidebar
        open={menuOpen}
        categories={categories}
        activeKey={activeKey}
        onHome={() => navigate("/")}
        onSelectCategory={(id) => navigate(id === "all" ? "/accounts" : `/c/${id}`)}
        onShowHealth={() => navigate("/health")}
        onShowAuthenticator={() => navigate("/authenticator")}
        onShowFinance={() => navigate("/finance")}
        onShowSettings={() => navigate("/settings")}
        onExport={handleExport}
        onLogout={handleLogout}
        arabicEnabled={arabicEnabled}
        lang={i18n.language}
        onChangeLang={changeLang}
        orgName={orgName}
      />
      <div className="page">
        <UpdateBanner />
        <Outlet context={ctx} />
      </div>
    </div>
  );
}
