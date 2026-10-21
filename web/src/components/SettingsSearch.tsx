import { useMemo, useState, type KeyboardEvent } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router-dom";
import { useLayout } from "./Layout";
import { buildIndex, search, type SettingsEntry } from "../settingsSearch";

// The Settings search is one piece of state shown in two places, because the layout
// differs: on desktop the box sits under the "Settings" title in the sidebar and the
// results replace the page links there; on a phone the box gets its own full-width
// row above the tab strip and the results open as a card under it. SettingsLayout
// calls this hook once and renders the box/results where each layout wants them.
export function useSettingsSearch() {
  const { i18n } = useTranslation();
  const { arabicEnabled } = useLayout();
  const navigate = useNavigate();
  const [q, setQ] = useState("");
  const [active, setActive] = useState(0);

  // Search in both languages (English always; Arabic when Arabic support is on).
  const index = useMemo(
    () => buildIndex(i18n, arabicEnabled ? ["en", "ar"] : ["en"]),
    [i18n, arabicEnabled],
  );
  const results = useMemo(() => search(index, q), [index, q]);
  const searching = q.trim() !== "";

  function clear() { setQ(""); setActive(0); }

  // Open the setting's page; SettingsLayout scrolls to (and flashes) the section from
  // the #hash.
  function open(e: SettingsEntry) {
    navigate(`${e.to}${e.anchor ? `#${e.anchor}` : ""}`);
    clear();
  }

  function onKeyDown(e: KeyboardEvent<HTMLInputElement>) {
    if (e.key === "Escape") { clear(); return; }
    if (!searching || results.length === 0) return;
    if (e.key === "ArrowDown") { e.preventDefault(); setActive((a) => Math.min(a + 1, results.length - 1)); }
    else if (e.key === "ArrowUp") { e.preventDefault(); setActive((a) => Math.max(a - 1, 0)); }
    else if (e.key === "Enter") { e.preventDefault(); open(results[Math.min(active, results.length - 1)]); }
  }

  return { q, setQ, active, setActive, results, searching, open, onKeyDown };
}

export type SettingsSearchState = ReturnType<typeof useSettingsSearch>;

// The text box.
export function SettingsSearchBox({ s }: { s: SettingsSearchState }) {
  const { t } = useTranslation();
  return (
    <input
      type="search"
      value={s.q}
      onChange={(e) => { s.setQ(e.target.value); s.setActive(0); }}
      onKeyDown={s.onKeyDown}
      placeholder={t("settingsSearch.placeholder")}
      aria-label={t("settingsSearch.label")}
      enterKeyHint="search"
      autoComplete="off"
    />
  );
}

// The matching settings (or a "none" message).
export function SettingsSearchResults({ s }: { s: SettingsSearchState }) {
  const { t } = useTranslation();
  return (
    <div className="subnav-results" role="listbox" aria-label={t("settingsSearch.label")}>
      {s.results.map((r, i) => {
        const name = t(r.label);
        const page = t(r.page);
        return (
          <button
            key={r.id}
            type="button"
            role="option"
            aria-selected={i === s.active}
            className={`subnav-result${i === s.active ? " active" : ""}`}
            onMouseEnter={() => s.setActive(i)}
            onClick={() => s.open(r)}
          >
            <span>{name}</span>
            {/* a page-level hit ("Email › Email") needs no breadcrumb */}
            {page !== name && <small>{page}</small>}
          </button>
        );
      })}
      {s.results.length === 0 && <p className="muted subnav-none">{t("settingsSearch.none")}</p>}
    </div>
  );
}
