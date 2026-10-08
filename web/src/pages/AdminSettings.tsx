import { useEffect, useRef, useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import {
  getProfile, updateProfile, changePassword,
  listAdmins, addAdmin, deleteAdmin, type Admin,
  getSettings, setSetting,
} from "../api";
import { confirmDialog, errorAlert, toast } from "../alerts";
import { getThemeMode, applyThemeMode, type ThemeMode } from "../theme";
import { useLayout } from "../components/Layout";
import { Button } from "../components/Button";
import { IconButton } from "../components/IconButton";
import { Field } from "../components/Field";
import { PasswordField } from "../components/PasswordField";
import { PasswordInput } from "../components/PasswordInput";
import { CircleButton } from "../components/CircleButton";
import { Modal } from "../components/Modal";
import { TwoFactorCard } from "../components/TwoFactor";
import { FaTrash, FaBuilding, FaCamera, FaTimes, FaShieldAlt, FaSun, FaMoon } from "react-icons/fa";

export function AdminSettings() {
  return (
    <>
      <div className="card">
        <div className="two-col">
          <ProfileCard />
          <ResetPasswordCard />
        </div>
        <TwoFactorCard />
      </div>
      <AppearanceCard />
      <OrganizationCard />
      <AdminsCard />
    </>
  );
}

// Per-user light/dark theme preference (stored locally on this device).
function AppearanceCard() {
  const { t } = useTranslation();
  const [mode, setMode] = useState<ThemeMode>(getThemeMode);
  function change(next: ThemeMode) {
    setMode(next);
    applyThemeMode(next);
  }
  return (
    <div className="card" style={{ marginTop: 16 }}>
      <h2 className="section-title">{t("config.appearance")}</h2>
      <div className="inline-field">
        <span>{t("config.theme")}</span>
        <button
          type="button"
          className={`theme-toggle ${mode === "dark" ? "dark" : "light"}`}
          role="switch"
          aria-checked={mode === "dark"}
          aria-label={t("config.theme")}
          title={mode === "dark" ? t("config.themeDark") : t("config.themeLight")}
          onClick={() => change(mode === "dark" ? "light" : "dark")}
        >
          <span className="theme-toggle-knob">{mode === "dark" ? <FaMoon /> : <FaSun />}</span>
        </button>
      </div>
      <p className="field-hint">{t("config.themeHint")}</p>
    </div>
  );
}

// Max logo size we'll store as base64 in a setting value (~5 MB of raw bytes).
const MAX_LOGO_BYTES = 5_000_000;

function OrganizationCard() {
  const { t } = useTranslation();
  const { reloadSettings } = useLayout();
  const [name, setName] = useState("");
  const [logo, setLogo] = useState(""); // data URL, or "" for none
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const fileRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    getSettings().then((s) => {
      setName(s.org_name ?? "");
      setLogo(s.org_logo ?? "");
    }).catch(() => {});
  }, []);

  function pickFile(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0];
    e.target.value = ""; // allow re-selecting the same file
    if (!file) return;
    if (!file.type.startsWith("image/")) { errorAlert(t("admin.notImage")); return; }
    if (file.size > MAX_LOGO_BYTES) { errorAlert(t("admin.orgTooLarge")); return; }
    const reader = new FileReader();
    reader.onload = () => { setLogo(String(reader.result)); };
    reader.onerror = () => errorAlert(t("admin.cantReadImage"));
    reader.readAsDataURL(file);
  }

  async function save(e: FormEvent) {
    e.preventDefault();
    setErr(null); setBusy(true);
    try {
      await setSetting("org_name", name.trim());
      await setSetting("org_logo", logo);
      toast(t("admin.orgSaved"));
      reloadSettings();
    } catch (e) { setErr((e as Error).message); }
    finally { setBusy(false); }
  }

  return (
    <div className="card" style={{ marginTop: 16 }}>
      <h2 className="section-title">{t("admin.organization")} <span className="muted" style={{ fontWeight: 400, fontSize: 13 }}>· {t("common.optional")}</span></h2>
      <form className="add-form" onSubmit={save}>
        <div className="org-row">
          <input ref={fileRef} type="file" accept="image/*" hidden onChange={pickFile} />
          <div className="org-logo-wrap">
            <button
              type="button"
              className="org-logo"
              title={logo ? t("admin.changePicture") : t("admin.uploadPicture")}
              onClick={() => fileRef.current?.click()}
            >
              {logo ? <img src={logo} alt={t("admin.logoAlt")} /> : <span className="org-logo-empty"><FaBuilding /></span>}
              <span className="org-logo-overlay"><FaCamera /></span>
            </button>
            {logo && (
              <button
                type="button"
                className="org-logo-remove"
                title={t("common.remove")}
                onClick={() => setLogo("")}
              >
                <FaTimes />
              </button>
            )}
          </div>
          <Field label={t("admin.orgName")} className="org-name-field">
            <input value={name} onChange={(e) => setName(e.target.value)} placeholder={t("admin.orgNamePlaceholder")} />
          </Field>
          <Button type="submit" disabled={busy}>{busy ? t("admin.savingOrg") : t("admin.saveOrg")}</Button>
        </div>
        {err && <p className="error">{err}</p>}
      </form>
    </div>
  );
}

