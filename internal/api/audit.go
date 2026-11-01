package api

import (
	"context"
	"net"
	"net/http"
	"regexp"
	"strings"

	"github.com/FayyazB106/account-manager/internal/store"
)

// This file implements audit logging as middleware: every security-relevant
// request (mutations, secret reveals, exports, downloads) is recorded once,
// centrally, with the actor, action, target path, client IP, and result. Logins
// are recorded explicitly in the auth handlers so the attempted email is kept.

// auditActor is a per-request holder the auth middleware fills in with the
// authenticated admin ID, so the outer audit middleware can attribute the action
// (context values set inside a handler don't propagate back out — a pointer does).
type auditActor struct{ adminID string }

type auditKeyT struct{}

var auditKey = auditKeyT{}

func auditActorFrom(ctx context.Context) (*auditActor, bool) {
	a, ok := ctx.Value(auditKey).(*auditActor)
	return a, ok
}

// statusRecorder captures the response status so we can record ok/fail.
type statusRecorder struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.wrote = true
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if !r.wrote {
		r.status = http.StatusOK
		r.wrote = true
	}
	return r.ResponseWriter.Write(b)
}

// auditRoute maps a method + path pattern to a stable action name.
type auditRoute struct {
	method string
	re     *regexp.Regexp
	action string
}

// auditRoutes lists every path we audit. Login/register are handled explicitly
// in the auth handlers, so they're not here. GET list endpoints are intentionally
// omitted (reading the list of accounts isn't itself sensitive; revealing a
// secret is). Order: specific before parameterized.
var auditRoutes = []auditRoute{
	{"POST", regexp.MustCompile(`^/accounts$`), "account.create"},
	{"GET", regexp.MustCompile(`^/accounts/secrets$`), "vault.scan"},
	{"GET", regexp.MustCompile(`^/accounts/export$`), "account.export"},
	{"GET", regexp.MustCompile(`^/accounts/[^/]+/reveal$`), "account.reveal"},
	{"GET", regexp.MustCompile(`^/accounts/[^/]+/history/[^/]+/reveal$`), "account.history_reveal"},
	{"PATCH", regexp.MustCompile(`^/accounts/[^/]+/status$`), "account.status"},
	{"POST", regexp.MustCompile(`^/accounts/[^/]+/extend$`), "account.extend"},
	{"POST", regexp.MustCompile(`^/accounts/[^/]+/payments$`), "payment.add"},
	{"PUT", regexp.MustCompile(`^/payments/[^/]+$`), "payment.update"},
	{"DELETE", regexp.MustCompile(`^/payments/[^/]+$`), "payment.delete"},
	{"POST", regexp.MustCompile(`^/accounts/[^/]+/totp$`), "account.totp_set"},
	{"DELETE", regexp.MustCompile(`^/accounts/[^/]+/totp$`), "account.totp_clear"},
	{"POST", regexp.MustCompile(`^/accounts/[^/]+/attachments$`), "attachment.upload"},
	{"GET", regexp.MustCompile(`^/accounts/[^/]+/attachments/export$`), "attachment.export"},
	{"PUT", regexp.MustCompile(`^/accounts/[^/]+$`), "account.update"},
	{"DELETE", regexp.MustCompile(`^/accounts/[^/]+$`), "account.delete"},
	{"GET", regexp.MustCompile(`^/attachments/[^/]+/download$`), "attachment.download"},
	{"DELETE", regexp.MustCompile(`^/attachments/[^/]+$`), "attachment.delete"},
	{"POST", regexp.MustCompile(`^/admins$`), "admin.create"},
	{"DELETE", regexp.MustCompile(`^/admins/[^/]+$`), "admin.delete"},
	{"PUT", regexp.MustCompile(`^/profile$`), "profile.update"},
	{"POST", regexp.MustCompile(`^/profile/password$`), "profile.password"},
	{"POST", regexp.MustCompile(`^/profile/totp/setup$`), "mfa.setup"},
	{"POST", regexp.MustCompile(`^/profile/totp/enable$`), "mfa.enable"},
	{"POST", regexp.MustCompile(`^/profile/totp/disable$`), "mfa.disable"},
	{"PUT", regexp.MustCompile(`^/profile/monthly-report$`), "profile.monthly_report"},
	{"PUT", regexp.MustCompile(`^/profile/payment-reminders$`), "profile.payment_reminders"},
	{"PUT", regexp.MustCompile(`^/profile/rotation-reminders$`), "profile.rotation_reminders"},
	{"PUT", regexp.MustCompile(`^/profile/breach-alerts$`), "profile.breach_alerts"},
	{"PUT", regexp.MustCompile(`^/profile/payment-reminder-days$`), "profile.payment_reminder_days"},
	{"PUT", regexp.MustCompile(`^/profile/rotation-reminder-days$`), "profile.rotation_reminder_days"},
	{"PUT", regexp.MustCompile(`^/profile/upcoming-payments$`), "profile.upcoming_payments"},
	{"POST", regexp.MustCompile(`^/reminder/preview$`), "reminder.preview"},
	{"POST", regexp.MustCompile(`^/rotation/preview$`), "rotation.preview"},
	{"POST", regexp.MustCompile(`^/breach-alert/preview$`), "breach_alert.preview"},
	{"POST", regexp.MustCompile(`^/upcoming/preview$`), "upcoming.preview"},
	{"PUT", regexp.MustCompile(`^/smtp$`), "smtp.update"},
	{"PUT", regexp.MustCompile(`^/smtp/enabled$`), "smtp.toggle"},
	{"POST", regexp.MustCompile(`^/smtp/test$`), "smtp.test"},
	{"POST", regexp.MustCompile(`^/report/preview$`), "report.preview"},
	{"POST", regexp.MustCompile(`^/categories$`), "category.create"},
	{"POST", regexp.MustCompile(`^/categories/reorder$`), "category.reorder"},
	{"PUT", regexp.MustCompile(`^/categories/[^/]+$`), "category.update"},
	{"DELETE", regexp.MustCompile(`^/categories/[^/]+$`), "category.delete"},
	{"POST", regexp.MustCompile(`^/settings/purge-arabic$`), "settings.purge_arabic"},
	{"PUT", regexp.MustCompile(`^/settings/[^/]+$`), "settings.update"},
	{"POST", regexp.MustCompile(`^/people$`), "person.create"},
	{"POST", regexp.MustCompile(`^/people/import$`), "person.import"},
	{"POST", regexp.MustCompile(`^/people/delete$`), "person.delete_bulk"},
	{"PUT", regexp.MustCompile(`^/people/[^/]+$`), "person.update"},
	{"DELETE", regexp.MustCompile(`^/people/[^/]+$`), "person.delete"},
	{"POST", regexp.MustCompile(`^/backups$`), "backup.create"},
	{"POST", regexp.MustCompile(`^/backups/[^/]+/restore$`), "backup.restore"},
	{"GET", regexp.MustCompile(`^/backups/[^/]+/download$`), "backup.download"},
	{"DELETE", regexp.MustCompile(`^/backups/[^/]+$`), "backup.delete"},
	{"PATCH", regexp.MustCompile(`^/currencies/[^/]+$`), "settings.currency"},
	{"POST", regexp.MustCompile(`^/breach/scan$`), "breach.scan"},
	{"POST", regexp.MustCompile(`^/update/apply$`), "system.update"},
}

