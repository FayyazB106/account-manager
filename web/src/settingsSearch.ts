// Settings search: a curated index of every Settings section and individual
// setting, matched in BOTH languages at once (an English word finds the item from the
// Arabic UI and vice versa), plus a few synonyms per entry.
//
// Why curated rather than "search all the page text": it points at real,
// jump-to-able sections, it needs no rendering of every page, and it can't go stale
// or pick up noise like hints and dialog text. Labels reuse the existing
// translation keys, so a renamed heading is renamed in the search too; only the
// synonyms live in `settingsSearch.kw.<id>` in the locale files.

import type { i18n as I18n } from "i18next";

export interface SettingsEntry {
  id: string;       // also the key of its synonyms: settingsSearch.kw.<id>
  to: string;       // the Settings page
  anchor?: string;  // element id to scroll to (and flash) on that page
  label: string;    // i18n key of what this is called: a heading or a setting
  page: string;     // i18n key of the page it lives on (settingsNav.*)
}

const A = "/settings/admin";
const C = "/settings/configuration";
const D = "/settings/deployment";
const E = "/settings/email";

export const SETTINGS_ENTRIES: SettingsEntry[] = [
  // Admin
  { id: "profile", to: A, anchor: "s-profile", label: "admin.profile", page: "settingsNav.admin" },
  { id: "reset-password", to: A, anchor: "s-reset-password", label: "admin.resetPassword", page: "settingsNav.admin" },
  { id: "two-factor", to: A, anchor: "s-two-factor", label: "totp.title", page: "settingsNav.admin" },
  { id: "send-emails", to: A, anchor: "s-send-emails", label: "report.title", page: "settingsNav.admin" },
  { id: "email-monthly", to: A, anchor: "s-send-emails", label: "report.monthlyName", page: "settingsNav.admin" },
  { id: "email-upcoming", to: A, anchor: "s-send-emails", label: "report.upcomingName", page: "settingsNav.admin" },
  { id: "email-reminders", to: A, anchor: "s-send-emails", label: "report.reminderName", page: "settingsNav.admin" },
  { id: "email-rotation", to: A, anchor: "s-send-emails", label: "report.rotationName", page: "settingsNav.admin" },
  { id: "email-breach", to: A, anchor: "s-send-emails", label: "report.breachName", page: "settingsNav.admin" },
  { id: "appearance", to: A, anchor: "s-appearance", label: "config.appearance", page: "settingsNav.admin" },
  { id: "organization", to: A, anchor: "s-organization", label: "admin.organization", page: "settingsNav.admin" },
  { id: "admins", to: A, anchor: "s-admins", label: "admin.admins", page: "settingsNav.admin" },
  // Categories
  { id: "categories", to: "/settings/categories", label: "categories.title", page: "settingsNav.categories" },
  // Configuration
  { id: "week-start", to: C, anchor: "s-general", label: "config.weekStartsOn", page: "settingsNav.configuration" },
  { id: "clipboard", to: C, anchor: "s-general", label: "config.clipboardClear", page: "settingsNav.configuration" },
  { id: "languages", to: C, anchor: "s-languages", label: "config.languages", page: "settingsNav.configuration" },
  { id: "updates", to: C, anchor: "s-updates", label: "update.settingsTitle", page: "settingsNav.configuration" },
  { id: "breach", to: C, anchor: "s-breach", label: "breach.title", page: "settingsNav.configuration" },
  { id: "backups", to: C, anchor: "s-backups", label: "backup.title", page: "settingsNav.configuration" },
  { id: "currencies", to: C, anchor: "s-currencies", label: "config.currencies", page: "settingsNav.configuration" },
  // Deployment
  { id: "deployment", to: D, label: "deploy.title", page: "settingsNav.deployment" },
  { id: "deploy-port", to: D, label: "deploy.port", page: "settingsNav.deployment" },
  { id: "deploy-network", to: D, label: "deploy.network", page: "settingsNav.deployment" },
  { id: "deploy-url", to: D, label: "deploy.publicUrl", page: "settingsNav.deployment" },
  // Email
  { id: "email", to: E, label: "email.title", page: "settingsNav.email" },
  { id: "smtp-host", to: E, label: "email.host", page: "settingsNav.email" },
  { id: "smtp-port", to: E, label: "email.port", page: "settingsNav.email" },
  { id: "smtp-security", to: E, label: "email.security", page: "settingsNav.email" },
  { id: "smtp-username", to: E, label: "email.username", page: "settingsNav.email" },
  { id: "smtp-password", to: E, label: "email.password", page: "settingsNav.email" },
  { id: "smtp-from", to: E, label: "email.from", page: "settingsNav.email" },
  // Logs, Users
  { id: "audit", to: "/settings/audit", label: "audit.title", page: "settingsNav.audit" },
  { id: "users", to: "/settings/users", label: "users.title", page: "settingsNav.users" },
  { id: "users-add", to: "/settings/users", label: "users.add", page: "settingsNav.users" },
];

