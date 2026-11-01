// Package api wires HTTP endpoints to the store and crypto layers.
package api

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/FayyazB106/account-manager/internal/backup"
	"github.com/FayyazB106/account-manager/internal/breach"
	"github.com/FayyazB106/account-manager/internal/report"
	"github.com/FayyazB106/account-manager/internal/store"
	"github.com/FayyazB106/account-manager/internal/update"
)

// Server holds the dependencies handlers need. Passing them in (rather than
// using globals) keeps things testable and explicit.
type Server struct {
	Admins      *store.AdminStore
	Accounts    *store.AccountStore
	Attachments *store.AttachmentStore
	Payments    *store.PaymentStore
	Categories  *store.CategoryStore
	People      *store.PersonStore
	Settings    *store.SettingStore
	Currencies  *store.CurrencyStore
	Audit       *store.AuditStore
	Refresh     *store.RefreshStore
	Backups     *backup.Manager
	Breach      *breach.Monitor
	Reports     *report.Service // SMTP config + monthly payment-history email
	JWTSecret   []byte
	KeyPath     string // sealed master-key file path (to detect setup vs locked state)

	// Version is the build version (e.g. "1.2.3"), "dev" for local builds. Desktop
	// is true when launched with -tray (an interactive desktop session) — only then
	// may an update be applied in-place; servers are notify-only. Updater checks
	// GitHub Releases (nil disables the feature).
	Version string
	Desktop bool
	Updater *update.Checker

	// MasterKey holds the key used for envelope encryption of account secrets. It
	// may be nil at startup (personal mode, before the browser Setup/Unlock step);
	// keyMu guards it because it can be set at runtime by /setup or /unlock.
	keyMu     sync.RWMutex
	MasterKey []byte

	// langSeen remembers the last language noted for each admin (see noteLanguage),
	// so the database is written only when it actually changes.
	langMu   sync.Mutex
	langSeen map[string]string
}

// ready reports whether the vault is unlocked (the master key is available).
func (s *Server) ready() bool {
	s.keyMu.RLock()
	defer s.keyMu.RUnlock()
	return len(s.MasterKey) > 0
}

// setMasterKey installs the master key at runtime (from /setup or /unlock).
func (s *Server) setMasterKey(k []byte) {
	s.keyMu.Lock()
	s.MasterKey = k
	s.keyMu.Unlock()
}

// MasterKeyValue returns the current master key (nil if locked) for background
// workers that captured the Server (e.g. the breach monitor).
func (s *Server) MasterKeyValue() []byte {
	s.keyMu.RLock()
	defer s.keyMu.RUnlock()
	return s.MasterKey
}

