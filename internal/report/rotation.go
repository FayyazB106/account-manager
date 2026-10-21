package report

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/FayyazB106/account-manager/internal/store"
)

// RotationItem is one active account whose password is due for rotation within the
// window, or already overdue.
type RotationItem struct {
	Service string
	Login   string    // empty when the account has no login; tells same-named accounts apart
	Due     time.Time // calendar date the rotation is due (password changed + rotate-every days)
	Days    int       // days until due; negative = overdue
}

// RotationItems selects the accounts that need a rotation reminder: active, with a
// rotation policy (rotate every N days), whose due date — the date the password was
// last changed plus N days, the same rule the app's own banner uses — is overdue or
// no more than ReminderWindowDays away. Most urgent first.
func RotationItems(accounts []store.AccountListItem, now time.Time) []RotationItem {
	today := dateOnly(now)
	var out []RotationItem
	for _, a := range accounts {
		if a.Status != "active" || a.RotateEveryDays == nil || *a.RotateEveryDays <= 0 {
			continue
		}
		due := dateOnly(a.PasswordChangedAt.AddDate(0, 0, *a.RotateEveryDays))
		days := int(due.Sub(today).Hours() / 24)
		if days > ReminderWindowDays {
			continue
		}
		login := ""
		if a.Login != nil {
			login = strings.TrimSpace(*a.Login)
		}
		out = append(out, RotationItem{Service: a.Service, Login: login, Due: due, Days: days})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].Due.Equal(out[j].Due) {
			return out[i].Due.Before(out[j].Due)
		}
		return strings.ToLower(out[i].Service) < strings.ToLower(out[j].Service)
	})
	return out
}

// RotationNow returns the passwords that need a rotation reminder right now.
func (s *Service) RotationNow(ctx context.Context) ([]RotationItem, error) {
	accounts, err := s.Accounts.List(ctx)
	if err != nil {
		return nil, err
	}
	return RotationItems(accounts, s.Now()), nil
}