function ProfileCard() {
  const { t } = useTranslation();
  const { arabicEnabled } = useLayout();
  const [name, setName] = useState("");
  const [nameAr, setNameAr] = useState("");
  const [email, setEmail] = useState("");
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    getProfile().then((p) => { setName(p.name); setNameAr(p.name_ar ?? ""); setEmail(p.email); }).catch(() => {});
  }, []);

  async function save(e: FormEvent) {
    e.preventDefault();
    setErr(null); setBusy(true);
    try { await updateProfile(name.trim(), email.trim(), nameAr.trim() || undefined); toast(t("admin.profileSaved")); }
    catch (e) { setErr((e as Error).message); }
    finally { setBusy(false); }
  }

  return (
    <div>
      <h2 className="section-title">{t("admin.profile")}</h2>
      <form className="add-form" onSubmit={save}>
        <Field label={t("common.name")}><input value={name} onChange={(e) => setName(e.target.value)} /></Field>
        {arabicEnabled && (
          <Field label={t("common.nameArabic")}><input value={nameAr} onChange={(e) => setNameAr(e.target.value)} dir="rtl" placeholder={t("common.namePlaceholderAr")} /></Field>
        )}
        <Field label={t("common.email")}><input type="email" value={email} onChange={(e) => setEmail(e.target.value)} required /></Field>
        {err && <p className="error">{err}</p>}
        <div className="form-actions"><Button type="submit" disabled={busy}>{busy ? t("admin.savingProfile") : t("admin.saveProfile")}</Button></div>
      </form>
    </div>
  );
}

function ResetPasswordCard() {
  const { t } = useTranslation();
  const [current, setCurrent] = useState("");
  const [next, setNext] = useState("");
  const [confirm, setConfirm] = useState("");
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function save(e: FormEvent) {
    e.preventDefault();
    setErr(null);
    if (next !== confirm) { setErr(t("admin.passwordsNoMatch")); return; }
    if (next.length < 12) { setErr(t("admin.passwordTooShort")); return; }
    setBusy(true);
    try {
      await changePassword(current, next);
      toast(t("admin.passwordChanged")); setCurrent(""); setNext(""); setConfirm("");
    } catch (e) { setErr((e as Error).message); }
    finally { setBusy(false); }
  }

  return (
    <div>
      <h2 className="section-title">{t("admin.resetPassword")}</h2>
      <form className="add-form" onSubmit={save}>
        <Field label={t("admin.currentPassword")}>
          <PasswordInput value={current} onChange={setCurrent} autoComplete="current-password" required />
        </Field>
        <PasswordField label={t("admin.newPassword")} hint={t("common.atLeast12")} value={next} onChange={setNext} autoComplete="new-password" required />
        <Field label={t("admin.confirmNewPassword")}>
          <PasswordInput value={confirm} onChange={setConfirm} autoComplete="new-password" required />
        </Field>
        {err && <p className="error">{err}</p>}
        <div className="form-actions"><Button type="submit" disabled={busy}>{busy ? t("admin.changingPassword") : t("admin.changePassword")}</Button></div>
      </form>
    </div>
  );
}

