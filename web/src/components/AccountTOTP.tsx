import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { accountTOTPCode, setAccountTOTP, clearAccountTOTP } from "../api";
import { useLayout } from "./Layout";
import { copySecret } from "../clipboard";
import { errorAlert, toast, confirmDialog } from "../alerts";
import { Button } from "./Button";
import { Field } from "./Field";
import { CountdownRing } from "./CountdownRing";
import { QRScanner } from "./QRScanner";
import { FaQrcode } from "react-icons/fa";

// AccountTOTP shows a saved service's live 2FA code (with a 30s countdown) or,
// when none is stored, a form to add one from a setup key / otpauth URI.
export function AccountTOTP({ accountId, configured, onChanged }: { accountId: string; configured: boolean; onChanged: () => void }) {
  const { t } = useTranslation();
  const { clipboardClear } = useLayout();
  const [has, setHas] = useState(configured);
  const [code, setCode] = useState("");
  const [remaining, setRemaining] = useState(0);
  const [secret, setSecret] = useState("");
  const [busy, setBusy] = useState(false);
  const [scanning, setScanning] = useState(false);
  const timer = useRef<ReturnType<typeof setInterval> | undefined>(undefined);

  useEffect(() => { setHas(configured); }, [configured]);

  // Fetch the current code, then tick down; refetch when the window elapses.
  useEffect(() => {
    if (!has) return;
    let alive = true;
    async function load() {
      try {
        const r = await accountTOTPCode(accountId);
        if (!alive) return;
        setCode(r.code);
        setRemaining(r.expires_in);
      } catch { /* cleared elsewhere, etc. */ }
    }
    load();
    timer.current = setInterval(() => {
      setRemaining((s) => {
        if (s <= 1) { load(); return 30; }
        return s - 1;
      });
    }, 1000);
    return () => { alive = false; if (timer.current) clearInterval(timer.current); };
  }, [has, accountId]);

  async function save(value?: string) {
    const v = (value ?? secret).trim();
    if (!v) return;
    setBusy(true);
    try {
      await setAccountTOTP(accountId, v);
      setSecret("");
      setHas(true);
      onChanged();
      toast(t("accountTotp.added"));
    } catch (e) {
      errorAlert((e as Error).message);
    } finally {
      setBusy(false);
    }
  }

  // A scanned QR is typically an otpauth:// URI (or, rarely, a bare base32 key).
  // Reject anything else with a clear message; otherwise save it directly (the
  // backend accepts both forms).
  function onScan(text: string) {
    setScanning(false);
    const v = text.trim();
    const looksTotp = v.toLowerCase().startsWith("otpauth://") || /^[a-z2-7]{8,}=*$/i.test(v.replace(/\s+/g, ""));
    if (!looksTotp) { errorAlert(t("qr.notTotp")); return; }
    setSecret(v);
    save(v);
  }

  async function remove() {
    const ok = await confirmDialog({
      title: t("accountTotp.removeTitle"),
      text: t("accountTotp.removeText"),
      confirmText: t("common.remove"),
      danger: true,
    });
    if (!ok) return;
    try {
      await clearAccountTOTP(accountId);
      setHas(false);
      setCode("");
      onChanged();
      toast(t("accountTotp.removed"));
    } catch (e) {
      errorAlert((e as Error).message);
    }
  }

  return (
    <div className="card" style={{ marginTop: 16 }}>
      <div className="topbar" style={{ marginBottom: 12 }}>
        <h2 className="section-title" style={{ margin: 0 }}>{t("accountTotp.title")}</h2>
        {has && <span title={t("accountTotp.expiresIn")}><CountdownRing seconds={remaining} /></span>}
      </div>
      {has ? (
        <div className="acct-totp">
          <code className="acct-totp-code" dir="ltr" title={t("common.clickToCopy")}
            onClick={() => code && copySecret(code, clipboardClear)}>
            {code ? `${code.slice(0, 3)} ${code.slice(3)}` : "······"}
          </code>
          <Button variant="ghost" onClick={remove}>{t("common.remove")}</Button>
        </div>
      ) : (
        <>
          <p className="muted" style={{ marginTop: 0 }}>{t("accountTotp.desc")}</p>
          <div className="acct-totp-add">
            <Field label={t("accountTotp.secretLabel")}>
              <input value={secret} onChange={(e) => setSecret(e.target.value)} dir="ltr"
                placeholder={t("accountTotp.secretPlaceholder")} autoComplete="off" />
            </Field>
            <div className="totp-actions">
              <Button variant="ghost" onClick={() => setScanning(true)} disabled={busy}>
                <FaQrcode style={{ marginInlineEnd: 6 }} />{t("qr.scanBtn")}
              </Button>
              <Button onClick={() => save()} disabled={busy || !secret.trim()}>{t("common.save")}</Button>
            </div>
          </div>
        </>
      )}
      {scanning && <QRScanner onResult={onScan} onClose={() => setScanning(false)} />}
    </div>
  );
}
