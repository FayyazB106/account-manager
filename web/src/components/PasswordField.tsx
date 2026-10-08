import { useEffect, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { generatePassword } from "../api";
import { errorAlert } from "../alerts";
import { passwordStrength, formatCrackTime } from "../strength";
import { pwnedCount } from "../pwned";
import { Field } from "./Field";
import { Button } from "./Button";
import { PasswordInput } from "./PasswordInput";

// PasswordField — the app's password entry control: Generate button, a live
// strength meter (offline-threat calibrated), and a debounced Have I Been Pwned
// breach check. The value is shown as text so a generated password is visible.
export function PasswordField({
  label,
  value,
  onChange,
  hint,
  autoComplete,
  required,
  full,
}: {
  label: ReactNode;
  value: string;
  onChange: (v: string) => void;
  hint?: string;
  autoComplete?: string;
  required?: boolean;
  full?: boolean;
}) {
  const { t } = useTranslation();
  const [pwned, setPwned] = useState<number | null>(null);

  // Debounced breach check: wait for typing to settle before querying HIBP.
  useEffect(() => {
    if (!value) { setPwned(null); return; }
    setPwned(null);
    const t = setTimeout(async () => {
      const n = await pwnedCount(value);
      setPwned(n);
    }, 500);
    return () => clearTimeout(t);
  }, [value]);

  async function gen() {
    try {
      onChange((await generatePassword(20)).password);
    } catch (err) {
      errorAlert((err as Error).message);
    }
  }

  const s = value ? passwordStrength(value) : null;

  return (
    <Field label={label} hint={hint} full={full}>
      <span className="pw-row">
        <PasswordInput value={value} onChange={onChange} autoComplete={autoComplete} required={required} />
        <Button type="button" onClick={gen}>{t("home.generate")}</Button>
      </span>
      {s && (
        <span className={`field-hint strength-${s.label}`}>
          {t("pw.strengthLine", { label: t(`strength.${s.label}`), time: formatCrackTime(t, s.crackSeconds) })}
        </span>
      )}
      {pwned !== null && pwned > 0 && (
        <span className="field-hint strength-weak">
          {t("pw.breach", { count: pwned })}
        </span>
      )}
    </Field>
  );
}
