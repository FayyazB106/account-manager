import { useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router-dom";
import { createFirstAdmin } from "../api";
import { Button } from "../components/Button";
import { Field } from "../components/Field";
import { PasswordInput } from "../components/PasswordInput";

// FirstAdminPage — shown when the vault is already unlocked (server mode, MASTER_KEY
// in the environment) but no admin exists yet, so there is nobody to sign in as.
// Creates the first admin and signs them in. The server refuses this once any admin
// exists, so it cannot be used to add accounts later.
export function FirstAdminPage({ onDone }: { onDone: () => void }) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [confirmPw, setConfirmPw] = useState("");
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function submit(e: FormEvent) {
    e.preventDefault();
    setErr(null);
    if (password.length < 12) { setErr(t("setup.passwordShort")); return; }
    if (password !== confirmPw) { setErr(t("setup.passwordMismatch")); return; }
    setBusy(true);
    try {
      await createFirstAdmin(email, password);
      onDone();
      navigate("/", { replace: true });
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="centered">
      <form className="card" onSubmit={submit}>
        <h1>{t("firstAdmin.title")}</h1>
        <p className="muted" style={{ marginTop: 0 }}>{t("firstAdmin.desc")}</p>

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
        <Button type="submit" disabled={busy}>{busy ? t("setup.creating") : t("firstAdmin.create")}</Button>
      </form>
    </div>
  );
}
