package report

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/FayyazB106/account-manager/internal/store"
)

// ReminderWindowDays is how far ahead a payment counts as "due soon". It matches
// the app's own 7-day "payments need attention" highlight, so the email and the
// UI always agree on what is flagged.
const ReminderWindowDays = 7

// ReminderHour is the earliest local hour the daily reminders (due payments and
// password rotation) are sent: 08:00, the same as the monthly upcoming-payments email, so they are waiting first thing in the
// morning. If the app starts later, they still go out that day.
const ReminderHour = 8

// DueItem is one active recurring payment that is due within the window or overdue.
type DueItem struct {
	Service string
	Due     time.Time // calendar date of the next payment
	Days    int       // days until due; negative = overdue
	Amount  *float64  // USD, as stored on the account
}

// dateOnly strips the time of day so two dates differ by a whole number of days.
func dateOnly(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// DueItems selects the accounts that need a reminder: active, recurring, with a
// next-payment date that is overdue or no more than ReminderWindowDays away.
// Sorted most urgent first (overdue, then soonest).
func DueItems(accounts []store.AccountListItem, now time.Time) []DueItem {
	today := dateOnly(now)
	var out []DueItem
	for _, a := range accounts {
		if a.Status != "active" || a.BillingPeriodMonths <= 0 || a.NextPaymentDate == nil {
			continue
		}
		due := dateOnly(*a.NextPaymentDate)
		days := int(due.Sub(today).Hours() / 24)
		if days > ReminderWindowDays {
			continue
		}
		out = append(out, DueItem{Service: a.Service, Due: due, Days: days, Amount: a.Amount})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].Due.Equal(out[j].Due) {
			return out[i].Due.Before(out[j].Due)
		}
		return strings.ToLower(out[i].Service) < strings.ToLower(out[j].Service)
	})
	return out
}

// DueNow returns the payments that need a reminder right now.
func (s *Service) DueNow(ctx context.Context) ([]DueItem, error) {
	accounts, err := s.Accounts.List(ctx)
	if err != nil {
		return nil, err
	}
	return DueItems(accounts, s.Now()), nil
}