// Routes returns the HTTP handler with all routes registered.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	// Public routes.
	mux.HandleFunc("GET /health", s.health)
	mux.HandleFunc("GET /status", s.status)  // setup | locked | ready
	mux.HandleFunc("POST /setup", s.setup)   // first-run: set passphrase + first admin
	mux.HandleFunc("POST /unlock", s.unlock) // enter passphrase to unlock a sealed vault
	mux.HandleFunc("POST /register", s.register)
	mux.HandleFunc("POST /login", s.login)
	mux.HandleFunc("POST /login/totp", s.loginTOTP)
	mux.HandleFunc("POST /refresh", s.refresh) // rotates the refresh cookie → new access token
	mux.HandleFunc("POST /logout", s.logout)   // revokes this session's refresh token

	// Protected routes — require a valid session token.
	mux.HandleFunc("POST /accounts", s.requireAuth(s.createAccount))
	mux.HandleFunc("GET /accounts", s.requireAuth(s.listAccounts))
	mux.HandleFunc("GET /accounts/export", s.requireAuth(s.exportAccounts))
	mux.HandleFunc("PUT /accounts/{id}", s.requireAuth(s.updateAccount))
	mux.HandleFunc("DELETE /accounts/{id}", s.requireAuth(s.deleteAccount))
	mux.HandleFunc("PATCH /accounts/{id}/status", s.requireAuth(s.setAccountStatus))
	mux.HandleFunc("POST /accounts/{id}/extend", s.requireAuth(s.extendAccount))
	mux.HandleFunc("GET /accounts/secrets", s.requireAuth(s.revealAllSecrets))
	mux.HandleFunc("GET /accounts/{id}/reveal", s.requireAuth(s.revealAccount))
	mux.HandleFunc("GET /accounts/{id}/payments", s.requireAuth(s.listAccountPayments))
	mux.HandleFunc("POST /accounts/{id}/payments", s.requireAuth(s.createPayment))
	mux.HandleFunc("GET /payments", s.requireAuth(s.listAllPayments))
	mux.HandleFunc("PUT /payments/{id}", s.requireAuth(s.updatePayment))
	mux.HandleFunc("DELETE /payments/{id}", s.requireAuth(s.deletePayment))
	mux.HandleFunc("GET /accounts/{id}/history", s.requireAuth(s.listPasswordHistory))
	mux.HandleFunc("GET /accounts/{id}/history/{hid}/reveal", s.requireAuth(s.revealPasswordHistory))
	mux.HandleFunc("POST /accounts/{id}/totp", s.requireAuth(s.setAccountTOTP))
	mux.HandleFunc("DELETE /accounts/{id}/totp", s.requireAuth(s.clearAccountTOTP))
	mux.HandleFunc("GET /accounts/totp/codes", s.requireAuth(s.allAccountTOTPCodes))
	mux.HandleFunc("GET /accounts/{id}/totp/code", s.requireAuth(s.accountTOTPCode))
	mux.HandleFunc("GET /generate", s.requireAuth(s.generatePassword))

	mux.HandleFunc("GET /admins", s.requireAuth(s.listAdmins))
	mux.HandleFunc("POST /admins", s.requireAuth(s.createAdminHandler))
	mux.HandleFunc("DELETE /admins/{id}", s.requireAuth(s.deleteAdmin))
	mux.HandleFunc("GET /profile", s.requireAuth(s.getProfile))
	mux.HandleFunc("PUT /profile", s.requireAuth(s.updateProfile))
	mux.HandleFunc("POST /profile/password", s.requireAuth(s.changePassword))
	mux.HandleFunc("GET /profile/totp", s.requireAuth(s.totpStatus))
	mux.HandleFunc("POST /profile/totp/setup", s.requireAuth(s.setupTOTP))
	mux.HandleFunc("POST /profile/totp/enable", s.requireAuth(s.enableTOTP))
	mux.HandleFunc("POST /profile/totp/disable", s.requireAuth(s.disableTOTP))
	mux.HandleFunc("PUT /profile/monthly-report", s.requireAuth(s.setMonthlyReport))
	mux.HandleFunc("PUT /profile/payment-reminders", s.requireAuth(s.setPaymentReminders))
	mux.HandleFunc("PUT /profile/rotation-reminders", s.requireAuth(s.setRotationReminders))
	mux.HandleFunc("PUT /profile/breach-alerts", s.requireAuth(s.setBreachAlerts))
	mux.HandleFunc("PUT /profile/payment-reminder-days", s.requireAuth(s.setPaymentReminderDays))
	mux.HandleFunc("PUT /profile/rotation-reminder-days", s.requireAuth(s.setRotationReminderDays))
	mux.HandleFunc("PUT /profile/upcoming-payments", s.requireAuth(s.setUpcomingPayments))

	mux.HandleFunc("GET /smtp", s.requireAuth(s.getSMTP))
	mux.HandleFunc("PUT /smtp", s.requireAuth(s.saveSMTP))
	mux.HandleFunc("PUT /smtp/enabled", s.requireAuth(s.setSMTPEnabled))
	mux.HandleFunc("POST /smtp/test", s.requireAuth(s.testSMTP))
	mux.HandleFunc("POST /report/preview", s.requireAuth(s.previewReport))
	mux.HandleFunc("POST /reminder/preview", s.requireAuth(s.previewReminder))
	mux.HandleFunc("POST /rotation/preview", s.requireAuth(s.previewRotation))
	mux.HandleFunc("POST /breach-alert/preview", s.requireAuth(s.previewBreachAlert))
	mux.HandleFunc("POST /upcoming/preview", s.requireAuth(s.previewUpcoming))

	mux.HandleFunc("GET /categories", s.requireAuth(s.listCategories))
	mux.HandleFunc("POST /categories", s.requireAuth(s.createCategory))
	mux.HandleFunc("POST /categories/reorder", s.requireAuth(s.reorderCategories))
	mux.HandleFunc("PUT /categories/{id}", s.requireAuth(s.updateCategory))
	mux.HandleFunc("DELETE /categories/{id}", s.requireAuth(s.deleteCategory))

	mux.HandleFunc("GET /version", s.requireAuth(s.getVersion))        // current + latest release
	mux.HandleFunc("POST /update/check", s.requireAuth(s.checkUpdate)) // force a fresh check
	mux.HandleFunc("POST /update/apply", s.requireAuth(s.applyUpdate)) // desktop: download+verify+run installer

	mux.HandleFunc("GET /branding", s.branding) // public: login-screen org name + logo
	mux.HandleFunc("GET /settings", s.requireAuth(s.listSettings))
	mux.HandleFunc("PUT /settings/{key}", s.requireAuth(s.setSetting))
	mux.HandleFunc("POST /settings/purge-arabic", s.requireAuth(s.purgeArabic))

	mux.HandleFunc("GET /backups", s.requireAuth(s.listBackups))
	mux.HandleFunc("POST /backups", s.requireAuth(s.createBackup))
	mux.HandleFunc("GET /backups/{name}/download", s.requireAuth(s.downloadBackup))
	mux.HandleFunc("POST /backups/{name}/restore", s.requireAuth(s.restoreBackup))
	mux.HandleFunc("DELETE /backups/{name}", s.requireAuth(s.deleteBackup))

	mux.HandleFunc("GET /audit", s.requireAuth(s.listAudit))

	mux.HandleFunc("GET /breach", s.requireAuth(s.breachStatus))
	mux.HandleFunc("POST /breach/scan", s.requireAuth(s.scanBreaches))

	mux.HandleFunc("GET /currencies", s.requireAuth(s.listCurrencies))
	mux.HandleFunc("PATCH /currencies/{code}", s.requireAuth(s.setCurrency))

	mux.HandleFunc("GET /people", s.requireAuth(s.listPeople))
	mux.HandleFunc("POST /people", s.requireAuth(s.createPerson))
	mux.HandleFunc("POST /people/import", s.requireAuth(s.importPeople))
	mux.HandleFunc("POST /people/delete", s.requireAuth(s.deletePeople))
	mux.HandleFunc("PUT /people/{id}", s.requireAuth(s.updatePerson))
	mux.HandleFunc("DELETE /people/{id}", s.requireAuth(s.deletePerson))

	mux.HandleFunc("POST /accounts/{id}/attachments", s.requireAuth(s.uploadAttachment))
	mux.HandleFunc("GET /accounts/{id}/attachments", s.requireAuth(s.listAttachments))
	mux.HandleFunc("GET /accounts/{id}/attachments/export", s.requireAuth(s.exportAttachments))
	mux.HandleFunc("GET /attachments/{id}/download", s.requireAuth(s.downloadAttachment))
	mux.HandleFunc("DELETE /attachments/{id}", s.requireAuth(s.deleteAttachment))

	return noStore(s.withAudit(withCORS(s.gate(mux))))
}