function AdminsCard() {
  const { t } = useTranslation();
  const { arabicEnabled, lang } = useLayout();
  // In Arabic, show the Arabic name as the primary (bold) and English as muted.
  const primaryName = (name: string, ar?: string) => (lang === "ar" && ar ? ar : name);
  const secondaryName = (name: string, ar?: string) => (lang === "ar" ? name : ar);
  const [admins, setAdmins] = useState<Admin[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [addError, setAddError] = useState<string | null>(null);
  const [email, setEmail] = useState("");
  const [name, setName] = useState("");
  const [nameAr, setNameAr] = useState("");
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [showAdd, setShowAdd] = useState(false);

  async function refresh() {
    try { setAdmins(await listAdmins()); } catch (e) { setError((e as Error).message); }
  }
  useEffect(() => { refresh(); }, []);

  async function handleAdd(e: FormEvent) {
    e.preventDefault();
    setAddError(null);
    if (password.length < 12) { setAddError(t("admin.passwordTooShortAdd")); return; }
    setBusy(true);
    try {
      await addAdmin(email.trim(), name.trim(), password, nameAr.trim() || undefined);
      setEmail(""); setName(""); setNameAr(""); setPassword(""); setShowAdd(false);
      await refresh();
    } catch (e) { setAddError((e as Error).message); }
    finally { setBusy(false); }
  }

  function openAdd() { setAddError(null); setShowAdd(true); }
  function closeAdd() { setAddError(null); setShowAdd(false); }

  async function remove(a: Admin) {
    const ok = await confirmDialog({
      title: t("admin.deleteAdminTitle", { name: a.name || a.email }),
      text: t("admin.deleteAdminText"),
      confirmText: t("common.delete"), danger: true,
    });
    if (!ok) return;
    try { await deleteAdmin(a.id); await refresh(); } catch (e) { setError((e as Error).message); }
  }

  return (
    <div className="card" style={{ marginTop: 16 }}>
      <div className="topbar" style={{ marginBottom: 12 }}>
        <h2 className="section-title" style={{ margin: 0 }}>{t("admin.admins")}</h2>
        <CircleButton open={showAdd} title={showAdd ? t("common.close") : t("admin.addAdmin")} onClick={() => (showAdd ? closeAdd() : openAdd())}>+</CircleButton>
      </div>
      {error && <p className="error">{error}</p>}
      <ul className="cat-list">
        {admins.map((a) => (
          <li key={a.id}>
            <span className="admin-name">
              <strong>{primaryName(a.name, a.name_ar) || "—"}</strong>
              {arabicEnabled && secondaryName(a.name, a.name_ar) && <span className="muted">· {secondaryName(a.name, a.name_ar)}</span>}
              <span className="muted">· {a.email}</span>
              {a.totp_enabled && <span className="mfa-badge" dir="ltr" title={t("admin.mfaOn")}><FaShieldAlt /> {t("admin.mfaBadge")}</span>}
            </span>
            <IconButton danger title={t("admin.deleteAdmin")} onClick={() => remove(a)}><FaTrash /></IconButton>
          </li>
        ))}
        {admins.length === 0 && <li className="muted">{t("admin.noAdmins")}</li>}
      </ul>

      {showAdd && (
        <Modal onClose={closeAdd}>
          <h2 className="section-title">{t("admin.addAdmin")}</h2>
          <form className="add-form" onSubmit={handleAdd}>
            <Field label={t("common.name")}><input value={name} onChange={(e) => setName(e.target.value)} /></Field>
            {arabicEnabled && (
              <Field label={t("common.nameArabic")}><input value={nameAr} onChange={(e) => setNameAr(e.target.value)} dir="rtl" placeholder={t("common.namePlaceholderAr")} /></Field>
            )}
            <Field label={t("common.email")}><input type="email" value={email} onChange={(e) => setEmail(e.target.value)} required /></Field>
            <PasswordField label={t("common.password")} hint={t("common.atLeast12")} value={password} onChange={setPassword} autoComplete="new-password" required />
            {addError && <p className="error">{addError}</p>}
            <div className="form-actions">
              <Button type="submit" disabled={busy}>{busy ? t("admin.addingAdmin") : t("admin.addAdmin")}</Button>
              <Button type="button" variant="ghost" onClick={closeAdd}>{t("common.cancel")}</Button>
            </div>
          </form>
        </Modal>
      )}
    </div>
  );
}
