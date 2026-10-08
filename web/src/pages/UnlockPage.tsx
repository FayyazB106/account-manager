import { useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { unlockVault } from "../api";
import { Button } from "../components/Button";
import { Field } from "../components/Field";
import { PasswordInput } from "../components/PasswordInput";

// UnlockPage — shown when a sealed vault exists but isn't unlocked this run
// (backend state "locked"). Entering the passphrase unseals the master key; the
// caller then proceeds to the normal login screen.
export function UnlockPage({ onUnlocked }: { onUnlocked: () => void }) {
  const { t } = useTranslation();
  const [passphrase, setPassphrase] = useState("");
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function submit(e: FormEvent) {
    e.preventDefault();
    setErr(null);
    setBusy(true);
    try {
      await unlockVault(passphrase);
      onUnlocked();
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="centered">
      <form className="card" onSubmit={submit}>
        <h1>{t("unlock.title")}</h1>
        <p className="muted" style={{ marginTop: 0 }}>{t("unlock.desc")}</p>
        <Field label={t("unlock.passphrase")}>
          <PasswordInput value={passphrase} onChange={setPassphrase} autoComplete="current-password" />
        </Field>
        {err && <p className="error">{err}</p>}
        <Button type="submit" disabled={busy}>{busy ? t("unlock.unlocking") : t("unlock.unlock")}</Button>
      </form>
    </div>
  );
}
