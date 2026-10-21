import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router-dom";
import Swal from "sweetalert2";
import { theme } from "../theme";
import { useLayout } from "../components/Layout";
import {
  setSetting, setCurrencyActive, purgeArabic, getSettings, logout,
  listBackups, createBackup, restoreBackup, deleteBackup, downloadBackup, type BackupInfo,
  getBreachStatus, scanBreaches,
  getVersion, checkUpdate, applyUpdate, type VersionInfo,
} from "../api";
import { confirmDialog, errorAlert, toast } from "../alerts";
import { WEEKDAYS } from "../config";
import { Button } from "../components/Button";
import { IconButton } from "../components/IconButton";
import { Combobox } from "../components/Combobox";
import { FaTrash, FaDownload, FaUndo } from "react-icons/fa";

export function ConfigurationSettings() {
  const { t, i18n } = useTranslation();
  const { weekStart, clipboardClear, reloadSettings } = useLayout();
  const [ws, setWs] = useState(weekStart);
  const [clip, setClip] = useState(String(clipboardClear));
  // Weekday names localized to the current language (0 = Sunday; 2023-01-01 was a Sunday).
  const dayFmt = new Intl.DateTimeFormat(i18n.language, { weekday: "long" });
  const weekdayNames = WEEKDAYS.map((_, i) => dayFmt.format(new Date(2023, 0, 1 + i)));

  async function changeWeek(day: number) {
    setWs(day);
    try { await setSetting("week_start", String(day)); reloadSettings(); toast(t("config.saved")); }
    catch { /* ignore */ }
  }

  async function saveClip() {
    const n = Number(clip);
    if (!Number.isInteger(n) || n < 0 || n > 3600) { errorAlert(t("config.clipboardInvalid")); return; }
    try { await setSetting("clipboard_clear_seconds", String(n)); reloadSettings(); toast(t("config.saved")); }
    catch (e) { errorAlert((e as Error).message); }
  }

  return (
    <>
      <div className="card" id="s-general">
        <h2 className="section-title">{t("config.title")}</h2>
        <div className="inline-field">
          <span>{t("config.weekStartsOn")}</span>
          <select dir="ltr" value={ws} onChange={(e) => changeWeek(Number(e.target.value))}>
            {weekdayNames.map((d, i) => <option key={i} value={i}>{d}</option>)}
          </select>
        </div>
        <div className="inline-field">
          <span title={t("config.clipboardHint")}>{t("config.clipboardClear")}</span>
          <span style={{ display: "inline-flex", gap: 8, alignItems: "center" }}>
            <input type="number" min={0} max={3600} dir="ltr" value={clip}
              onChange={(e) => setClip(e.target.value)} onBlur={saveClip} style={{ maxWidth: 90 }} />
            <span className="muted">{t("config.seconds")}</span>
          </span>
        </div>
        <p className="field-hint">{t("config.clipboardHint")}</p>
      </div>

      <LanguagesSection />

      <UpdatesSection />

      <BreachSection />

      <BackupSection />

      <CurrenciesSection />
    </>
  );
}

