package report

import (
	"context"
	"fmt"
	"log"
	"net/mail"
	"time"

	"github.com/FayyazB106/account-manager/internal/mailer"
	"github.com/FayyazB106/account-manager/internal/store"
)

// breachTimes says how many times a password appears in breach data.
func breachTimes(l Lang, n int) string {
	switch {
	case l != LangAR && n == 1:
		return "exposed 1 time"
	case l != LangAR:
		return fmt.Sprintf("exposed %d times", n)
	case n == 1:
		return "ظهرت مرة واحدة"
	case n == 2:
		return "ظهرت مرتين"
	default:
		return fmt.Sprintf("ظهرت %d مرة", n)
	}
}

// breachMessage is the alert for passwords found in known data breaches. `fresh` is
// true for the automatic alert (found by the scan that just ran) and false for
// "Send Now" (everything currently flagged).
func breachMessage(to Recipient, items []store.BreachHit, fresh bool) mailer.Message {
	l := to.Lang
	w := &body{l: l}
	w.line(greeting(l, to.Name))
	w.blank()
	if fresh {
		w.line(pick(l,
			"A breach scan just found these passwords in known data breaches:",
			"عثر فحص التسريبات للتو على كلمات المرور هذه في تسريبات بيانات معروفة:"))
	} else {
		w.line(pick(l,
			"These passwords are currently flagged as found in known data breaches:",
			"كلمات المرور هذه مصنّفة حاليًا على أنها وُجدت في تسريبات بيانات معروفة:"))
	}
	w.blank()
	for _, it := range items {
		w.line(fmt.Sprintf("  - %s: %s", who(l, it.Service, it.Login), breachTimes(l, it.Count)))
	}
	w.blank()
	w.line(pick(l,
		"Change them as soon as possible and save the new passwords in Account Manager. A breached password can be tried against your accounts even if the service itself was never hacked.",
		"يرجى تغييرها في أقرب وقت وحفظ كلمات المرور الجديدة في مدير الحسابات. يمكن تجربة كلمة المرور المسرّبة على حساباتك حتى لو لم تتعرض الخدمة نفسها للاختراق."))
	w.blank()
	w.line(optOutNote(l, pick(l,
		"You receive this right after a breach scan finds a new breach because breach alerts are turned on for your profile",
		"تصلك هذه الرسالة فور اكتشاف فحص التسريبات تسريبًا جديدًا لأن تنبيهات التسريب مفعّلة في ملفك الشخصي")))
	return mailer.Message{
		To: []string{to.Email},
		Subject: subject(l, pick(l,
			nounPassword.of(l, len(items))+" found in a data breach",
			fmt.Sprintf("كلمات مرور ظهرت في تسريب بيانات (%d)", len(items)))),
		Body: w.String(),
		RTL:  l == LangAR,
	}
}

// breachRetryEvery is how long to wait before trying an undelivered breach alert again
// (the scheduler also only looks every checkEvery).
const breachRetryEvery = 30 * time.Minute

// NotifyBreaches emails every admin with breach alerts on, right now, about the
// passwords a scan just found in new breaches. It is event-driven, not scheduled, and
// stays quiet when email is off or not set up. Each alert is queued first and removed
// once delivered, so one that can't be sent (mail server or network down) is retried
// by RetryBreachAlerts until it goes through.
func (s *Service) NotifyBreaches(ctx context.Context, hits []store.BreachHit) {
	if len(hits) == 0 || len(s.KeyFn()) == 0 {
		return
	}
	recipients, err := s.Admins.ListBreachAlertRecipients(ctx)
	if err != nil || len(recipients) == 0 {
		return
	}
	cfg, err := s.MailerConfig(ctx)
	if err != nil {
		return // off / not configured / undecryptable
	}
	ids := make([]string, len(hits))
	for i, h := range hits {
		ids[i] = h.ID
	}
	for _, r := range recipients {
		if err := s.Admins.AddBreachPending(ctx, r.ID, ids); err != nil {
			log.Printf("breach alert: queue for %s: %v", r.Email, err)
			continue
		}
		s.deliverBreach(ctx, cfg, r, true)
	}
}

// RetryBreachAlerts tries again to deliver breach alerts that could not be sent
// earlier. Called by the scheduler; waits breachRetryEvery between attempts per admin.
func (s *Service) RetryBreachAlerts(ctx context.Context) {
	if len(s.KeyFn()) == 0 {
		return
	}
	recipients, err := s.Admins.ListBreachPendingRecipients(ctx)
	if err != nil || len(recipients) == 0 {
		return
	}
	cfg, err := s.MailerConfig(ctx)
	if err != nil {
		return
	}
	for _, r := range recipients {
		key := "breach.alert:" + r.ID
		s.mu.Lock()
		last, tried := s.lastTry[key]
		s.mu.Unlock()
		if tried && s.Now().Sub(last) < breachRetryEvery {
			continue
		}
		s.deliverBreach(ctx, cfg, r, false)
	}
}

// deliverBreach sends one admin everything still queued for them and clears the queue
// on success. Alerts that stopped being true (password changed, account deactivated
// or deleted) are dropped. Only the first failure is audited; later retries are
// logged, so an outage doesn't fill the audit log.
func (s *Service) deliverBreach(ctx context.Context, cfg mailer.Config, r store.ReportRecipient, first bool) {
	hits, err := s.Admins.PendingBreachHits(ctx, r.ID)
	if err != nil {
		log.Printf("breach alert: read queue for %s: %v", r.Email, err)
		return
	}
	if len(hits) == 0 {
		_ = s.Admins.ClearBreachPending(ctx, r.ID)
		return
	}
	s.mu.Lock()
	s.lastTry["breach.alert:"+r.ID] = s.Now()
	s.mu.Unlock()

	if _, perr := mail.ParseAddress(r.Email); perr != nil {
		_ = s.Admins.ClearBreachPending(ctx, r.ID)
		s.audit("breach.alert", r.Email, "invalid address", "fail")
		return
	}
	if err := s.SendFn(ctx, cfg, breachMessage(s.recipientFor(ctx, r), hits, true)); err != nil {
		log.Printf("breach alert to %s failed (will retry): %v", r.Email, err)
		if first {
			s.audit("breach.alert", r.Email, truncate(err.Error(), 200)+" (will retry)", "fail")
		}
		return
	}
	if err := s.Admins.ClearBreachPending(ctx, r.ID); err != nil {
		log.Printf("breach alert: clear queue for %s: %v", r.Email, err)
	}
	detail := fmt.Sprintf("%d new", len(hits))
	if !first {
		detail += " (retried)"
	}
	s.audit("breach.alert", r.Email, detail, "ok")
}

// SendBreachPreview emails everything currently flagged as breached to one address
// now. When nothing is flagged it sends nothing and returns 0.
func (s *Service) SendBreachPreview(ctx context.Context, to Recipient) (int, error) {
	cfg, err := s.MailerConfig(ctx)
	if err != nil {
		return 0, err
	}
	hits, err := s.Accounts.ListBreached(ctx)
	if err != nil || len(hits) == 0 {
		return 0, err
	}
	return len(hits), s.SendFn(ctx, cfg, breachMessage(to, hits, false))
}
