package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/FayyazB106/account-manager/internal/report"
	"github.com/FayyazB106/account-manager/internal/store"
)

// smtpInput is the JSON body for saving the email (SMTP) configuration. An empty
// password keeps the stored one; the stored password is never sent back.
type smtpInput struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Security string `json:"security"`
	Username string `json:"username"`
	From     string `json:"from"`
	Password string `json:"password"`
}

// writeReportError maps report/mailer errors to HTTP responses. Validation and
// "not configured" problems are the caller's to fix (400); anything else is a
// delivery failure, whose message (e.g. "smtp login: 535 ...") helps the admin fix
// their provider settings, so it is returned to the (authenticated) admin.
func writeReportError(w http.ResponseWriter, context string, err error) {
	var ve *report.ValidationError
	switch {
	case errors.As(err, &ve):
		writeError(w, http.StatusBadRequest, ve.Msg)
	case errors.Is(err, report.ErrNotConfigured):
		writeError(w, http.StatusBadRequest, report.ErrNotConfigured.Error())
	case errors.Is(err, report.ErrDisabled):
		writeError(w, http.StatusBadRequest, report.ErrDisabled.Error())
	default:
		logErr(context, err)
		writeError(w, http.StatusBadGateway, "could not send the email: "+err.Error())
	}
}