// normalize makes matching forgiving: case-insensitive, accents and Arabic
// diacritics/tatweel dropped, and the Arabic letters people type interchangeably
// (أ إ آ ا, ى ي, ة ه) treated as one.
export function normalize(s: string): string {
  return s
    .toLowerCase()
    .normalize("NFKD")
    .replace(/[̀-ًͯ-ٰٟـ]/g, "")
    .replace(/[أإآٱ]/g, "ا")
    .replace(/ى/g, "ي")
    .replace(/ة/g, "ه");
}

interface Indexed {
  entry: SettingsEntry;
  title: string;    // the entry's name in every searched language
  page: string;     // the page it lives on, in every searched language
  keywords: string; // its synonyms, in every searched language
}

// buildIndex reads each entry's wording in every searched language. It uses
// fixed-language translators, so the UI language doesn't have to change.
export function buildIndex(i18n: I18n, langs: readonly string[]): Indexed[] {
  return SETTINGS_ENTRIES.map((entry) => {
    const titles: string[] = [];
    const pages: string[] = [];
    const kws: string[] = [];
    for (const lng of langs) {
      const t = i18n.getFixedT(lng);
      const kwKey = `settingsSearch.kw.${entry.id}`;
      titles.push(t(entry.label));
      pages.push(t(entry.page));
      kws.push(i18n.exists(kwKey, { lng }) ? t(kwKey) : "");
    }
    return {
      entry,
      title: normalize(titles.join(" ")),
      page: normalize(pages.join(" ")),
      keywords: normalize(kws.join(" ")),
    };
  });
}

// startsWord: does `term` begin a word of `text`? (Arabic words often start with the
// article "ال", so "نسخ" also counts as starting "النسخ".)
function startsWord(text: string, term: string): boolean {
  const padded = " " + text;
  return padded.includes(" " + term) || padded.includes(" ال" + term);
}

// score rates how well one word matches an entry — its own name best (and the start
// of a word beats the middle of one, so "port" prefers "Port" over "reports"), then
// the page it is on, then its synonyms. 0 = no match.
function score(it: Indexed, term: string): number {
  if (it.title.includes(term)) return startsWord(it.title, term) ? 4 : 3;
  if (it.page.includes(term)) return startsWord(it.page, term) ? 2.5 : 2;
  if (it.keywords.includes(term)) return startsWord(it.keywords, term) ? 1.5 : 1;
  return 0;
}

// search returns the entries matching every word typed (words may match anywhere, in
// any order). Entries are ranked by how well the words match (see score); ties keep
// the sidebar's order.
export function search(index: Indexed[], query: string): SettingsEntry[] {
  const terms = normalize(query).split(/\s+/).filter(Boolean);
  if (terms.length === 0) return [];
  const hits: { entry: SettingsEntry; total: number; order: number }[] = [];
  index.forEach((it, order) => {
    let total = 0;
    for (const w of terms) {
      const s = score(it, w);
      if (s === 0) return; // every word must match
      total += s;
    }
    hits.push({ entry: it.entry, total, order });
  });
  hits.sort((a, b) => b.total - a.total || a.order - b.order);
  return hits.map((h) => h.entry);
}