function UpdatesSection() {
  const { t } = useTranslation();
  const [enabled, setEnabled] = useState(true);
  const [info, setInfo] = useState<VersionInfo | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    getSettings().then((s) => setEnabled(s.update_check !== "off")).catch(() => {});
    getVersion().then(setInfo).catch(() => {});
  }, []);

  async function toggle(on: boolean) {
    setEnabled(on);
    try { await setSetting("update_check", on ? "on" : "off"); toast(t("config.saved")); }
    catch (e) { errorAlert((e as Error).message); }
  }

  async function checkNow() {
    setBusy(true);
    try { setInfo(await checkUpdate()); }
    catch (e) { errorAlert((e as Error).message); }
    finally { setBusy(false); }
  }

  async function updateNow() {
    if (!info) return;
    const mac = info.os === "darwin";
    const ok = await confirmDialog({
      title: t("update.confirmTitle", { version: info.latest }),
      text: mac ? t("update.confirmTextMac") : t("update.confirmText"),
      confirmText: t("update.updateNow"),
    });
    if (!ok) return;
    setBusy(true);
    try { await applyUpdate(); toast(mac ? t("update.startedMac") : t("update.started")); }
    catch (e) { errorAlert((e as Error).message); }
    finally { setBusy(false); }
  }

  const canUpdate = !!info?.updateAvailable && !!info?.canApply;

  // Show the update banner with a mock version so its appearance can be checked
  // without an actual release available.
  function previewBanner() {
    const mock: VersionInfo = {
      current: info?.current ?? "0.0.0",
      latest: "9.9.9",
      updateAvailable: true,
      url: info?.url || "https://github.com/FayyazB106/account-manager/releases",
      notes: "",
      checked: new Date().toISOString(),
      canApply: info?.canApply ?? false,
      os: info?.os ?? "windows",
    };
    window.dispatchEvent(new CustomEvent<VersionInfo>("am:preview-update", { detail: mock }));
    // The banner renders at the top of the page; scroll up so it's visible.
    document.querySelector(".page")?.scrollTo({ top: 0, behavior: "smooth" });
    window.scrollTo({ top: 0, behavior: "smooth" });
  }

  return (
    <div className="card" style={{ marginTop: 16 }} id="s-updates">
      <h2 className="section-title">{t("update.settingsTitle")}</h2>

      <label className="switch-row">
        <input type="checkbox" checked={enabled} onChange={(e) => toggle(e.target.checked)} />
        <span>{t("update.checkForUpdates")}</span>
      </label>

      <div className="inline-field">
        <span>{t("update.currentVersion")}</span>
        <span className="muted" dir="ltr">{info ? info.current : "…"}</span>
      </div>

      <div style={{ display: "flex", gap: 12, alignItems: "center", marginTop: 8, flexWrap: "wrap" }}>
        {canUpdate ? (
          <Button onClick={updateNow} disabled={busy}>{busy ? t("update.updating") : t("update.updateNow")}</Button>
        ) : (
          <Button onClick={checkNow} disabled={busy}>{busy ? t("update.checking") : t("update.checkNow")}</Button>
        )}
        {import.meta.env.DEV && <Button variant="ghost" onClick={previewBanner}>{t("update.preview")}</Button>}
        <span className="muted">
          {info && info.updateAvailable ? (
            <>
              {t("update.available", { version: info.latest })}
              {info.url && <> · <a href={info.url} target="_blank" rel="noreferrer" className="update-banner-link">{t("update.whatsNew")}</a></>}
            </>
          ) : info && info.checked ? (
            t("update.upToDate")
          ) : ""}
        </span>
      </div>
    </div>
  );
}

function BreachSection() {
  const { t, i18n } = useTranslation();
  const { refresh } = useLayout();
  const [freq, setFreq] = useState("off");
  const [status, setStatus] = useState<{ breached: number; last_check: string }>({ breached: 0, last_check: "" });
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    getSettings().then((s) => setFreq(s.breach_monitor || "off")).catch(() => {});
    getBreachStatus().then(setStatus).catch(() => {});
  }, []);

  async function changeFreq(v: string) {
    setFreq(v);
    try { await setSetting("breach_monitor", v); toast(t("config.saved")); }
    catch (e) { errorAlert((e as Error).message); }
  }

  async function checkNow() {
    setBusy(true);
    try {
      const r = await scanBreaches();
      setStatus(await getBreachStatus());
      refresh(); // update account breach flags in the shared list
      toast(t("breach.checkedToast", { n: r.checked }));
    } catch (e) {
      errorAlert((e as Error).message);
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="card" style={{ marginTop: 16 }} id="s-breach">
      <h2 className="section-title">{t("breach.title")}</h2>
      <p className="muted" style={{ marginTop: 0 }}>{t("breach.desc")}</p>
      <div className="inline-field">
        <span>{t("breach.frequency")}</span>
        <select dir="ltr" value={freq} onChange={(e) => changeFreq(e.target.value)}>
          <option value="off">{t("breach.off")}</option>
          <option value="daily">{t("breach.daily")}</option>
          <option value="weekly">{t("breach.weekly")}</option>
        </select>
      </div>
      <div style={{ display: "flex", gap: 12, alignItems: "center", marginTop: 8, flexWrap: "wrap" }}>
        <Button onClick={checkNow} disabled={busy}>{busy ? t("breach.checking") : t("breach.checkNow")}</Button>
        <span className="muted">
          {status.last_check
            ? t("breach.lastCheck", { when: new Date(status.last_check).toLocaleString(i18n.language) })
            : t("breach.never")}
          {status.breached > 0 && <span className="danger-text"> · {t("breach.breachedCount", { n: status.breached })}</span>}
        </span>
      </div>
    </div>
  );
}

function humanSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(0)} KB`;
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
}

function BackupSection() {
  const { t, i18n } = useTranslation();
  const navigate = useNavigate();
  const [freq, setFreq] = useState("off");
  const [weekday, setWeekday] = useState("0");
  const [time, setTime] = useState("02:00");
  const [keep, setKeep] = useState("7");
  const [list, setList] = useState<BackupInfo[]>([]);
  const [busy, setBusy] = useState(false);

  // Localized weekday names (0 = Sunday; 2023-01-01 was a Sunday).
  const dayFmt = new Intl.DateTimeFormat(i18n.language, { weekday: "long" });
  const weekdayNames = Array.from({ length: 7 }, (_, i) => dayFmt.format(new Date(2023, 0, 1 + i)));

  async function refresh() {
    try { setList(await listBackups()); } catch { /* ignore */ }
  }
  useEffect(() => {
    getSettings().then((s) => {
      setFreq(s.backup_frequency || "off");
      setWeekday(s.backup_weekday || "0");
      setTime(s.backup_time || "02:00");
      setKeep(s.backup_keep || "7");
    }).catch(() => {});
    refresh();
  }, []);

  async function save(key: string, value: string) {
    try { await setSetting(key, value); toast(t("config.saved")); }
    catch (e) { errorAlert((e as Error).message); }
  }

  async function backupNow() {
    setBusy(true);
    try { await createBackup(); await refresh(); toast(t("backup.created")); }
    catch (e) { errorAlert((e as Error).message); }
    finally { setBusy(false); }
  }

  async function restore(name: string) {
    const ok = await confirmDialog({
      title: t("backup.restoreTitle"), text: t("backup.restoreText"),
      confirmText: t("backup.restore"), danger: true,
    });
    if (!ok) return;
    try {
      await restoreBackup(name);
      toast(t("backup.restored"));
      // Data (including admins/sessions) was replaced — sign out.
      logout();
      navigate("/login");
    } catch (e) { errorAlert((e as Error).message); }
  }

  async function remove(name: string) {
    const ok = await confirmDialog({
      title: t("backup.deleteTitle"), text: t("backup.deleteText"),
      confirmText: t("common.delete"), danger: true,
    });
    if (!ok) return;
    try { await deleteBackup(name); await refresh(); } catch (e) { errorAlert((e as Error).message); }
  }

  return (
    <div className="card" style={{ marginTop: 16 }} id="s-backups">
      <h2 className="section-title">{t("backup.title")}</h2>
      <p className="muted" style={{ marginTop: 0 }}>{t("backup.hint")}</p>

      <div className="backup-controls">
        <label className="inline-field">
          <span>{t("backup.frequency")}</span>
          <select dir="ltr" value={freq} onChange={(e) => { setFreq(e.target.value); save("backup_frequency", e.target.value); }}>
            <option value="off">{t("backup.off")}</option>
            <option value="daily">{t("backup.daily")}</option>
            <option value="weekly">{t("backup.weekly")}</option>
          </select>
        </label>
        {freq === "weekly" && (
          <label className="inline-field">
            <span>{t("backup.day")}</span>
            <select dir="ltr" value={weekday} onChange={(e) => { setWeekday(e.target.value); save("backup_weekday", e.target.value); }}>
              {weekdayNames.map((d, i) => <option key={i} value={i}>{d}</option>)}
            </select>
          </label>
        )}
        {freq !== "off" && (
          <label className="inline-field">
            <span>{t("backup.time")}</span>
            <input dir="ltr" type="time" value={time} onChange={(e) => setTime(e.target.value)} onBlur={() => save("backup_time", time)} />
          </label>
        )}
        <label className="inline-field">
          <span>{t("backup.keep")}</span>
          <input dir="ltr" type="number" min={1} max={100} value={keep}
            onChange={(e) => setKeep(e.target.value)} onBlur={() => save("backup_keep", keep)} />
        </label>
        <Button onClick={backupNow} disabled={busy}>{busy ? t("backup.creating") : t("backup.now")}</Button>
      </div>

      <ul className="cat-list" style={{ marginTop: 12 }}>
        {list.map((b) => (
          <li key={b.name}>
            <span>
              {new Date(b.created_at).toLocaleString(i18n.language)}
              <span className="muted"> · {humanSize(b.size)}</span>
            </span>
            <span className="att-actions">
              <IconButton title={t("backup.download")} onClick={() => downloadBackup(b.name).catch((e) => errorAlert((e as Error).message))}><FaDownload /></IconButton>
              <IconButton title={t("backup.restore")} onClick={() => restore(b.name)}><FaUndo /></IconButton>
              <IconButton danger title={t("common.delete")} onClick={() => remove(b.name)}><FaTrash /></IconButton>
            </span>
          </li>
        ))}
        {list.length === 0 && <li className="muted">{t("backup.none")}</li>}
      </ul>
    </div>
  );
}

function LanguagesSection() {
  const { t } = useTranslation();
  const { arabicEnabled, reloadSettings } = useLayout();
  const [busy, setBusy] = useState(false);

  async function enable() {
    setBusy(true);
    try { await setSetting("lang_arabic_enabled", "1"); await reloadSettings(); }
    catch (e) { errorAlert((e as Error).message); }
    finally { setBusy(false); }
  }

  async function disable() {
    const r = await Swal.fire({
      background: theme.surface,
      color: theme.text,
      icon: "warning",
      iconColor: theme.warn,
      title: t("config.turnOffTitle"),
      html: `<p style="margin:0 0 14px">${t("config.turnOffText")}</p>
        <div class="inline-check">
          <input type="checkbox" id="purge-ar">
          <span>${t("config.purgeLabel")}</span>
        </div>`,
      showCancelButton: true,
      confirmButtonText: t("config.turnOff"),
      cancelButtonText: t("common.cancel"),
      confirmButtonColor: theme.warn,
      cancelButtonColor: theme.borderStrong,
      preConfirm: () => ({ purge: (document.getElementById("purge-ar") as HTMLInputElement | null)?.checked ?? false }),
    });
    if (!r.isConfirmed) return;
    setBusy(true);
    try {
      await setSetting("lang_arabic_enabled", "0");
      if (r.value?.purge) await purgeArabic();
      await reloadSettings();
    } catch (e) { errorAlert((e as Error).message); }
    finally { setBusy(false); }
  }

  return (
    <div className="card" style={{ marginTop: 16 }} id="s-languages">
      <h2 className="section-title">{t("config.languages")}</h2>
      <label className="switch-row">
        <input
          type="checkbox"
          checked={arabicEnabled}
          disabled={busy}
          onChange={(e) => (e.target.checked ? enable() : disable())}
        />
        <span className="help-hint" title={t("config.languagesFields")}>{t("config.arabicSupport")}</span>
      </label>
    </div>
  );
}

function CurrenciesSection() {
  const { t } = useTranslation();
  const { currencies, reloadSettings } = useLayout();
  const [error, setError] = useState<string | null>(null);
  const [query, setQuery] = useState("");

  async function toggle(code: string, active: boolean) {
    try { await setCurrencyActive(code, active); reloadSettings(); } catch (e) { setError((e as Error).message); }
  }

  const active = currencies.filter((c) => c.active);
  const inactive = currencies.filter((c) => !c.active);

  return (
    <div className="card" style={{ marginTop: 16 }} id="s-currencies">
      <h2 className="section-title">{t("config.currencies")}</h2>
      <p className="muted" style={{ marginTop: 0 }}>
        {t("config.currenciesHint")}
      </p>
      {error && <p className="error">{error}</p>}

      <ul className="cat-list">
        {active.map((c) => (
          <li key={c.code}>
            <span>
              <strong>{c.code}</strong> <span className="muted">— {c.name}</span>
              {c.code !== "USD" && c.rate > 0 && <span className="muted"> · 1 USD = {c.rate.toFixed(2)} {c.code}</span>}
            </span>
            {c.code !== "USD" && (
              <IconButton danger title={t("config.deactivate")} onClick={() => toggle(c.code, false)}><FaTrash /></IconButton>
            )}
          </li>
        ))}
      </ul>

      <div className="inline-add">
        <Combobox
          value={query}
          placeholder={t("config.addCurrency")}
          options={inactive.map((c) => `${c.code} — ${c.name}`)}
          onChange={(v) => {
            const match = inactive.find((c) => `${c.code} — ${c.name}` === v);
            if (match) { toggle(match.code, true); setQuery(""); } else setQuery(v);
          }}
        />
      </div>
    </div>
  );
}
