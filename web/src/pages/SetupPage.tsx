import { useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { setupApp } from "../api";
import { Button } from "../components/Button";
import { Field } from "../components/Field";
import { PasswordInput } from "../components/PasswordInput";

// SetupPage — first-run onboarding in the browser: choose a passphrase (which
// seals the vault's master key) and create the first admin account. Shown when
// the backend reports state "setup". On success the new admin is logged in.
export function SetupPage({ onDone }: { onDone: () => void }) {
  const { t } = useTranslation();
  const [passphrase, setPassphrase] = useState("");
  const [confirm, setConfirm] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [confirmPw, setConfirmPw] = useState("");
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function submit(e: FormEvent) {
    e.preventDefault();
    setErr(null);
    if (passphrase.length < 8) { setErr(t("setup.passphraseShort")); return; }
    if (passphrase !== confirm) { setErr(t("setup.passphraseMismatch")); return; }
    if (password.length < 12) { setErr(t("setup.passwordShort")); return; }
    if (password !== confirmPw) { setErr(t("setup.passwordMismatch")); return; }
    setBusy(true);
    try {
      await setupApp(passphrase, email, password);
      onDone();
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="centered">
      <form className="card" onSubmit={submit}>
        <h1>{t("setup.title")}</h1>
        <p className="muted" style={{ marginTop: 0 }}>{t("setup.desc")}</p>

        <h2 className="section-title" style={{ marginBottom: 0 }}>{t("setup.vaultHeading")}</h2>
        <Field label={t("setup.passphrase")} hint={t("setup.passphraseHint")}>
          <PasswordInput value={passphrase} onChange={setPassphrase} autoComplete="new-password" />
        </Field>
        <Field label={t("setup.confirmPassphrase")}>
          <PasswordInput value={confirm} onChange={setConfirm} autoComplete="new-password" />
        </Field>

        <h2 className="section-title" style={{ marginBottom: 0 }}>{t("setup.adminHeading")}</h2>
        <Field label={t("setup.adminEmail")}>
          <input type="email" value={email} onChange={(e) => setEmail(e.target.value)} autoComplete="username" required />
        </Field>
        <Field label={t("setup.adminPassword")} hint={t("setup.passwordHint")}>
          <PasswordInput value={password} onChange={setPassword} autoComplete="new-password" />
        </Field>
        <Field label={t("setup.confirmPassword")}>
          <PasswordInput value={confirmPw} onChange={setConfirmPw} autoComplete="new-password" />
        </Field>

        {err && <p className="error">{err}</p>}
        <Button type="submit" disabled={busy}>{busy ? t("setup.creating") : t("setup.create")}</Button>
      </form>
    </div>
  );
}
