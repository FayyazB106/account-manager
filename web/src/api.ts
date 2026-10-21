// api.ts — a thin typed client for the Go backend.
//
// Auth (Option C): the ACCESS token lives only in memory here — never in
// localStorage/sessionStorage, so an XSS bug can't read it and a reload drops it.
// A long-lived REFRESH token lives in an httpOnly cookie the browser holds and JS
// can't touch. On startup (restoreSession) and whenever an access token expires
// (a 401), we silently POST /refresh to get a new access token and retry.

import i18n from "./i18n";

// Same-origin `/api` prefix: in production the Go binary serves the built SPA and
// the API together (see internal/webui). In dev, Vite proxies `/api` to the Go
// server on :8080 (see vite.config.ts), so there's no CORS and no hardcoded host.
const BASE = "/api";

// Access token — in-memory only.
let authToken: string | null = null;

export function setToken(token: string | null) {
  authToken = token;
}
export function hasToken() {
  return authToken !== null;
}

// Endpoints that must NOT trigger the auto-refresh-and-retry on a 401.
const AUTH_PATHS = new Set(["/login", "/login/totp", "/refresh", "/logout", "/status", "/setup", "/unlock"]);

// Single-flight refresh: many parallel requests hitting a 401 at once share one
// /refresh call rather than stampeding the endpoint (and rotating the cookie N times).
let refreshInFlight: Promise<boolean> | null = null;
function tryRefresh(): Promise<boolean> {
  if (!refreshInFlight) {
    refreshInFlight = fetch(`${BASE}/refresh`, { method: "POST", headers: { "Content-Type": "application/json" } })
      .then(async (res) => {
        if (!res.ok) return false;
        const body = await res.json().catch(() => ({}));
        if ((body as { token?: string }).token) { authToken = (body as { token: string }).token; return true; }
        return false;
      })
      .catch(() => false)
      .finally(() => { refreshInFlight = null; });
  }
  return refreshInFlight;
}

// restoreSession attempts to re-establish a session from the refresh cookie on
// app startup (the access token is gone after a reload). Returns whether it worked.
//
// The result is cached for the page's lifetime so it runs exactly ONCE, even if
// called multiple times — notably under React StrictMode, which double-invokes
// mount effects in dev. Two startup /refresh calls would otherwise rotate the
// refresh cookie twice; the second replays a now-spent token, which the server's
// reuse-detection treats as theft and revokes the session (a spurious logout on
// every reload in dev). One cached call avoids that entirely.
let restorePromise: Promise<boolean> | null = null;
export function restoreSession(): Promise<boolean> {
  if (!restorePromise) restorePromise = tryRefresh();
  return restorePromise;
}

// Core fetch wrapper: attaches the access token, transparently refreshes it once
// on a 401, and parses JSON / errors uniformly.
async function request<T>(path: string, options: RequestInit = {}, retried = false): Promise<T> {
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
    // The language the app is showing right now. The server notes it for the signed-in
    // admin so the emails sent to them (and "Send Now") are written in it.
    "X-App-Language": i18n.language === "ar" ? "ar" : "en",
    ...(options.headers as Record<string, string>),
  };
  if (authToken) headers["Authorization"] = `Bearer ${authToken}`;

  const res = await fetch(`${BASE}${path}`, { ...options, headers });

  // Access token expired/invalid: try a silent refresh once, then retry the call.
  if (res.status === 401 && !AUTH_PATHS.has(path)) {
    if (!retried && (await tryRefresh())) {
      return request<T>(path, options, true);
    }
    // Refresh failed → the session is really over: sign out and go to login.
    setToken(null);
    if (!window.location.pathname.startsWith("/login")) {
      window.location.assign("/login");
    }
  }

  const body = await res.json().catch(() => ({}));

  if (!res.ok) {
    // Prefer a translated message from the server's stable error code; fall back
    // to the server's English text, then a generic message.
    const { error, code } = body as { error?: string; code?: string };
    const key = code ? `errors.${code}` : "";
    const msg = key && i18n.exists(key) ? i18n.t(key) : (error || `request failed (${res.status})`);
    throw new Error(msg);
  }
  return body as T;
}

// --- Types -----------------------------------------------------------------

