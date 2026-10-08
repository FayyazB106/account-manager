import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import QRCode from "qrcode";
import Swal from "sweetalert2";
import { theme } from "../theme";
import { totpStatus, totpSetup, totpEnable, totpDisable } from "../api";
import { errorAlert, toast } from "../alerts";
import { Button } from "./Button";
import { Field } from "./Field";
import { Modal } from "./Modal";

// TwoFactorCard manages the admin's TOTP MFA: enrollment (QR + confirm code +
// backup codes) and disabling. Shown in Settings → Admin.
export function TwoFactorCard() {
  const { t } = useTranslation();
  const [enabled, setEnabled] = useState(false);
  const [remaining, setRemaining] = useState(0);
  const [loading, setLoading] = useState(true);

  // Enrollment state.
  const [qr, setQr] = useState<string | null>(null);
  const [secret, setSecret] = useState("");
  const [code, setCode] = useState("");
  const [busy, setBusy] = useState(false);
  const [backupCodes, setBackupCodes] = useState<string[] | null>(null);

  async function refresh() {
    try {
      const s = await totpStatus();
      setEnabled(s.enabled);
      setRemaining(s.backup_codes_remaining);
    } catch { /* ignore */ } finally {
      setLoading(false);
    }
  }
  useEffect(() => { refresh(); }, []);

  async function startSetup() {
    setBusy(true);
    try {
      const { secret, otpauth_url } = await totpSetup();
      setSecret(secret);
      setQr(await QRCode.toDataURL(otpauth_url, { margin: 1, width: 200 }));
      setCode("");
    } catch (e) {
      errorAlert((e as Error).message);
    } finally {
      setBusy(false);
    }
  }

  async function confirmEnable() {
    setBusy(true);
    try {
      const { backup_codes } = await totpEnable(code.trim());
      setQr(null);
      setSecret("");
      setBackupCodes(backup_codes);
      await refresh();
      toast(t("totp.enabledToast"));
    } catch (e) {
      errorAlert((e as Error).message);
    } finally {
      setBusy(false);
    }
  }

  async function disable() {
    const { value: entered } = await Swal.fire({
      background: theme.surface, color: theme.text,
      title: t("totp.disableTitle"),
      input: "text",
      inputAttributes: { autocapitalize: "off", autocomplete: "one-time-code" },
      inputPlaceholder: t("totp.codePlaceholder"),
      showCancelButton: true,
      confirmButtonText: t("totp.disable"),
      cancelButtonText: t("common.cancel"),
      confirmButtonColor: theme.danger,
      cancelButtonColor: theme.borderStrong,
      preConfirm: (v: string) => (v && v.trim() ? v.trim() : Swal.showValidationMessage(t("totp.codeRequired"))),
    });
    if (!entered) return;
    try {
      await totpDisable(entered);
      await refresh();
      toast(t("totp.disabledToast"));
    } catch (e) {
      errorAlert((e as Error).message);
    }
  }

  function downloadCodes() {
    if (!backupCodes) return;
    const blob = new Blob([backupCodes.join("\n") + "\n"], { type: "text/plain" });
    const a = document.createElement("a");
    a.href = URL.createObjectURL(blob);
    a.download = "account-manager-backup-codes.txt";
    a.click();
    URL.revokeObjectURL(a.href);
  }

  return (
    <div className="totp-section">
      <h2 className="section-title">{t("totp.title")}</h2>
      <p className="muted" style={{ marginTop: 0 }}>{t("totp.desc")}</p>

      {!loading && !qr && (
        <label className="switch-row">
          <input
            type="checkbox"
            checked={enabled}
            disabled={busy}
            onChange={(e) => (e.target.checked ? startSetup() : disable())}
          />
          <span>
            {enabled
              ? <>{t("totp.statusOn")} <span className="muted">· {t("totp.remaining", { n: remaining })}</span></>
              : t("totp.statusOff")}
          </span>
        </label>
      )}

      {qr && (
        <div className="totp-setup">
          <p style={{ marginTop: 0 }}>{t("totp.scanHint")}</p>
          <div className="totp-setup-cols">
            <img className="totp-qr" src={qr} alt="TOTP QR code" width={200} height={200} />
            <div className="totp-setup-fields">
              <p className="field-hint" style={{ marginTop: 0 }}>{t("totp.manualHint")}</p>
              <code className="totp-secret" dir="ltr">{secret}</code>
              <Field label={t("totp.codeLabel")}>
                <input value={code} onChange={(e) => setCode(e.target.value)} dir="ltr" inputMode="numeric" autoComplete="one-time-code" style={{ maxWidth: 160 }} />
              </Field>
              <div className="form-actions">
                <Button onClick={confirmEnable} disabled={busy || !code.trim()}>{t("totp.verifyEnable")}</Button>
                <Button variant="ghost" onClick={() => { setQr(null); setSecret(""); }}>{t("common.cancel")}</Button>
              </div>
            </div>
          </div>
        </div>
      )}

      {backupCodes && (
        <Modal onClose={() => setBackupCodes(null)}>
          <h2 className="section-title">{t("totp.backupTitle")}</h2>
          <p className="muted">{t("totp.backupHint")}</p>
          <ul className="backup-codes" dir="ltr">
            {backupCodes.map((c) => <li key={c}><code>{c}</code></li>)}
          </ul>
          <div className="backup-actions">
            <Button variant="ghost" onClick={downloadCodes}>{t("totp.download")}</Button>
            <Button onClick={() => setBackupCodes(null)}>{t("totp.done")}</Button>
          </div>
        </Modal>
      )}
    </div>
  );
}