func auditable(method, path string) (string, bool) {
	for _, rt := range auditRoutes {
		if rt.method == method && rt.re.MatchString(path) {
			return rt.action, true
		}
	}
	return "", false
}

// clientIP returns the best-guess client address, honoring X-Forwarded-For set
// by a reverse proxy (first hop), else the direct connection address.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return strings.TrimSpace(strings.Split(xff, ",")[0])
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

// withAudit records one audit entry per matched request, after it completes.
func (s *Server) withAudit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		action, ok := auditable(r.Method, r.URL.Path)
		if !ok {
			next.ServeHTTP(w, r)
			return
		}
		actor := &auditActor{}
		r = r.WithContext(context.WithValue(r.Context(), auditKey, actor))
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)

		result := "ok"
		if rec.status >= 400 {
			result = "fail"
		}
		email := ""
		if actor.adminID != "" {
			if a, err := s.Admins.GetByID(context.Background(), actor.adminID); err == nil {
				email = a.Email
			}
		}
		if err := s.Audit.Insert(context.Background(), store.AuditEntry{
			ActorEmail: email,
			Action:     action,
			Target:     r.URL.Path,
			IP:         clientIP(r),
			Result:     result,
		}); err != nil {
			logErr("audit insert", err)
		}
	})
}

// auditRaw records an explicit entry (used by the auth handlers for logins).
func (s *Server) auditRaw(r *http.Request, email, action, target, detail, result string) {
	if err := s.Audit.Insert(context.Background(), store.AuditEntry{
		ActorEmail: email,
		Action:     action,
		Target:     target,
		Detail:     detail,
		IP:         clientIP(r),
		Result:     result,
	}); err != nil {
		logErr("audit insert", err)
	}
}