export interface AccountListItem {
  id: string;
  category_id?: string;
  category_name?: string;
  category_icon?: string;
  service: string;
  website?: string;
  login?: string;
  billing_period_months: number;
  amount?: number;
  next_payment_date?: string;
  password_changed_at: string;
  rotate_every_days?: number;
  status: string;
  notes?: string;
  has_totp?: boolean;
  breach_count?: number;
  breach_checked_at?: string;
  seats: string[];
  seats_ar?: (string | null)[];
}

export interface GenerateResult {
  password: string;
  entropy_bits: number;
  strength: string;
}

export interface NewAccount {
  category_id?: string | null;
  service: string;
  website?: string;
  login?: string;
  password: string;
  billing_period_months?: number;
  amount?: number | null;
  next_payment_date?: string;
  notes?: string;
  rotate_every_days?: number;
  seats?: string[];
  seats_ar?: (string | null)[];
}

// --- Endpoints -------------------------------------------------------------

// LoginResult is either a completed session (token set) or a request to provide
// a second factor (MFA is enabled for this admin).
export type LoginResult = { kind: "session" } | { kind: "mfa"; mfaToken: string };

export async function login(email: string, password: string): Promise<LoginResult> {
  const res = await request<{ token?: string; mfa_required?: boolean; mfa_token?: string }>("/login", {
    method: "POST",
    body: JSON.stringify({ email, password }),
  });
  if (res.mfa_required && res.mfa_token) return { kind: "mfa", mfaToken: res.mfa_token };
  if (res.token) setToken(res.token);
  return { kind: "session" };
}

// loginTOTP exchanges the MFA-pending token + a code (authenticator or backup)
// for a session token.
export async function loginTOTP(mfaToken: string, code: string): Promise<void> {
  const { token } = await request<{ token: string }>("/login/totp", {
    method: "POST",
    body: JSON.stringify({ mfa_token: mfaToken, code }),
  });
  setToken(token);
}

export function logout() {
  // Best-effort: revoke the refresh token + clear its cookie server-side.
  fetch(`${BASE}/logout`, { method: "POST", headers: { "Content-Type": "application/json" } }).catch(() => {});
  setToken(null);
}

// --- First-run setup / unlock ----------------------------------------------

// AppState drives onboarding: "setup" = first run (choose passphrase + first
// admin), "locked" = enter passphrase to unseal, "ready" = normal login/app.
export type AppState = "setup" | "locked" | "ready";

export async function getAppState(): Promise<AppState> {
  try {
    const res = await fetch(`${BASE}/status`);
    const body = await res.json().catch(() => ({}));
    return ((body as { state?: AppState }).state) ?? "ready";
  } catch {
    return "ready";
  }
}

// setupApp performs first-run setup (seal key under a passphrase + create the
// first admin) and logs the new admin in.
export async function setupApp(passphrase: string, email: string, password: string): Promise<void> {
  const { token } = await request<{ token: string }>("/setup", {
    method: "POST",
    body: JSON.stringify({ passphrase, email, password }),
  });
  setToken(token);
}

// unlockVault unseals an existing vault with the passphrase (does not log in).
export async function unlockVault(passphrase: string): Promise<void> {
  await request("/unlock", { method: "POST", body: JSON.stringify({ passphrase }) });
}

// --- Two-factor (TOTP) enrollment (Settings → Admin) ---
export function totpStatus() {
  return request<{ enabled: boolean; backup_codes_remaining: number }>("/profile/totp");
}
export function totpSetup() {
  return request<{ secret: string; otpauth_url: string }>("/profile/totp/setup", { method: "POST" });
}
export function totpEnable(code: string) {
  return request<{ backup_codes: string[] }>("/profile/totp/enable", {
    method: "POST",
    body: JSON.stringify({ code }),
  });
}
export function totpDisable(code: string) {
  return request<{ status: string }>("/profile/totp/disable", {
    method: "POST",
    body: JSON.stringify({ code }),
  });
}

export function listAccounts() {
  return request<AccountListItem[]>("/accounts");
}

export function createAccount(a: NewAccount) {
  return request<{ id: string }>("/accounts", {
    method: "POST",
    body: JSON.stringify(a),
  });
}

export function updateAccount(id: string, a: NewAccount) {
  return request<{ status: string }>(`/accounts/${id}`, {
    method: "PUT",
    body: JSON.stringify(a),
  });
}

