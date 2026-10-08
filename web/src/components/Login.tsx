import { useEffect, useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { login, loginTOTP, getBranding } from "../api";
import { Button } from "./Button";
import { Field } from "./Field";
import { PasswordInput } from "./PasswordInput";
import { runHealthScan } from "../health-scan";

export function Login({ onLoggedIn }: { onLoggedIn: () => void }) {
  const { t } = useTranslation();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [org, setOrg] = useState<{ name: string; logo: string }>({ name: "", logo: "" });
  // Set once the password step passes but MFA is required.
  const [mfaToken, setMfaToken] = useState<string | null>(null);
  const [code, setCode] = useState("");

  useEffect(() => {
    getBranding().then((b) => setOrg({ name: b.org_name ?? "", logo: b.org_logo ?? "" })).catch(() => {});
  }, []);

  function finish() {
    // Kick off the Vault Health scan in the background on every sign-in so the
    // cached report is fresh; don't block navigation on it.
    runHealthScan().catch(() => {});
    onLoggedIn();
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    setBusy(true);
    try {
      const res = await login(email, password);
      if (res.kind === "mfa") {
        setMfaToken(res.mfaToken);
        return;
      }
      finish();
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setBusy(false);
    }
  }

  async function handleMfa(e: FormEvent) {
    e.preventDefault();
    setError(null);
    setBusy(true);
    try {
      await loginTOTP(mfaToken!, code.trim());
      finish();
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setBusy(false);
    }
  }

  const header = (
    <>
      {org.logo && <img className="login-logo" src={org.logo} alt={org.name || "Organization"} />}
      {org.name && <div className="login-org">{org.name}</div>}
    </>
  );

  if (mfaToken) {
    return (
      <div className="centered">
        <form className="card" onSubmit={handleMfa}>
          {header}
          <h1 style={{ textAlign: "center" }}>{t("login.mfaTitle")}</h1>
          <p className="muted" style={{ textAlign: "center", margin: 0 }}>{t("login.mfaHint")}</p>

          <Field label={t("login.code")}>
            <input
              value={code}
              onChange={(e) => setCode(e.target.value)}
              dir="ltr"
              inputMode="numeric"
              autoComplete="one-time-code"
              autoFocus
              required
            />
          </Field>

          {error && <p className="error">{error}</p>}

          <Button type="submit" disabled={busy}>
            {busy ? t("login.verifying") : t("login.verify")}
          </Button>
          <p className="field-hint" style={{ textAlign: "center" }}>{t("login.mfaBackupHint")}</p>
        </form>
      </div>
    );
  }

  return (
    <div className="centered">
      <form className="card" onSubmit={handleSubmit}>
        {header}
        <h1 style={{ textAlign: "center" }}>{t("login.title")}</h1>

        <Field label={t("login.email")}>
          <input
            type="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            autoComplete="username"
            required
          />
        </Field>

        <Field label={t("login.password")}>
          <PasswordInput
            value={password}
            onChange={setPassword}
            autoComplete="current-password"
            required
          />
        </Field>

        {error && <p className="error">{error}</p>}

        <Button type="submit" disabled={busy}>
          {busy ? t("login.signingIn") : t("login.signIn")}
        </Button>
      </form>
    </div>
  );
}
