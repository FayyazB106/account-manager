import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { getVersion, type VersionInfo } from "../api";
import { confirmAndApplyUpdate } from "../updateFlow";
import { Button } from "./Button";

// UpdateBanner shows a dismissible notice when a newer release is available.
// On desktop (canApply) it offers a one-click "Update now"; otherwise a link to
// the release page to download manually. Dismissal is per-version (sessionStorage)
// so a new version re-shows it.
export function UpdateBanner() {
  const { t } = useTranslation();
  const [info, setInfo] = useState<VersionInfo | null>(null);
  const [busy, setBusy] = useState(false);
  const [dismissed, setDismissed] = useState(false);

  useEffect(() => {
    getVersion()
      .then((v) => {
        if (v.updateAvailable) {
          const key = `am_update_dismissed_${v.latest}`;
          let already = false;
          try { already = sessionStorage.getItem(key) === "1"; } catch { /* ignore */ }
          if (!already) setInfo(v);
        }
      })
      .catch(() => { /* update check is best-effort */ });
  }, []);

  // Preview trigger (Settings → Updates → "Preview notification"): force the
  // banner to show with a mock version so the look can be checked without a
  // real release.
  useEffect(() => {
    function onPreview(e: Event) {
      setInfo((e as CustomEvent<VersionInfo>).detail);
      setDismissed(false);
    }
    window.addEventListener("am:preview-update", onPreview as EventListener);
    return () => window.removeEventListener("am:preview-update", onPreview as EventListener);
  }, []);

  if (!info || dismissed) return null;

  function dismiss() {
    setDismissed(true);
    try { sessionStorage.setItem(`am_update_dismissed_${info!.latest}`, "1"); } catch { /* ignore */ }
  }

  function update() {
    return confirmAndApplyUpdate(info!, setBusy);
  }

  return (
    <div className="update-banner">
      <span className="update-banner-text">
        {t("update.available", { version: info.latest })}
      </span>
      <span className="update-banner-actions">
        {info.url && (
          <a href={info.url} target="_blank" rel="noreferrer" className="update-banner-link">
            {t("update.whatsNew")}
          </a>
        )}
        {info.canApply ? (
          <Button onClick={update} disabled={busy}>
            {busy ? t("update.updating") : t("update.updateNow")}
          </Button>
        ) : (
          info.url && (
            <a href={info.url} target="_blank" rel="noreferrer" className="btn-primary update-banner-dl">
              {t("update.download")}
            </a>
          )
        )}
        <button className="update-banner-close" onClick={dismiss} aria-label={t("common.close")}>×</button>
      </span>
    </div>
  );
}