export function extendAccount(id: string) {
  return request<{ next_payment_date: string }>(`/accounts/${id}/extend`, { method: "POST" });
}
export function setAccountStatus(id: string, status: "active" | "inactive") {
  return request<{ status: string }>(`/accounts/${id}/status`, {
    method: "PATCH",
    body: JSON.stringify({ status }),
  });
}

export async function deleteAccount(id: string): Promise<void> {
  await request<unknown>(`/accounts/${id}`, { method: "DELETE" });
}

export async function revealPassword(id: string): Promise<string> {
  const { password } = await request<{ password: string }>(`/accounts/${id}/reveal`);
  return password;
}

// revealAllSecrets returns { accountId: password } for the Vault Health scan
// (one request, logged once — not a reveal per account).
export function revealAllSecrets() {
  return request<Record<string, string>>("/accounts/secrets");
}

// --- Breach monitoring ---
export function getBreachStatus() {
  return request<{ breached: number; last_check: string }>("/breach");
}
export function scanBreaches() {
  return request<{ checked: number }>("/breach/scan", { method: "POST" });
}

// --- Account TOTP (2FA codes for saved services) ---
export function accountTOTPCode(accountId: string) {
  return request<{ code: string; expires_in: number }>(`/accounts/${accountId}/totp/code`);
}
// allAccountTOTPCodes returns { accountId: {code, expires_in} } for every account
// with a stored 2FA secret (the authenticator list).
export function allAccountTOTPCodes() {
  return request<Record<string, { code: string; expires_in: number }>>("/accounts/totp/codes");
}
export function setAccountTOTP(accountId: string, secret: string) {
  return request<{ status: string }>(`/accounts/${accountId}/totp`, {
    method: "POST",
    body: JSON.stringify({ secret }),
  });
}
export function clearAccountTOTP(accountId: string) {
  return request<{ status: string }>(`/accounts/${accountId}/totp`, { method: "DELETE" });
}

export interface PasswordHistoryEntry {
  id: string;
  changed_at: string;
}
export function listPasswordHistory(accountId: string) {
  return request<PasswordHistoryEntry[]>(`/accounts/${accountId}/history`);
}
export async function revealPasswordHistory(accountId: string, historyId: string): Promise<string> {
  const { password } = await request<{ password: string }>(`/accounts/${accountId}/history/${historyId}/reveal`);
  return password;
}

export function generatePassword(length = 20) {
  return request<GenerateResult>(`/generate?length=${length}`);
}

// --- Payments (ledger) -----------------------------------------------------

export interface Payment {
  id: string;
  account_id: string;
  paid_on: string; // "YYYY-MM-DD" (may arrive as a full ISO timestamp — slice to 10)
  amount: number;
  currency: string;
  note?: string;
  attachment_id?: string;
  source: "manual" | "auto";
  created_at: string;
  attachment_name?: string; // linked invoice filename, if any
}

// PaymentListItem is a payment plus the joined account fields the global ledger
// needs for filtering/display across accounts.
export interface PaymentListItem extends Payment {
  service: string;
  category_id?: string;
  category_name?: string;
  category_icon?: string;
  billing_period_months: number;
  seats: string[];
  seats_ar?: (string | null)[];
}

export interface PaymentInput {
  paid_on: string;
  amount: number;
  currency: string;
  note?: string;
  attachment_id?: string;
}

export function listAccountPayments(accountId: string) {
  return request<Payment[]>(`/accounts/${accountId}/payments`);
}
export function addPayment(accountId: string, p: PaymentInput) {
  return request<{ id: string }>(`/accounts/${accountId}/payments`, { method: "POST", body: JSON.stringify(p) });
}
export function updatePayment(id: string, p: PaymentInput) {
  return request<{ status: string }>(`/payments/${id}`, { method: "PUT", body: JSON.stringify(p) });
}
export async function deletePayment(id: string): Promise<void> {
  await request<unknown>(`/payments/${id}`, { method: "DELETE" });
}
export function listAllPayments() {
  return request<PaymentListItem[]>("/payments");
}

// --- Admins / profile ------------------------------------------------------