// noStore marks every API response as uncacheable. This is a vault: no response
// (account lists, settings, secrets, session state) should ever be stored by the
// browser or an intermediary proxy — both for privacy and so a proxy (e.g. a
// Tailscale Funnel) can't serve stale data after a change. Static SPA assets are
// served outside /api and stay cacheable.
func noStore(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

// gate blocks all API routes until the vault is unlocked, except a small set
// needed to reach the setup/unlock/login screens. Prevents any secret-touching
// endpoint (or login) from running while the master key is unavailable.
func (s *Server) gate(next http.Handler) http.Handler {
	open := map[string]bool{
		"/health": true, "/status": true, "/setup": true, "/unlock": true, "/branding": true,
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.ready() || open[r.URL.Path] {
			next.ServeHTTP(w, r)
			return
		}
		writeError(w, http.StatusLocked, "not initialized")
	})
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "account-manager"})
}

// --- JSON helpers -----------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// writeError sends {error, code}. The English message stays for logging and as
// a fallback; the stable `code` (looked up from the message) lets the frontend
// show a translated string. No call site needs to change.
func writeError(w http.ResponseWriter, status int, msg string) {
	body := map[string]string{"error": msg}
	if code, ok := errorCodes[msg]; ok {
		body["code"] = code
	}
	writeJSON(w, status, body)
}

