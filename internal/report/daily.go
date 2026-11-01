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

// dailyKind describes one scheduled "heads-up" email, so payment reminders,
// password-rotation reminders and the monthly upcoming-payments email share one
// scheduler. Selection of what to report, the wording, and how often it repeats
// (marker) are the only differences.
type dailyKind struct {
	audit   string // audit action for scheduled sends ("reminder.send", ...)
	logName string // for server log lines
	unit    string // noun used in the audit detail ("due", "payments")
	hour    int    // earliest local hour this email is sent, so it lands in the morning
	// marker is the period an admin's "already sent" record is keyed on: "YYYY-MM-DD"
	// for a daily email, "YYYY-MM" for a monthly one. An admin is emailed when their
	// stored marker is earlier than the current one.
	marker func(now time.Time) string
	// markEmpty: when nothing needs reporting, record the period as handled (without
	// sending) instead of retrying on the next check. Right for a monthly email,
	// where adding an entry mid-month shouldn't trigger a late "start of month" mail.
	markEmpty bool
	listDue   func(ctx context.Context, marker string) ([]store.ReportRecipient, error)
	markSent  func(ctx context.Context, id, marker string) error
	// build returns how many items currently need attention and a renderer that
	// turns them into one admin's email. render gets the admin's chosen reminder-day
	// mask and returns how many items it included; 0 means nothing for this admin today
	// (no email). Emails that have no day choice ignore the mask.
	build func(ctx context.Context) (count int, render func(to Recipient, mask int) (mailer.Message, int), err error)
}

func dayMarker(now time.Time) string   { return now.Format("2006-01-02") }
func monthMarker(now time.Time) string { return now.Format("2006-01") }

func (s *Service) paymentKind() dailyKind {
	return dailyKind{
		audit:    "reminder.send",
		logName:  "payment reminders",
		unit:     "due",
		hour:     ReminderHour,
		marker:   dayMarker,
		listDue:  s.Admins.ListReminderDue,
		markSent: s.Admins.MarkReminderSent,
		build: func(ctx context.Context) (int, func(Recipient, int) (mailer.Message, int), error) {
			items, err := s.DueNow(ctx)
			return len(items), func(to Recipient, mask int) (mailer.Message, int) {
				var sel []DueItem
				for _, it := range items {
					if store.DayWanted(mask, it.Days) {
						sel = append(sel, it)
					}
				}
				if len(sel) == 0 {
					return mailer.Message{}, 0
				}
				return reminderMessage(to, sel), len(sel)
			}, err
		},
	}
}

func (s *Service) rotationKind() dailyKind {
	return dailyKind{
		audit:    "rotation.send",
		logName:  "password rotation reminders",
		unit:     "due",
		hour:     ReminderHour,
		marker:   dayMarker,
		listDue:  s.Admins.ListRotationDue,
		markSent: s.Admins.MarkRotationSent,
		build: func(ctx context.Context) (int, func(Recipient, int) (mailer.Message, int), error) {
			items, err := s.RotationNow(ctx)
			return len(items), func(to Recipient, mask int) (mailer.Message, int) {
				var sel []RotationItem
				for _, it := range items {
					if store.DayWanted(mask, it.Days) {
						sel = append(sel, it)
					}
				}
				if len(sel) == 0 {
					return mailer.Message{}, 0
				}
				return rotationMessage(to, sel), len(sel)
			}, err
		},
	}
}

func (s *Service) upcomingKind() dailyKind {
	return dailyKind{
		audit:     "upcoming.send",
		logName:   "upcoming payments",
		unit:      "payments",
		hour:      UpcomingHour,
		marker:    monthMarker,
		markEmpty: true,
		listDue:   s.Admins.ListUpcomingDue,
		markSent:  s.Admins.MarkUpcomingSent,
		build: func(ctx context.Context) (int, func(Recipient, int) (mailer.Message, int), error) {
			month, items, conv, err := s.UpcomingNow(ctx)
			return len(items), func(to Recipient, _ int) (mailer.Message, int) {
				return upcomingMessage(to, month, items, conv), len(items)
			}, err
		},
	}
}

// BaselineDay is today's "YYYY-MM-DD", recorded as already-sent when an admin opts
// in to a daily reminder so the first one is tomorrow's, not an immediate one.
func (s *Service) BaselineDay() string { return dayMarker(s.Now()) }

// CurrentMonth is this month's "YYYY-MM", recorded as already-handled when an admin
// opts in to the monthly upcoming-payments email, so the first one is next month's.
func (s *Service) CurrentMonth() string { return monthMarker(s.Now()) }