export interface Admin {
  id: string;
  email: string;
  name: string;
  name_ar?: string;
  totp_enabled?: boolean;
  monthly_report?: boolean; // gets last month's payment history by email
  payment_reminders?: boolean; // gets daily reminders while a payment is due within 7 days or overdue
  breach_alerts?: boolean; // emailed right away when a scan finds a newly breached password
  rotation_reminders?: boolean; // gets daily reminders while a password is due for rotation within 7 days or overdue
  upcoming_payments?: boolean; // gets the month's upcoming payments by email on the 1st
  language?: "en" | "ar"; // UI language saved on the server; emails to this admin follow it
  created_at: string;
}

// --- Email (SMTP) + monthly payment report ---------------------------------

export type SMTPSecurity = "tls" | "starttls" | "none";

// SMTPConfig is the stored configuration. The password is never returned —
// has_password only says whether one is saved.
export interface SMTPConfig {
  host: string;
  port: number;
  security: SMTPSecurity | "";
  username: string;
  from: string;
  has_password: boolean;
  configured: boolean;
  enabled: boolean; // master switch: nothing is sent while off (settings are kept)
}
export interface SMTPInput {
  host: string;
  port: number;
  security: SMTPSecurity;
  username: string;
  from: string;
  password?: string; // omit/empty to keep the saved password
}
export function getSMTP() {
  return request<SMTPConfig>("/smtp");
}
export function saveSMTP(input: SMTPInput) {
  return request<{ status: string }>("/smtp", { method: "PUT", body: JSON.stringify(input) });
}
export function setSMTPEnabled(enabled: boolean) {
  return request<{ enabled: boolean }>("/smtp/enabled", { method: "PUT", body: JSON.stringify({ enabled }) });
}
export function sendTestEmail() {
  return request<{ sent_to: string }>("/smtp/test", { method: "POST" });
}
// Emails last month's report to the signed-in admin right now (doesn't affect the
// real monthly schedule).
export function sendReportPreview() {
  return request<{ sent_to: string; payments: number }>("/report/preview", { method: "POST" });
}
// Emails the current due-payment reminder to the signed-in admin right now. When
// nothing is due, nothing is sent and sent is false.
export function sendReminderPreview() {
  return request<{ sent_to: string; payments: number; sent: boolean }>("/reminder/preview", { method: "POST" });
}
// Emails the current password-rotation reminder to the signed-in admin right now.
// When nothing is due, nothing is sent and sent is false.
export function sendRotationPreview() {
  return request<{ sent_to: string; passwords: number; sent: boolean }>("/rotation/preview", { method: "POST" });
}
// Emails this month's upcoming payments (the Monthly Costs list) to the signed-in
// admin right now. When nothing is scheduled, nothing is sent and sent is false.
export function sendUpcomingPreview() {
  return request<{ sent_to: string; payments: number; sent: boolean }>("/upcoming/preview", { method: "POST" });
}
export function setUpcomingPayments(enabled: boolean) {
  return request<{ enabled: boolean }>("/profile/upcoming-payments", { method: "PUT", body: JSON.stringify({ enabled }) });
}
// Emails the passwords currently flagged as breached to the signed-in admin right
// now. When none are flagged, nothing is sent and sent is false.
export function sendBreachAlertPreview() {
  return request<{ sent_to: string; passwords: number; sent: boolean }>("/breach-alert/preview", { method: "POST" });
}
export function setBreachAlerts(enabled: boolean) {
  return request<{ enabled: boolean }>("/profile/breach-alerts", { method: "PUT", body: JSON.stringify({ enabled }) });
}
export function setRotationReminders(enabled: boolean) {
  return request<{ enabled: boolean }>("/profile/rotation-reminders", { method: "PUT", body: JSON.stringify({ enabled }) });
}
export function setPaymentReminders(enabled: boolean) {
  return request<{ enabled: boolean }>("/profile/payment-reminders", { method: "PUT", body: JSON.stringify({ enabled }) });
}
export function setMonthlyReport(enabled: boolean) {
  return request<{ enabled: boolean }>("/profile/monthly-report", { method: "PUT", body: JSON.stringify({ enabled }) });
}

export interface AuditEntry {
  id: string;
  at: string;
  actor_email: string;
  action: string;
  target: string;
  detail: string;
  ip: string;
  result: string;
}
export function listAudit(params: { actor?: string; action?: string; limit?: number; offset?: number }) {
  const q = new URLSearchParams();
  if (params.actor) q.set("actor", params.actor);
  if (params.action) q.set("action", params.action);
  q.set("limit", String(params.limit ?? 100));
  q.set("offset", String(params.offset ?? 0));
  return request<{ entries: AuditEntry[]; total: number }>(`/audit?${q.toString()}`);
}

