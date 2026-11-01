package report

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/FayyazB106/account-manager/internal/store"
)

// UpcomingHour is the earliest local hour the monthly upcoming-payments email is
// sent on the 1st (08:00). The daily reminders use the same hour — see ReminderHour.
const UpcomingHour = 8

// UpcomingItem is one payment projected to fall in a given month — the same rows
// the Finance → Monthly Costs page lists under a month.
type UpcomingItem struct {
	Service  string
	Login    string
	Date     time.Time
	Amount   float64 // in Currency, as stored on the account
	Currency string  // currency of Amount ("USD" if unset)
}

// addMonthsClamped adds `add` months to base, clamping the day to the target
// month's length so a 31st (or 29th) start doesn't skip or overflow a short month.
// Mirrors addMonthsClamped in the Monthly Costs page.
func addMonthsClamped(base time.Time, add int) time.Time {
	first := time.Date(base.Year(), base.Month()+time.Month(add), 1, 0, 0, 0, 0, time.UTC)
	last := time.Date(first.Year(), first.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
	day := base.Day()
	if day > last {
		day = last
	}
	return time.Date(first.Year(), first.Month(), day, 0, 0, 0, 0, time.UTC)
}

func monthIndex(t time.Time) int { return t.Year()*12 + int(t.Month()) - 1 }

// ProjectMonth lists the payments that fall in the calendar month containing
// `month`, using the Monthly Costs page's rule: only active accounts with an amount
// and a next-payment date; a recurring account pays on its next-payment date and
// every billing period after it (stepping from that date, clamped to short
// months); a one-time account pays once, on its date. Sorted by date, then service.
func ProjectMonth(accounts []store.AccountListItem, month time.Time) []UpcomingItem {
	target := monthIndex(month)
	var out []UpcomingItem
	for _, a := range accounts {
		if a.Status != "active" || a.Amount == nil || *a.Amount <= 0 || a.NextPaymentDate == nil {
			continue
		}
		login := ""
		if a.Login != nil {
			login = strings.TrimSpace(*a.Login)
		}
		start := dateOnly(*a.NextPaymentDate)
		push := func(d time.Time) {
			out = append(out, UpcomingItem{Service: a.Service, Login: login, Date: d, Amount: *a.Amount, Currency: a.Currency})
		}
		if a.BillingPeriodMonths <= 0 {
			if monthIndex(start) == target {
				push(start)
			}
			continue
		}
		for k := 0; k < 200; k++ { // same bound as the page
			occ := addMonthsClamped(start, k*a.BillingPeriodMonths)
			idx := monthIndex(occ)
			if idx > target {
				break
			}
			if idx == target {
				push(occ)
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].Date.Equal(out[j].Date) {
			return out[i].Date.Before(out[j].Date)
		}
		return strings.ToLower(out[i].Service) < strings.ToLower(out[j].Service)
	})
	return out
}

// money formats a USD amount; with other active currencies it appends each
// conversion ("$5.00 / SAR 18.75"), like the amounts elsewhere in the app.
func money(usd float64, conv *Converter) string {
	parts := []string{fmt.Sprintf("$%.2f", usd)}
	for _, c := range conv.Active() {
		if c.Code != "USD" {
			parts = append(parts, fmt.Sprintf("%s %.2f", c.Code, conv.AmountIn(usd, "USD", c)))
		}
	}
	return strings.Join(parts, " / ")
}

// UpcomingNow returns the current month and its projected payments, plus the
// currency converter used to show the total in every active currency.
func (s *Service) UpcomingNow(ctx context.Context) (time.Time, []UpcomingItem, *Converter, error) {
	now := s.Now()
	month := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	accounts, err := s.Accounts.List(ctx)
	if err != nil {
		return month, nil, nil, err
	}
	currencies, err := s.Currencies.List(ctx)
	if err != nil {
		return month, nil, nil, err
	}
	return month, ProjectMonth(accounts, month), NewConverter(currencies), nil
}
