import { useEffect, useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import {
  getSMTP, saveSMTP, setSMTPEnabled, type SMTPConfig, type SMTPSecurity,
} from "../api";
import { errorAlert, toast } from "../alerts";
import { Button } from "../components/Button";
import { Field } from "../components/Field";
import { PasswordInput } from "../components/PasswordInput";

// Typical port for each connection mode — applied when the mode is changed and the
// port is still one of these defaults (so a custom port is never overwritten).
const DEFAULT_PORT: Record<SMTPSecurity, string> = { tls: "465", starttls: "587", none: "25" };

// EmailSettings configures the SMTP account used to send reports and reminders.
// It is provider-agnostic; the Resend button just prefills its values. The test
// and "Send Now" buttons live with the emails themselves (Settings → Admin).
export function EmailSettings() {
  const { t } = useTranslation();
  const [cfg, setCfg] = useState<SMTPConfig | null>(null);
  const [host, setHost] = useState("");
  const [port, setPort] = useState("587");
  const [security, setSecurity] = useState<SMTPSecurity>("starttls");
  const [username, setUsername] = useState("");
  const [from, setFrom] = useState("");
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState<"" | "save" | "toggle">("");
  const [loadError, setLoadError] = useState<string | null>(null);

  function load(c: SMTPConfig) {
    setCfg(c);
    setHost(c.host);
    setPort(c.port ? String(c.port) : "587");
    setSecurity((c.security || "starttls") as SMTPSecurity);
    setUsername(c.username);
    setFrom(c.from);
    setPassword("");
  }
  // Surface a failed load instead of leaving a silently dead (locked) page — the
  // usual cause is a server that is older than this page and lacks /smtp.
  function fetchCfg() {
    setLoadError(null);
    getSMTP().then(load).catch((e) => setLoadError((e as Error).message));
  }
  useEffect(() => { fetchCfg(); }, []);

  function changeSecurity(next: SMTPSecurity) {
    if (Object.values(DEFAULT_PORT).includes(port)) setPort(DEFAULT_PORT[next]);
    setSecurity(next);
  }

  function useResend() {
    setHost("smtp.resend.com");
    setPort("465");
    setSecurity("tls");
    setUsername("resend");
  }

  async function save(e: FormEvent) {
    e.preventDefault();
    setBusy("save");
    try {
      await saveSMTP({
        host: host.trim(), port: Number(port), security, username: username.trim(),
        from: from.trim(), password: password || undefined,
      });
      load(await getSMTP());
      toast(t("config.saved"));
    } catch (err) {
      errorAlert((err as Error).message);
    } finally {
      setBusy("");
    }
  }

  // The master switch saves immediately (like the other switches in Settings).
  async function toggle(on: boolean) {
    setBusy("toggle");
    try {
      await setSMTPEnabled(on);
      setCfg((c) => (c ? { ...c, enabled: on } : c));
      toast(on ? t("email.enabledToast") : t("email.disabledToast"));
    } catch (err) {
      errorAlert((err as Error).message);
    } finally {
      setBusy("");
    }
  }

  const enabled = !!cfg?.enabled;

  return (
    <div className="card">
      <div className="topbar" style={{ marginBottom: 8 }}>
        <h2 className="section-title" style={{ margin: 0 }}>{t("email.title")}</h2>
        <label className="switch-row" title={t("email.enable")}>
          <input
            type="checkbox"
            role="switch"
            aria-label={t("email.enable")}
            checked={enabled}
            disabled={!cfg || busy !== ""}
            onChange={(e) => toggle(e.target.checked)}
          />
        </label>
      </div>
      {loadError && (
        <p className="error">
          {t("email.loadFailed")} <span dir="ltr">({loadError})</span>{" "}
          <Button type="button" variant="ghost" onClick={fetchCfg}>{t("email.retry")}</Button>
        </p>
      )}
      <p className="muted" style={{ marginTop: 0 }}>{enabled ? t("email.desc") : t("email.offHint")}</p>

      {/* The master switch locks the whole form while off (settings are kept). */}
      <fieldset className="email-fields" disabled={!enabled}>
      <form className="add-form" onSubmit={save}>
        <div className="grid">
          <Field label={t("email.host")}>
            <input dir="ltr" value={host} onChange={(e) => setHost(e.target.value)} placeholder="smtp.example.com" required />
          </Field>
          <Field label={t("email.port")}>
            <input dir="ltr" type="number" min={1} max={65535} value={port} onChange={(e) => setPort(e.target.value)} required />
          </Field>
          <Field label={t("email.security")}>
            <select dir="ltr" value={security} onChange={(e) => changeSecurity(e.target.value as SMTPSecurity)}>
              <option value="tls">{t("email.securityTls")}</option>
              <option value="starttls">{t("email.securityStarttls")}</option>
              <option value="none">{t("email.securityNone")}</option>
            </select>
          </Field>
          <Field label={t("email.username")}>
            <input dir="ltr" value={username} onChange={(e) => setUsername(e.target.value)} autoComplete="off" />
          </Field>
          <Field label={t("email.password")} hint={cfg?.has_password ? t("email.passwordKeep") : undefined}>
            {/* .pw-row keeps the eye centred; bare, the input stretches to the taller neighbour. */}
            <span className="pw-row">
              <PasswordInput value={password} onChange={setPassword} autoComplete="new-password"
                placeholder={cfg?.has_password ? "••••••••" : ""} />
            </span>
          </Field>
          <Field label={t("email.from")}>
            <input dir="ltr" type="email" value={from} onChange={(e) => setFrom(e.target.value)} placeholder="reports@example.com" required />
          </Field>
        </div>
        <div className="form-actions">
          <Button type="submit" disabled={busy !== ""}>{busy === "save" ? t("email.saving") : t("common.save")}</Button>
          <Button type="button" variant="ghost" onClick={useResend}>{t("email.useResend")}</Button>
        </div>
      </form>
      </fieldset>
    </div>
  );
}