export function listAdmins() {
  return request<Admin[]>("/admins");
}
export function addAdmin(email: string, name: string, password: string, name_ar?: string) {
  return request<Admin>("/admins", { method: "POST", body: JSON.stringify({ email, name, name_ar, password }) });
}
export async function deleteAdmin(id: string): Promise<void> {
  await request<unknown>(`/admins/${id}`, { method: "DELETE" });
}
export function getProfile() {
  return request<Admin>("/profile");
}
export function updateProfile(name: string, email: string, name_ar?: string) {
  return request<{ status: string }>("/profile", { method: "PUT", body: JSON.stringify({ name, name_ar, email }) });
}
export function changePassword(current_password: string, new_password: string) {
  return request<{ status: string }>("/profile/password", {
    method: "POST",
    body: JSON.stringify({ current_password, new_password }),
  });
}

// --- Settings (shared) -----------------------------------------------------

export function getSettings() {
  return request<Record<string, string>>("/settings");
}
export function setSetting(key: string, value: string) {
  return request<{ status: string }>(`/settings/${key}`, { method: "PUT", body: JSON.stringify({ value }) });
}

// --- Updates ---------------------------------------------------------------

export interface VersionInfo {
  current: string;
  latest: string;
  updateAvailable: boolean;
  url: string;
  notes: string;
  checked: string;
  canApply: boolean; // true = the app can download+run the update itself (desktop)
  os: string;        // "windows" | "darwin" | "linux" — for platform-specific messaging
}
export function getVersion() {
  return request<VersionInfo>("/version");
}
export function checkUpdate() {
  return request<VersionInfo>("/update/check", { method: "POST" });
}
export function applyUpdate() {
  return request<{ status: string }>("/update/apply", { method: "POST" });
}
// --- Backups ---------------------------------------------------------------

export interface BackupInfo {
  name: string;
  size: number;
  created_at: string;
}
export function listBackups() {
  return request<BackupInfo[]>("/backups");
}
export function createBackup() {
  return request<{ name: string }>("/backups", { method: "POST" });
}
export function restoreBackup(name: string) {
  return request<{ status: string }>(`/backups/${encodeURIComponent(name)}/restore`, { method: "POST" });
}
export async function deleteBackup(name: string): Promise<void> {
  await request<unknown>(`/backups/${encodeURIComponent(name)}`, { method: "DELETE" });
}
export async function downloadBackup(name: string) {
  const res = await fetch(`${BASE}/backups/${encodeURIComponent(name)}/download`, {
    headers: authToken ? { Authorization: `Bearer ${authToken}` } : {},
  });
  if (!res.ok) throw new Error(`download failed (${res.status})`);
  const blob = await res.blob();
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = name;
  a.click();
  URL.revokeObjectURL(url);
}

// Public org branding (name + logo) for the login screen — no auth required.
export function getBranding() {
  return request<{ org_name: string; org_logo: string }>("/branding");
}
// Permanently clear all Arabic display names (categories, seats, admins).
export function purgeArabic() {
  return request<{ status: string }>("/settings/purge-arabic", { method: "POST" });
}

// --- Currencies ------------------------------------------------------------

export interface Currency {
  code: string;
  name: string;
  active: boolean;
  rate: number;
  updated_at?: string;
}

export function listCurrencies() {
  return request<Currency[]>("/currencies");
}
export function setCurrencyActive(code: string, active: boolean) {
  return request<{ active: boolean }>(`/currencies/${code}`, { method: "PATCH", body: JSON.stringify({ active }) });
}

// --- People (seat users) ---------------------------------------------------

export interface Person {
  id: string;
  name: string;
  name_ar?: string;
}

export function listPeople() {
  return request<Person[]>("/people");
}
export function addPerson(name: string, name_ar?: string) {
  return request<Person>("/people", { method: "POST", body: JSON.stringify({ name, name_ar }) });
}
export function updatePerson(id: string, name: string, name_ar?: string) {
  return request<{ status: string }>(`/people/${id}`, { method: "PUT", body: JSON.stringify({ name, name_ar }) });
}
export async function deletePerson(id: string): Promise<void> {
  await request<unknown>(`/people/${id}`, { method: "DELETE" });
}
export function deletePeople(ids: string[]) {
  return request<{ deleted: number }>("/people/delete", { method: "POST", body: JSON.stringify({ ids }) });
}
export function importPeople(people: { name: string; name_ar?: string }[]) {
  return request<{ added: number }>("/people/import", { method: "POST", body: JSON.stringify({ people }) });
}