// errorCodes maps each English error message to a stable slug the client can
// translate. Keep in sync with `errors.*` keys in the web locale files.
var errorCodes = map[string]string{
	"USD is the base currency and can't be changed":           "usd_base_immutable",
	"a category with that name already exists":                "category_exists",
	"account has no payment date or billing period to extend": "extend_unavailable",
	"account must be inactive before it can be deleted":       "account_must_be_inactive",
	"account not found":                          "account_not_found",
	"admin not found":                            "admin_not_found",
	"an admin with that email already exists":    "admin_exists",
	"attachment not found":                       "attachment_not_found",
	"billing_period_months must be 0 or greater": "billing_invalid",
	"cannot delete a category that still has accounts; move or remove them first": "category_in_use",
	"cannot delete the last admin":  "last_admin",
	"category not found":            "category_not_found",
	"currency not found":            "currency_not_found",
	"current password is incorrect": "current_password_wrong",
	"email is required":             "email_required",
	"email required and password must be at least 12 characters": "email_and_password_required",
	"file too large or invalid form":                             "file_invalid",
	"internal error":                                             "internal",
	"invalid code":                                               "invalid_code",
	"invalid TOTP secret":                                        "totp_secret_invalid",
	"invalid email or password":                                  "invalid_credentials",
	"not initialized":                                            "not_initialized",
	"payment not found":                                          "payment_not_found",
	"invalid SMTP host":                                          "smtp_host_invalid",
	"invalid SMTP port":                                          "smtp_port_invalid",
	"invalid SMTP security mode":                                 "smtp_security_invalid",
	"invalid from address":                                       "smtp_from_invalid",
	"invalid SMTP credentials":                                   "smtp_credentials_invalid",
	"refusing to send credentials over an unencrypted connection": "smtp_insecure",
	"smtp password is required when a username is set":            "smtp_password_required",
	"email is not configured":                                     "email_not_configured",
	"email is turned off":                                         "email_disabled",
	"paid_on must be YYYY-MM-DD":                                  "paid_on_invalid",
	"amount is required":                                          "amount_required",
	"already initialized":                                         "already_initialized",
	"the vault is not locked":                                     "not_locked",
	"incorrect passphrase":                                        "incorrect_passphrase",
	"passphrase must be at least 8 characters":                    "passphrase_short",
	"two-factor is not set up":                                    "totp_not_setup",
	"invalid or expired token":                                    "token_invalid",
	"invalid request body":                                        "bad_request",
	"length must be an integer between 1 and 128":                 "length_invalid",
	"missing 'file' field":                                        "file_missing",
	"missing account id":                                          "account_id_missing",
	"missing or malformed authorization header":                   "auth_header_invalid",
	"name is required":                                            "name_required",
	"new password must be at least 12 characters":                 "new_password_short",
	"next_payment_date must be YYYY-MM-DD":                        "next_payment_invalid",
	"no attachments to export":                                    "no_attachments",
	"not authenticated":                                           "not_authenticated",
	"registration is closed; ask an existing admin to invite you": "registration_closed",
	"rotate_every_days must be positive":                          "rotate_invalid",
	"service and password are required":                           "service_password_required",
	"service is required":                                         "service_required",
	"status must be 'active' or 'inactive'":                       "status_invalid",
	"that email is already in use":                                "email_in_use",
	"too many attempts, try again later":                          "too_many_attempts",
	"user not found":                                              "user_not_found",
	"you cannot delete your own account":                          "cannot_delete_self",
}

// decodeJSON reads a JSON body into dst, rejecting bodies that are too large or
// contain unknown fields (defense against malformed / oversized input).
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	return decodeJSONLimit(w, r, dst, 1<<20) // 1 MiB cap
}

// decodeJSONLimit is decodeJSON with a caller-chosen size cap, for endpoints
// that legitimately accept larger bodies (e.g. a base64-encoded logo image).
func decodeJSONLimit(w http.ResponseWriter, r *http.Request, dst any, max int64) error {
	r.Body = http.MaxBytesReader(w, r.Body, max)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

// logErr logs server-side detail we deliberately do NOT expose to the client.
func logErr(context string, err error) {
	log.Printf("%s: %v", context, err)
}