// previewDaily emails the current report to one address right now. When nothing
// needs attention it sends nothing and returns 0. It never touches the markers, so
// it does not suppress the scheduled email.
func (s *Service) previewDaily(ctx context.Context, k dailyKind, to Recipient) (int, error) {
	cfg, err := s.MailerConfig(ctx)
	if err != nil {
		return 0, err
	}
	count, render, err := k.build(ctx)
	if err != nil || count == 0 {
		return 0, err
	}
	msg, n := render(to, store.PreviewDays) // Send Now shows everything flagged, whatever days are chosen
	if n == 0 {
		return 0, nil
	}
	return n, s.SendFn(ctx, cfg, msg)
}

// runDaily sends the scheduled email to every opted-in admin who has not had one
// for the current period, but only when something needs reporting and it is past
// the email's hour (k.hour). Safe to call often: each admin is marked once their copy is
// delivered, and a failed send is retried every retryEvery (30 minutes) until it goes through.
func (s *Service) runDaily(ctx context.Context, k dailyKind) {
	if len(s.KeyFn()) == 0 {
		return // vault locked — can't read the SMTP password yet
	}
	now := s.Now()
	if now.Hour() < k.hour {
		return
	}
	marker := k.marker(now)
	due, err := k.listDue(ctx, marker)
	if err != nil || len(due) == 0 {
		return
	}
	cfg, err := s.MailerConfig(ctx)
	if err != nil {
		return // off / not configured / undecryptable: stay quiet, retry next check
	}
	count, render, err := k.build(ctx)
	if err != nil {
		log.Printf("%s: %v", k.logName, err)
		return
	}
	if count == 0 {
		if k.markEmpty { // nothing this period: settle it quietly rather than mail later
			for _, r := range due {
				if err := k.markSent(ctx, r.ID, marker); err != nil {
					log.Printf("%s: mark handled for %s: %v", k.logName, r.Email, err)
				}
			}
		}
		return
	}

	for _, r := range due {
		key := k.audit + ":" + r.ID
		s.mu.Lock()
		last, tried := s.lastTry[key]
		if tried && s.Now().Sub(last) < retryEvery {
			s.mu.Unlock()
			continue
		}
		s.lastTry[key] = s.Now()
		s.mu.Unlock()

		if _, perr := mail.ParseAddress(r.Email); perr != nil {
			s.audit(k.audit, r.Email, marker+": invalid address", "fail")
			continue
		}
		msg, n := render(s.recipientFor(ctx, r), r.Days&^store.OverdueBit) // overdue items are never reminded about
		if n == 0 {
			continue // none of today's items falls on a day this admin chose
		}
		if err := s.SendFn(ctx, cfg, msg); err != nil {
			log.Printf("%s to %s failed: %v", k.logName, r.Email, err)
			if !tried { // only the first failure is audited; retries just log
				s.audit(k.audit, r.Email, marker+": "+truncate(err.Error(), 200)+" (will retry)", "fail")
			}
			continue
		}
		s.mu.Lock()
		delete(s.lastTry, key)
		s.mu.Unlock()
		if err := k.markSent(ctx, r.ID, marker); err != nil {
			log.Printf("%s: mark sent for %s: %v", k.logName, r.Email, err)
		}
		s.audit(k.audit, r.Email, fmt.Sprintf("%s / %d %s", marker, n, k.unit), "ok")
	}
}

// RunRemindersIfDue sends today's due-payment reminders.
func (s *Service) RunRemindersIfDue(ctx context.Context) { s.runDaily(ctx, s.paymentKind()) }

// RunRotationRemindersIfDue sends today's password-rotation reminders.
func (s *Service) RunRotationRemindersIfDue(ctx context.Context) { s.runDaily(ctx, s.rotationKind()) }

// RunUpcomingIfDue sends this month's upcoming-payments email.
func (s *Service) RunUpcomingIfDue(ctx context.Context) { s.runDaily(ctx, s.upcomingKind()) }

// SendReminderPreview emails the current due-payment reminder to one address now.
func (s *Service) SendReminderPreview(ctx context.Context, to Recipient) (int, error) {
	return s.previewDaily(ctx, s.paymentKind(), to)
}

// SendRotationPreview emails the current password-rotation reminder to one address now.
func (s *Service) SendRotationPreview(ctx context.Context, to Recipient) (int, error) {
	return s.previewDaily(ctx, s.rotationKind(), to)
}

// SendUpcomingPreview emails this month's upcoming payments to one address now.
func (s *Service) SendUpcomingPreview(ctx context.Context, to Recipient) (int, error) {
	return s.previewDaily(ctx, s.upcomingKind(), to)
}