// --- Categories ------------------------------------------------------------

export interface Category {
  id: string;
  name: string;
  name_ar?: string;
  icon: string;
}

export function listCategories() {
  return request<Category[]>("/categories");
}
export function createCategory(name: string, icon: string, name_ar?: string) {
  return request<Category>("/categories", { method: "POST", body: JSON.stringify({ name, name_ar, icon }) });
}
export function updateCategory(id: string, name: string, icon: string, name_ar?: string) {
  return request<{ status: string }>(`/categories/${id}`, { method: "PUT", body: JSON.stringify({ name, name_ar, icon }) });
}
export async function deleteCategory(id: string): Promise<void> {
  await request<unknown>(`/categories/${id}`, { method: "DELETE" });
}
export async function reorderCategories(ids: string[]): Promise<void> {
  await request<unknown>("/categories/reorder", { method: "POST", body: JSON.stringify({ ids }) });
}

// --- Attachments -----------------------------------------------------------

export interface AttachmentMeta {
  id: string;
  filename: string;
  mime_type: string;
  created_at: string;
}

export function listAttachments(accountId: string) {
  return request<AttachmentMeta[]>(`/accounts/${accountId}/attachments`);
}

// Upload uses FormData/multipart — we must NOT set Content-Type ourselves, so
// the browser can add the correct multipart boundary. Hence raw fetch here.
export async function uploadAttachment(accountId: string, file: File) {
  const form = new FormData();
  form.append("file", file);
  const res = await fetch(`${BASE}/accounts/${accountId}/attachments`, {
    method: "POST",
    headers: authToken ? { Authorization: `Bearer ${authToken}` } : {},
    body: form,
  });
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error((body as { error?: string }).error || `upload failed (${res.status})`);
  }
  return res.json();
}

export async function deleteAttachment(id: string): Promise<void> {
  await request<unknown>(`/attachments/${id}`, { method: "DELETE" });
}

// Export all attachments of an account as a single .zip.
export async function exportAttachments(accountId: string) {
  const res = await fetch(`${BASE}/accounts/${accountId}/attachments/export`, {
    headers: authToken ? { Authorization: `Bearer ${authToken}` } : {},
  });
  if (!res.ok) throw new Error(`export failed (${res.status})`);
  const blob = await res.blob();
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = "attachments.zip";
  a.click();
  URL.revokeObjectURL(url);
}

// Export all account data as an Excel workbook (one sheet per category). When
// withAttachments is true, the server returns a .zip of the workbook plus a
// folder of decrypted files per account.
export async function exportAccounts(withAttachments: boolean) {
  const res = await fetch(`${BASE}/accounts/export?attachments=${withAttachments}`, {
    headers: authToken ? { Authorization: `Bearer ${authToken}` } : {},
  });
  if (!res.ok) throw new Error(`export failed (${res.status})`);
  const blob = await res.blob();
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = withAttachments ? "accounts-export.zip" : "accounts.xlsx";
  a.click();
  URL.revokeObjectURL(url);
}

// fetchAttachmentBlob returns the decrypted bytes of an attachment as a Blob,
// without triggering a save (used to bundle invoices into an export zip).
export async function fetchAttachmentBlob(id: string): Promise<Blob> {
  const res = await fetch(`${BASE}/attachments/${id}/download`, {
    headers: authToken ? { Authorization: `Bearer ${authToken}` } : {},
  });
  if (!res.ok) throw new Error(`download failed (${res.status})`);
  return res.blob();
}

// Download fetches the decrypted bytes as a Blob and triggers a browser save.
export async function downloadAttachment(id: string, filename: string) {
  const res = await fetch(`${BASE}/attachments/${id}/download`, {
    headers: authToken ? { Authorization: `Bearer ${authToken}` } : {},
  });
  if (!res.ok) throw new Error(`download failed (${res.status})`);
  const blob = await res.blob();
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  a.click();
  URL.revokeObjectURL(url);
}
