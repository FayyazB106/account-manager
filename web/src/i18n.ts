import i18n from "i18next";
import { initReactI18next } from "react-i18next";
import en from "./locales/en.json";
import ar from "./locales/ar.json";

// Languages the code ships with. Arabic is only *selectable* when an admin turns
// it on (the `lang_arabic_enabled` setting) — see availableLanguages().
export const LANGUAGES = ["en", "ar"] as const;
export type Lang = (typeof LANGUAGES)[number];

export const RTL_LANGS: Lang[] = ["ar"];
const STORAGE_KEY = "am_lang";

export function isRtl(lang: string): boolean {
  return RTL_LANGS.includes(lang as Lang);
}

// The per-user stored preference (may be a language that is not currently
// enabled — callers resolve that against availableLanguages()).
export function storedLang(): Lang {
  try {
    const v = localStorage.getItem(STORAGE_KEY);
    if (v && (LANGUAGES as readonly string[]).includes(v)) return v as Lang;
  } catch { /* ignore */ }
  return "en";
}

// Which languages a user may actually pick, given the activation setting.
// English is always available; Arabic only when enabled.
export function availableLanguages(arabicEnabled: boolean): Lang[] {
  return arabicEnabled ? ["en", "ar"] : ["en"];
}

// Apply a language app-wide: switch i18next, persist it, and set the document
// direction + lang attributes so RTL layout kicks in.
export function applyLanguage(lang: Lang) {
  i18n.changeLanguage(lang);
  try { localStorage.setItem(STORAGE_KEY, lang); } catch { /* ignore */ }
  const dir = isRtl(lang) ? "rtl" : "ltr";
  document.documentElement.setAttribute("dir", dir);
  document.documentElement.setAttribute("lang", lang);
}

i18n.use(initReactI18next).init({
  resources: {
    en: { translation: en },
    ar: { translation: ar },
  },
  lng: storedLang(),
  fallbackLng: "en",
  interpolation: {
    escapeValue: false,
  },
});

// Render numbers in the active language's digits (e.g. Arabic-Indic ٩) via the
// v26 formatter service, used as {{value, number}} in the locale strings.
i18n.services.formatter?.add("number", (value: unknown, lng?: string) =>
  typeof value === "number" ? value.toLocaleString(lng === "ar" ? "ar-EG" : lng) : String(value),
);

// Reflect the initial stored language in the document direction on boot.
applyLanguage(storedLang());

export default i18n;