// getSMTP handles GET /smtp — the stored configuration, without the password.
func (s *Server) getSMTP(w http.ResponseWriter, r *http.Request) {
	cur, err := s.Reports.LoadSMTP(r.Context())
	if err != nil {
		logErr("getSMTP", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, cur)
}

// saveSMTP handles PUT /smtp.
func (s *Server) saveSMTP(w http.ResponseWriter, r *http.Request) {
	var in smtpInput
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	err := s.Reports.SaveSMTP(r.Context(), report.SMTPInput{
		Host: in.Host, Port: in.Port, Security: in.Security,
		Username: in.Username, From: in.From, Password: in.Password,
	})
	if err != nil {
		var ve *report.ValidationError
		if errors.As(err, &ve) {
			writeError(w, http.StatusBadRequest, ve.Msg)
			return
		}
		logErr("saveSMTP", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "saved"})
}

// setSMTPEnabled handles PUT /smtp/enabled with body {"enabled": bool} — the master
// on/off switch. Turning it off keeps the saved settings but stops all sending.
func (s *Server) setSMTPEnabled(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := s.Reports.SetEnabled(r.Context(), body.Enabled); err != nil {
		logErr("setSMTPEnabled", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"enabled": body.Enabled})
}

// currentAdmin returns the signed-in admin's id, name and email.
func (s *Server) currentAdmin(w http.ResponseWriter, r *http.Request) (id, name, email string, ok bool) {
	adminID, found := adminIDFrom(r.Context())
	if !found {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return "", "", "", false
	}
	a, err := s.Admins.GetByID(r.Context(), adminID)
	if err != nil {
		logErr("currentAdmin", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return "", "", "", false
	}
	return a.ID, a.Name, a.Email, true
}

// currentRecipient is the signed-in admin as an email recipient: their address, the
// name to greet them with, and the language to write in (their own UI language).
func (s *Server) currentRecipient(w http.ResponseWriter, r *http.Request) (report.Recipient, bool) {
	adminID, found := adminIDFrom(r.Context())
	if !found {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return report.Recipient{}, false
	}
	a, err := s.Admins.GetByID(r.Context(), adminID)
	if err != nil {
		logErr("currentRecipient", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return report.Recipient{}, false
	}
	return s.Reports.RecipientForAdmin(r.Context(), a), true
}

// testSMTP handles POST /smtp/test — a short test message to the signed-in admin.
func (s *Server) testSMTP(w http.ResponseWriter, r *http.Request) {
	to, ok := s.currentRecipient(w, r)
	if !ok {
		return
	}
	if err := s.Reports.SendTest(r.Context(), to); err != nil {
		writeReportError(w, "testSMTP", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"sent_to": to.Email})
}

// previewReport handles POST /report/preview — emails last month's report to the
// signed-in admin now. It never marks anything as sent, so the real monthly send
// is unaffected.
func (s *Server) previewReport(w http.ResponseWriter, r *http.Request) {
	to, ok := s.currentRecipient(w, r)
	if !ok {
		return
	}
	count, err := s.Reports.SendPreview(r.Context(), to)
	if err != nil {
		writeReportError(w, "previewReport", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sent_to": to.Email, "payments": count})
}

// emailReady reports whether email is switched on and fully configured, writing
// the matching 400 (or 500) when it is not. Used before an admin opts in to any
// email, so a switch can't be turned on while nothing could be sent.
func (s *Server) emailReady(w http.ResponseWriter, r *http.Request, where string) bool {
	cur, err := s.Reports.LoadSMTP(r.Context())
	if err != nil {
		logErr(where, err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return false
	}
	if !cur.Enabled {
		writeError(w, http.StatusBadRequest, report.ErrDisabled.Error())
		return false
	}
	if !cur.Configured {
		writeError(w, http.StatusBadRequest, report.ErrNotConfigured.Error())
		return false
	}
	return true
}

// setDailyOptIn is the shared body of the per-admin daily-reminder opt-in endpoints
// (body {"enabled": bool}). Turning one on requires email to be on and configured.
func (s *Server) setDailyOptIn(w http.ResponseWriter, r *http.Request, where string, baseline func() string,
	set func(ctx context.Context, id string, on bool, baseline string) error) {
	adminID, _, _, ok := s.currentAdmin(w, r)
	if !ok {
		return
	}
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.Enabled && !s.emailReady(w, r, where) {
		return
	}
	if err := set(r.Context(), adminID, body.Enabled, baseline()); err != nil {
		logErr(where, err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"enabled": body.Enabled})
}

// setPaymentReminders handles PUT /profile/payment-reminders — the daily
// due-payment reminder opt-in.
func (s *Server) setPaymentReminders(w http.ResponseWriter, r *http.Request) {
	s.setDailyOptIn(w, r, "setPaymentReminders", s.Reports.BaselineDay, s.Admins.SetPaymentReminders)
}

// setRotationReminders handles PUT /profile/rotation-reminders — the daily
// password-rotation reminder opt-in.
func (s *Server) setRotationReminders(w http.ResponseWriter, r *http.Request) {
	s.setDailyOptIn(w, r, "setRotationReminders", s.Reports.BaselineDay, s.Admins.SetRotationReminders)
}

// setReminderDays is the shared body of PUT /profile/payment-reminder-days and
// /profile/rotation-reminder-days: body {"days": [7,3,1,0,-1]} (days before the due
// date, -1 = while overdue). At least one day is required.
func (s *Server) setReminderDays(w http.ResponseWriter, r *http.Request, where string,
	set func(ctx context.Context, id string, mask int) error) {
	adminID, _, _, ok := s.currentAdmin(w, r)
	if !ok {
		return
	}
	var body struct {
		Days []int `json:"days"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	mask, err := store.DaysToMask(body.Days)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := set(r.Context(), adminID, mask); err != nil {
		logErr(where, err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string][]int{"days": store.MaskToDays(mask)})
}

func (s *Server) setPaymentReminderDays(w http.ResponseWriter, r *http.Request) {
	s.setReminderDays(w, r, "setPaymentReminderDays", s.Admins.SetPaymentReminderDays)
}

func (s *Server) setRotationReminderDays(w http.ResponseWriter, r *http.Request) {
	s.setReminderDays(w, r, "setRotationReminderDays", s.Admins.SetRotationReminderDays)
}

// setBreachAlerts handles PUT /profile/breach-alerts — the "email me when a new
// breach is found" opt-in. It has no schedule, so there is no baseline to record.
func (s *Server) setBreachAlerts(w http.ResponseWriter, r *http.Request) {
	s.setDailyOptIn(w, r, "setBreachAlerts", func() string { return "" },
		func(ctx context.Context, id string, on bool, _ string) error {
			return s.Admins.SetBreachAlerts(ctx, id, on)
		})
}

// previewBreachAlert handles POST /breach-alert/preview — emails the passwords
// currently flagged as breached to the signed-in admin now. If none are flagged,
// nothing is sent and "sent" is false.
func (s *Server) previewBreachAlert(w http.ResponseWriter, r *http.Request) {
	to, ok := s.currentRecipient(w, r)
	if !ok {
		return
	}
	count, err := s.Reports.SendBreachPreview(r.Context(), to)
	if err != nil {
		writeReportError(w, "previewBreachAlert", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sent_to": to.Email, "passwords": count, "sent": count > 0})
}

// setUpcomingPayments handles PUT /profile/upcoming-payments — the monthly
// upcoming-payments email opt-in. Its baseline is the current month, so opting in
// mid-month doesn't send this month's list; the first email is next month's.
func (s *Server) setUpcomingPayments(w http.ResponseWriter, r *http.Request) {
	s.setDailyOptIn(w, r, "setUpcomingPayments", s.Reports.CurrentMonth, s.Admins.SetUpcomingPayments)
}

// previewReminder handles POST /reminder/preview — emails the current due-payment
// reminder to the signed-in admin now. If nothing is due, nothing is sent and
// "sent" is false. It never touches the daily markers.
func (s *Server) previewReminder(w http.ResponseWriter, r *http.Request) {
	to, ok := s.currentRecipient(w, r)
	if !ok {
		return
	}
	count, err := s.Reports.SendReminderPreview(r.Context(), to)
	if err != nil {
		writeReportError(w, "previewReminder", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sent_to": to.Email, "payments": count, "sent": count > 0})
}

// previewUpcoming handles POST /upcoming/preview — emails this month's upcoming
// payments to the signed-in admin now. If nothing is scheduled, nothing is sent and
// "sent" is false. It never touches the monthly markers.
func (s *Server) previewUpcoming(w http.ResponseWriter, r *http.Request) {
	to, ok := s.currentRecipient(w, r)
	if !ok {
		return
	}
	count, err := s.Reports.SendUpcomingPreview(r.Context(), to)
	if err != nil {
		writeReportError(w, "previewUpcoming", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sent_to": to.Email, "payments": count, "sent": count > 0})
}

// previewRotation handles POST /rotation/preview — emails the current
// password-rotation reminder to the signed-in admin now. If nothing is due,
// nothing is sent and "sent" is false. It never touches the daily markers.
func (s *Server) previewRotation(w http.ResponseWriter, r *http.Request) {
	to, ok := s.currentRecipient(w, r)
	if !ok {
		return
	}
	count, err := s.Reports.SendRotationPreview(r.Context(), to)
	if err != nil {
		writeReportError(w, "previewRotation", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sent_to": to.Email, "passwords": count, "sent": count > 0})
}

// setMonthlyReport handles PUT /profile/monthly-report with body {"enabled": bool}.
// Turning it on requires email to be configured.
func (s *Server) setMonthlyReport(w http.ResponseWriter, r *http.Request) {
	adminID, _, _, ok := s.currentAdmin(w, r)
	if !ok {
		return
	}
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.Enabled && !s.emailReady(w, r, "setMonthlyReport") {
		return
	}
	if err := s.Admins.SetMonthlyReport(r.Context(), adminID, body.Enabled, s.Reports.BaselineMonth()); err != nil {
		logErr("setMonthlyReport", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"enabled": body.Enabled})
}
