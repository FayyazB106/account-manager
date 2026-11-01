package report

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/FayyazB106/account-manager/internal/crypto"
	"github.com/FayyazB106/account-manager/internal/db"
	"github.com/FayyazB106/account-manager/internal/mailer"
	"github.com/FayyazB106/account-manager/internal/store"
	"github.com/xuri/excelize/v2"
)

func TestPreviousMonth(t *testing.T) {
	cases := []struct {
		now             time.Time
		from, to, label string
	}{
		{time.Date(2026, 11, 1, 8, 0, 0, 0, time.UTC), "2026-10-01", "2026-10-31", "2026-10"},
		{time.Date(2027, 1, 15, 0, 0, 0, 0, time.UTC), "2026-12-01", "2026-12-31", "2026-12"},
		{time.Date(2024, 3, 5, 0, 0, 0, 0, time.UTC), "2024-02-01", "2024-02-29", "2024-02"}, // leap year
	}
	for _, c := range cases {
		from, to, label := PreviousMonth(c.now)
		if from.Format("2006-01-02") != c.from || to.Format("2006-01-02") != c.to || label != c.label {
			t.Errorf("PreviousMonth(%s) = %s..%s %s, want %s..%s %s", c.now.Format("2006-01-02"),
				from.Format("2006-01-02"), to.Format("2006-01-02"), label, c.from, c.to, c.label)
		}
	}
}

// alice is a plain English recipient used by the Send Now tests.
var alice = Recipient{Name: "Alice", Email: "a@example.com", Lang: LangEN}

func pay(day string, amount float64, ccy string) store.PaymentListItem {
	d, _ := time.Parse("2006-01-02", day)
	return store.PaymentListItem{Payment: store.Payment{PaidOn: d, Amount: amount, Currency: ccy}, Service: "Adobe"}
}

func TestFilterByRangeIsInclusive(t *testing.T) {
	ps := []store.PaymentListItem{
		pay("2026-09-30", 1, "USD"), pay("2026-10-01", 2, "USD"),
		pay("2026-10-31", 3, "USD"), pay("2026-11-01", 4, "USD"),
	}
	from, to, _ := PreviousMonth(time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC))
	got := FilterByRange(ps, from, to)
	if len(got) != 2 || got[0].Amount != 2 || got[1].Amount != 3 {
		t.Fatalf("FilterByRange kept %v, want only the 1st and 31st", got)
	}
}

func testCurrencies() []store.Currency {
	return []store.Currency{
		{Code: "USD", Active: true, Rate: 1},
		{Code: "SAR", Active: true, Rate: 3.75},
		{Code: "EUR", Active: false, Rate: 0.9}, // inactive → no column
		{Code: "GBP", Active: true, Rate: 0},    // active but no rate → no column
	}
}

func TestPaymentsWorkbookLayout(t *testing.T) {
	note, inv, cat := "renewal", "invoice.pdf", "Design"
	p1 := pay("2026-10-05", 10, "USD")
	p1.Note, p1.AttachmentName, p1.CategoryName = &note, &inv, &cat
	p1.Seats = []string{"Ahmed", " ", "Mark"}
	p2 := pay("2026-10-02", 37.5, "SAR")

	data, err := BuildPaymentsWorkbook([]store.PaymentListItem{p1, p2}, NewConverter(testCurrencies()), LangEN)
	if err != nil {
		t.Fatal(err)
	}
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := f.GetRows(PaymentsSheetName)
	if len(rows) != 4 { // header + 2 payments + total
		t.Fatalf("rows = %d, want 4: %v", len(rows), rows)
	}
	wantHeader := "Date|Service|Category|Users|Amount (USD)|Amount (SAR)|Paid in|Note|Invoice"
	if got := strings.Join(rows[0], "|"); got != wantHeader {
		t.Fatalf("header = %s\nwant     %s", got, wantHeader)
	}
	if got := strings.Join(rows[1], "|"); got != "2026-10-05|Adobe|Design|Ahmed, Mark|10|37.5|USD|renewal|invoice.pdf" {
		t.Errorf("row1 = %s", got)
	}
	// 37.5 SAR = 10 USD; the SAR payment is shown in both currency columns.
	if got := strings.Join(rows[2][:6], "|"); got != "2026-10-02|Adobe||"+"|10|37.5" {
		t.Errorf("row2 = %s", got)
	}
	if rows[2][6] != "SAR" {
		t.Errorf("Paid in = %q, want SAR", rows[2][6])
	}
	if rows[3][0] != "Total" || rows[3][4] != "20" || rows[3][5] != "75" {
		t.Errorf("total row = %v, want 20 USD / 75 SAR", rows[3])
	}
}

// --- integration: scheduler + SMTP settings against a real database ----------

type fixture struct {
	svc    *Service
	sent   []mailer.Message
	failN  int // fail this many sends before succeeding
	now    time.Time
	key    []byte
	locked bool
	db     *sql.DB
	adminA string
	adminB string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	database, err := db.Open(filepath.Join(t.TempDir(), "r.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	ctx := context.Background()

	fx := &fixture{now: time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC), db: database}
	fx.key, _ = crypto.RandomBytes(crypto.KeySize)

	admins := store.NewAdminStore(database)
	fx.adminA, _ = admins.Create(ctx, "a@example.com", "Alice", nil, "h", []byte("s"), []byte("w"))
	fx.adminB, _ = admins.Create(ctx, "b@example.com", "Bob", nil, "h", []byte("s"), []byte("w"))

	accounts := store.NewAccountStore(database)
	acc, err := accounts.Create(ctx, &store.Account{Service: "Adobe", PasswordWrappedDEK: []byte{1}, PasswordCiphertext: []byte{1}})
	if err != nil {
		t.Fatal(err)
	}
	// Two recurring accounts for the reminder tests, relative to fx.now (2026-11-02):
	// Zoom is due in 3 days, Figma is 3 days overdue. "Adobe" above has no billing
	// period, so it never counts as due.
	mkDue := func(service, day string, amount float64) {
		d, _ := time.Parse("2006-01-02", day)
		a := amount
		if _, err := accounts.Create(ctx, &store.Account{Service: service, BillingPeriodMonths: 1, Amount: &a,
			NextPaymentDate: &d, PasswordWrappedDEK: []byte{1}, PasswordCiphertext: []byte{1}}); err != nil {
			t.Fatal(err)
		}
	}
	mkDue("Zoom", "2026-11-05", 12.5)
	mkDue("Figma", "2026-10-30", 15)

	// Two accounts with a rotate-every-30-days policy, relative to fx.now:
	// Slack's password changed 2026-10-01 → due 2026-10-31 (2 days overdue);
	// Trello's changed 2026-10-08 → due 2026-11-07 (in 5 days).
	mkRotate := func(service, login, changed string) {
		days := 30
		lg := login
		id, err := accounts.Create(ctx, &store.Account{Service: service, Login: &lg, RotateEveryDays: &days,
			PasswordWrappedDEK: []byte{1}, PasswordCiphertext: []byte{1}})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := database.Exec(`UPDATE account SET password_changed_at = $2 WHERE id = $1`, id, changed); err != nil {
			t.Fatal(err)
		}
	}
	mkRotate("Slack", "ops@example.com", "2026-10-01 10:00:00")
	mkRotate("Trello", "", "2026-10-08 10:00:00")
	payments := store.NewPaymentStore(database)
	for _, p := range []struct {
		day string
		amt float64
	}{{"2026-09-20", 111}, {"2026-10-05", 10}, {"2026-10-20", 16}, {"2026-11-01", 999}} {
		d, _ := time.Parse("2006-01-02", p.day)
		if _, err := payments.Create(ctx, &store.Payment{AccountID: acc, PaidOn: d, Amount: p.amt, Currency: "USD"}); err != nil {
			t.Fatal(err)
		}
	}

	fx.svc = New(store.NewSettingStore(database), admins, payments, store.NewCurrencyStore(database), accounts,
		func() []byte {
			if fx.locked {
				return nil
			}
			return fx.key
		})
	fx.svc.Now = func() time.Time { return fx.now }
	fx.svc.SendFn = func(_ context.Context, _ mailer.Config, m mailer.Message) error {
		if fx.failN > 0 {
			fx.failN--
			return errors.New("smtp login: 535 bad credentials")
		}
		fx.sent = append(fx.sent, m)
		return nil
	}
	err = fx.svc.SaveSMTP(ctx, SMTPInput{Host: "smtp.resend.com", Port: 465, Security: "tls",
		Username: "resend", From: "onboarding@resend.dev", Password: "re_super_secret"})
	if err != nil {
		t.Fatal(err)
	}
	if err := fx.svc.SetEnabled(ctx, true); err != nil {
		t.Fatal(err)
	}
	return fx
}

func attachmentRows(t *testing.T, m mailer.Message) [][]string {
	t.Helper()
	if len(m.Attachments) != 1 {
		t.Fatalf("attachments = %d, want 1", len(m.Attachments))
	}
	f, err := excelize.OpenReader(bytes.NewReader(m.Attachments[0].Data))
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := f.GetRows(PaymentsSheetName)
	return rows
}

func TestRunIfDueSendsLastMonthOncePerAdmin(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()
	// A opted in during October (baseline = September) → due for October's report.
	if err := fx.svc.Admins.SetMonthlyReport(ctx, fx.adminA, true, "2026-09"); err != nil {
		t.Fatal(err)
	}

	fx.svc.RunIfDue(ctx)
	if len(fx.sent) != 1 || fx.sent[0].To[0] != "a@example.com" {
		t.Fatalf("sent = %d messages, want one to a@example.com", len(fx.sent))
	}
	m := fx.sent[0]
	if m.Attachments[0].Filename != "payments-2026-10.xlsx" || !strings.Contains(m.Subject, "October 2026") {
		t.Errorf("attachment %q subject %q", m.Attachments[0].Filename, m.Subject)
	}
	rows := attachmentRows(t, m)
	if len(rows) != 4 { // header + the two October payments + total
		t.Fatalf("report rows = %d, want only October's two payments: %v", len(rows), rows)
	}
	if rows[len(rows)-1][4] != "26" {
		t.Errorf("October total = %v, want 26", rows[len(rows)-1])
	}

	fx.svc.RunIfDue(ctx) // marker set → no duplicate
	if len(fx.sent) != 1 {
		t.Fatalf("report was re-sent: %d messages", len(fx.sent))
	}
}

func TestOptInMidMonthDoesNotBackfill(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()
	// B opts in on 2 Nov using the real baseline → October already counts as sent.
	if err := fx.svc.Admins.SetMonthlyReport(ctx, fx.adminB, true, fx.svc.BaselineMonth()); err != nil {
		t.Fatal(err)
	}
	fx.svc.RunIfDue(ctx)
	if len(fx.sent) != 0 {
		t.Fatalf("opting in mid-month triggered an immediate backfill (%d sent)", len(fx.sent))
	}
	// …but the next month's report arrives once December starts.
	fx.now = time.Date(2026, 12, 1, 8, 0, 0, 0, time.UTC)
	fx.svc.RunIfDue(ctx)
	if len(fx.sent) != 1 || !strings.Contains(fx.sent[0].Subject, "November 2026") {
		t.Fatalf("expected November's report in December, got %d: %v", len(fx.sent), fx.sent)
	}
}

func TestRunIfDueRetriesFailuresWithBackoff(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()
	_ = fx.svc.Admins.SetMonthlyReport(ctx, fx.adminA, true, "2026-09")
	var audited []string
	fx.svc.Audit = func(action, target, detail, result string) { audited = append(audited, action+":"+result) }

	fx.failN = 1
	fx.svc.RunIfDue(ctx) // fails
	fx.svc.RunIfDue(ctx) // inside the backoff window → not retried
	if len(fx.sent) != 0 || len(audited) != 1 || audited[0] != "report.send:fail" {
		t.Fatalf("sent=%d audited=%v, want one failed attempt and no retry yet", len(fx.sent), audited)
	}
	fx.now = fx.now.Add(retryEvery + time.Minute)
	fx.svc.RunIfDue(ctx) // retried → succeeds
	if len(fx.sent) != 1 {
		t.Fatalf("expected the retry to deliver, sent=%d", len(fx.sent))
	}
}

func TestRunIfDueDoesNothingWhileVaultLocked(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()
	_ = fx.svc.Admins.SetMonthlyReport(ctx, fx.adminA, true, "2026-09")
	fx.locked = true
	fx.svc.RunIfDue(ctx)
	if len(fx.sent) != 0 {
		t.Fatal("sent mail while the vault was locked")
	}
	fx.locked = false
	fx.svc.RunIfDue(ctx) // unlocked later → catches up
	if len(fx.sent) != 1 {
		t.Fatalf("expected catch-up after unlock, sent=%d", len(fx.sent))
	}
}

func TestPreviewDoesNotSuppressTheRealSend(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()
	_ = fx.svc.Admins.SetMonthlyReport(ctx, fx.adminA, true, "2026-09")
	n, err := fx.svc.SendPreview(ctx, alice)
	if err != nil || n != 2 || len(fx.sent) != 1 {
		t.Fatalf("preview n=%d err=%v sent=%d", n, err, len(fx.sent))
	}
	fx.svc.RunIfDue(ctx)
	if len(fx.sent) != 2 {
		t.Fatalf("scheduled send was suppressed by the preview (sent=%d)", len(fx.sent))
	}
}

func TestSMTPPasswordIsSealedAndNeverExposed(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()

	all, _ := fx.svc.Settings.All(ctx)
	for k, v := range all {
		if strings.Contains(v, "re_super_secret") {
			t.Fatalf("plaintext SMTP password found in setting %q", k)
		}
	}
	if all[KeyPasswordEnc] == "" {
		t.Fatal("password was not stored")
	}
	cur, _ := fx.svc.LoadSMTP(ctx)
	if !cur.HasPassword || !cur.Configured || cur.Host != "smtp.resend.com" {
		t.Fatalf("LoadSMTP = %+v", cur)
	}
	cfg, err := fx.svc.MailerConfig(ctx)
	if err != nil || cfg.Password != "re_super_secret" {
		t.Fatalf("MailerConfig password = %q, %v", cfg.Password, err)
	}

	// Saving again with an empty password keeps the stored one.
	if err := fx.svc.SaveSMTP(ctx, SMTPInput{Host: "smtp.resend.com", Port: 587, Security: "starttls",
		Username: "resend", From: "onboarding@resend.dev"}); err != nil {
		t.Fatal(err)
	}
	if cfg, _ := fx.svc.MailerConfig(ctx); cfg.Password != "re_super_secret" || cfg.Port != 587 {
		t.Fatalf("empty password should keep the stored one: %+v", cfg)
	}

	// A different master key (e.g. restored onto another install) can't read it.
	fx.key, _ = crypto.RandomBytes(crypto.KeySize)
	if _, err := fx.svc.MailerConfig(ctx); err == nil || !strings.Contains(err.Error(), "re-enter") {
		t.Fatalf("expected a clear re-enter-password error, got %v", err)
	}
}

func TestSaveSMTPValidation(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()
	bad := map[string]SMTPInput{
		"credentials over plaintext": {Host: "h.example.com", Port: 25, Security: "none", Username: "u", Password: "p", From: "a@example.com"},
		"bad port":                   {Host: "h.example.com", Port: 0, Security: "tls", From: "a@example.com"},
		"bad from":                   {Host: "h.example.com", Port: 465, Security: "tls", From: "nope"},
		"username without password":  {Host: "other.example.com", Port: 465, Security: "tls", Username: "u2", From: "a@example.com"},
	}
	// The fixture already stored a password for "resend"; clear the login first so
	// "username without password" is genuinely passwordless.
	if err := fx.svc.SaveSMTP(ctx, SMTPInput{Host: "h.example.com", Port: 465, Security: "tls", From: "a@example.com"}); err != nil {
		t.Fatal(err)
	}
	if cur, _ := fx.svc.LoadSMTP(ctx); cur.HasPassword {
		t.Fatal("clearing the username should clear the stored password")
	}
	for name, in := range bad {
		err := fx.svc.SaveSMTP(ctx, in)
		var ve *ValidationError
		if !errors.As(err, &ve) {
			t.Errorf("%s: expected a ValidationError, got %v", name, err)
		}
	}

	// No master key → cannot store a password.
	fx.locked = true
	err := fx.svc.SaveSMTP(ctx, SMTPInput{Host: "h.example.com", Port: 465, Security: "tls", Username: "u", Password: "p", From: "a@example.com"})
	if !errors.Is(err, ErrLocked) {
		t.Errorf("expected ErrLocked, got %v", err)
	}
}

func TestIsManagedKey(t *testing.T) {
	for _, k := range []string{"smtp_host", "smtp_password_enc", "smtp_anything"} {
		if !IsManagedKey(k) {
			t.Errorf("%s should be a managed key", k)
		}
	}
	for _, k := range []string{"week_start", "breach_monitor", "org_name"} {
		if IsManagedKey(k) {
			t.Errorf("%s should not be a managed key", k)
		}
	}
}

// The master switch: while email is off, nothing is sent by any path, the saved
// settings survive, and turning it back on resumes (catching up a due report).
func TestMasterSwitchStopsAllSending(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()
	_ = fx.svc.Admins.SetMonthlyReport(ctx, fx.adminA, true, "2026-09")

	if err := fx.svc.SetEnabled(ctx, false); err != nil {
		t.Fatal(err)
	}
	cur, _ := fx.svc.LoadSMTP(ctx)
	if cur.Enabled || !cur.Configured || !cur.HasPassword || cur.Host != "smtp.resend.com" {
		t.Fatalf("turning email off must keep the saved settings: %+v", cur)
	}
	if _, err := fx.svc.MailerConfig(ctx); !errors.Is(err, ErrDisabled) {
		t.Fatalf("MailerConfig while off = %v, want ErrDisabled", err)
	}
	if err := fx.svc.SendTest(ctx, alice); !errors.Is(err, ErrDisabled) {
		t.Errorf("SendTest while off = %v, want ErrDisabled", err)
	}
	if _, err := fx.svc.SendPreview(ctx, alice); !errors.Is(err, ErrDisabled) {
		t.Errorf("SendPreview while off = %v, want ErrDisabled", err)
	}
	fx.svc.RunIfDue(ctx)
	if len(fx.sent) != 0 {
		t.Fatalf("scheduled report was sent while email was off (%d)", len(fx.sent))
	}

	if err := fx.svc.SetEnabled(ctx, true); err != nil {
		t.Fatal(err)
	}
	fx.svc.RunIfDue(ctx)
	if len(fx.sent) != 1 {
		t.Fatalf("expected the due report to go out once email was turned back on, sent=%d", len(fx.sent))
	}
}

// Email starts switched off on a fresh install.
func TestEmailDefaultsToOff(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()
	_ = fx.svc.Settings.Set(ctx, KeyEnabled, "") // behave like a key that was never set
	if cur, _ := fx.svc.LoadSMTP(ctx); cur.Enabled {
		t.Fatal("email should default to off")
	}
}

// --- payment reminders -------------------------------------------------------

func item(service, due string, status string, billing int, withDate bool) store.AccountListItem {
	a := store.AccountListItem{Service: service, Status: status, BillingPeriodMonths: billing}
	if withDate {
		d, _ := time.Parse("2006-01-02", due)
		a.NextPaymentDate = &d
	}
	return a
}

func TestDueItemsSelection(t *testing.T) {
	// 23:30 in UTC+3 is still 2 Nov locally: "today" is the local calendar date.
	now := time.Date(2026, 11, 2, 23, 30, 0, 0, time.FixedZone("AST", 3*3600))
	accounts := []store.AccountListItem{
		item("Eight days", "2026-11-10", "active", 1, true), // outside the window
		item("Seven days", "2026-11-09", "active", 1, true), // last day inside it
		item("Today", "2026-11-02", "active", 12, true),     // due today
		item("Overdue", "2026-10-29", "active", 1, true),    // 4 days late
		item("Inactive", "2026-11-03", "inactive", 1, true), // deactivated: ignored
		item("Free plan", "2026-11-03", "active", 0, true),  // not recurring: ignored
		item("No date", "", "active", 1, false),             // nothing to remind about
		item("Tomorrow", "2026-11-03", "active", 1, true),
	}
	got := DueItems(accounts, now)
	var names []string
	for _, g := range got {
		names = append(names, g.Service)
	}
	if want := "Overdue,Today,Tomorrow,Seven days"; strings.Join(names, ",") != want {
		t.Fatalf("due = %v, want %s (most urgent first)", names, want)
	}
	if got[0].Days != -4 || got[1].Days != 0 || got[3].Days != 7 {
		t.Errorf("days = %d/%d/%d, want -4/0/7", got[0].Days, got[1].Days, got[3].Days)
	}
}

func TestReminderMessageContent(t *testing.T) {
	amt := 15.0
	m := reminderMessage(Recipient{Name: "Alice", Email: "a@example.com", Lang: LangEN}, []DueItem{
		{Service: "Figma", Due: time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC), Days: -3, Amount: &amt},
		{Service: "Zoom", Due: time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC), Days: 3},
		{Service: "Slack", Due: time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC), Days: 1},
	})
	if m.Subject != "Account Manager: 3 payments due" {
		t.Errorf("subject = %q", m.Subject)
	}
	for _, want := range []string{"Hi Alice", "Figma: 2026-10-30 (3 days overdue) - $15.00", "Zoom: 2026-11-05 (due in 3 days)", "Slack: 2026-11-03 (due in 1 day)", "Take action by updating account info with the latest payment details or deactivate account."} {
		if !strings.Contains(m.Body, want) {
			t.Errorf("body is missing %q:\n%s", want, m.Body)
		}
	}
	if one := reminderMessage(Recipient{Name: "", Email: "a@example.com", Lang: LangEN}, []DueItem{{Service: "Zoom", Days: 0}}); one.Subject != "Account Manager: 1 payment due" || !strings.Contains(one.Body, "due today") {
		t.Errorf("single/today case: %q / %s", one.Subject, one.Body)
	}
}

func TestReminderSentOncePerDay(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()
	_ = fx.svc.Admins.SetPaymentReminders(ctx, fx.adminA, true, "2026-11-01") // opted in yesterday

	fx.svc.RunRemindersIfDue(ctx)
	if len(fx.sent) != 1 || fx.sent[0].To[0] != "a@example.com" {
		t.Fatalf("sent = %d, want one reminder to a@example.com", len(fx.sent))
	}
	m := fx.sent[0]
	// Only the upcoming payment: overdue items (Figma) are never reminded about.
	if m.Subject != "Account Manager: 1 payment due" || len(m.Attachments) != 0 {
		t.Errorf("subject %q, attachments %d", m.Subject, len(m.Attachments))
	}
	if !strings.Contains(m.Body, "Zoom: 2026-11-05 (due in 3 days)") || strings.Contains(m.Body, "Figma") {
		t.Errorf("unexpected body:\n%s", m.Body)
	}

	fx.svc.RunRemindersIfDue(ctx) // same day: no second reminder
	if len(fx.sent) != 1 {
		t.Fatalf("a second reminder went out the same day (%d)", len(fx.sent))
	}
	fx.now = fx.now.Add(24 * time.Hour) // next morning: reminded again
	fx.svc.RunRemindersIfDue(ctx)
	if len(fx.sent) != 2 {
		t.Fatalf("expected a fresh reminder the next day, sent=%d", len(fx.sent))
	}
}

func TestReminderWaitsForMorningAndSkipsOptInDay(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()
	_ = fx.svc.Admins.SetPaymentReminders(ctx, fx.adminA, true, "2026-11-01")

	fx.now = time.Date(2026, 11, 2, ReminderHour-1, 59, 0, 0, time.UTC)
	fx.svc.RunRemindersIfDue(ctx)
	if len(fx.sent) != 0 {
		t.Fatal("reminder sent before the morning cut-off")
	}
	fx.now = time.Date(2026, 11, 2, ReminderHour, 0, 0, 0, time.UTC)
	fx.svc.RunRemindersIfDue(ctx)
	if len(fx.sent) != 1 {
		t.Fatalf("expected the reminder at %02d:00, sent=%d", ReminderHour, len(fx.sent))
	}

	// B opts in today using the real baseline: no reminder today, one tomorrow.
	_ = fx.svc.Admins.SetPaymentReminders(ctx, fx.adminB, true, fx.svc.BaselineDay())
	fx.svc.RunRemindersIfDue(ctx)
	if len(fx.sent) != 1 {
		t.Fatalf("opting in triggered a same-day reminder (%d)", len(fx.sent))
	}
	fx.now = fx.now.Add(24 * time.Hour)
	fx.svc.RunRemindersIfDue(ctx)
	if len(fx.sent) != 3 { // A again + B's first
		t.Fatalf("expected reminders for both admins the next day, sent=%d", len(fx.sent))
	}
}

func TestReminderSkippedWhenNothingDue(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()
	_ = fx.svc.Admins.SetPaymentReminders(ctx, fx.adminA, true, "2025-12-31")
	fx.now = time.Date(2026, 1, 15, 9, 0, 0, 0, time.UTC) // everything is months away
	fx.svc.RunRemindersIfDue(ctx)
	if len(fx.sent) != 0 {
		t.Fatalf("sent a reminder with nothing due (%d)", len(fx.sent))
	}
	fx.now = time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC) // until they come due
	fx.svc.RunRemindersIfDue(ctx)
	if len(fx.sent) != 1 {
		t.Fatalf("the skipped day must not suppress a later reminder, sent=%d", len(fx.sent))
	}
}

func TestReminderRespectsMasterSwitchLockAndBackoff(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()
	_ = fx.svc.Admins.SetPaymentReminders(ctx, fx.adminA, true, "2026-11-01")

	_ = fx.svc.SetEnabled(ctx, false)
	fx.svc.RunRemindersIfDue(ctx)
	_ = fx.svc.SetEnabled(ctx, true)
	fx.locked = true
	fx.svc.RunRemindersIfDue(ctx)
	fx.locked = false
	if len(fx.sent) != 0 {
		t.Fatalf("sent while email was off or the vault locked (%d)", len(fx.sent))
	}

	var audited []string
	fx.svc.Audit = func(action, _, _, result string) { audited = append(audited, action+":"+result) }
	fx.failN = 1
	fx.svc.RunRemindersIfDue(ctx) // fails
	fx.svc.RunRemindersIfDue(ctx) // inside the backoff window
	if len(fx.sent) != 0 || len(audited) != 1 || audited[0] != "reminder.send:fail" {
		t.Fatalf("sent=%d audited=%v, want one failed attempt and no retry yet", len(fx.sent), audited)
	}
	fx.now = fx.now.Add(retryEvery + time.Minute)
	fx.svc.RunRemindersIfDue(ctx)
	if len(fx.sent) != 1 {
		t.Fatalf("expected the retry to deliver, sent=%d", len(fx.sent))
	}
}

func TestReminderPreviewDoesNotSuppressTheRealOne(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()
	_ = fx.svc.Admins.SetPaymentReminders(ctx, fx.adminA, true, "2026-11-01")

	n, err := fx.svc.SendReminderPreview(ctx, alice)
	if err != nil || n != 2 || len(fx.sent) != 1 {
		t.Fatalf("preview n=%d err=%v sent=%d", n, err, len(fx.sent))
	}
	fx.svc.RunRemindersIfDue(ctx)
	if len(fx.sent) != 2 {
		t.Fatalf("the scheduled reminder was suppressed by the preview (%d)", len(fx.sent))
	}

	// Nothing due: nothing is sent and the count is 0.
	fx.now = time.Date(2026, 1, 15, 9, 0, 0, 0, time.UTC)
	before := len(fx.sent)
	if n, err := fx.svc.SendReminderPreview(ctx, alice); err != nil || n != 0 || len(fx.sent) != before {
		t.Fatalf("empty preview n=%d err=%v sent=%d", n, err, len(fx.sent)-before)
	}
}

// --- password rotation reminders ---------------------------------------------

func rotItem(service, login string, changed string, every int, status string) store.AccountListItem {
	t, _ := time.Parse("2006-01-02 15:04:05", changed)
	a := store.AccountListItem{Service: service, Status: status, PasswordChangedAt: t}
	if every > 0 {
		e := every
		a.RotateEveryDays = &e
	}
	if login != "" {
		l := login
		a.Login = &l
	}
	return a
}

func TestRotationItemsSelection(t *testing.T) {
	// 23:30 in UTC+3 is still 2 Nov locally: "today" is the local calendar date.
	now := time.Date(2026, 11, 2, 23, 30, 0, 0, time.FixedZone("AST", 3*3600))
	accounts := []store.AccountListItem{
		rotItem("Eight days", "", "2026-10-11 09:00:00", 30, "active"), // due 11-10: outside the window
		rotItem("Seven days", "", "2026-10-10 09:00:00", 30, "active"), // due 11-09: last day inside it
		rotItem("Today", "", "2026-10-03 09:00:00", 30, "active"),      // due 11-02: today
		rotItem("Overdue", "", "2026-09-28 09:00:00", 30, "active"),    // due 10-28: 5 days late
		rotItem("Inactive", "", "2026-09-01 09:00:00", 30, "inactive"), // deactivated: ignored
		rotItem("No policy", "", "2026-01-01 09:00:00", 0, "active"),   // never rotates: ignored
	}
	got := RotationItems(accounts, now)
	var names []string
	for _, g := range got {
		names = append(names, g.Service)
	}
	if want := "Overdue,Today,Seven days"; strings.Join(names, ",") != want {
		t.Fatalf("due = %v, want %s (most urgent first)", names, want)
	}
	if got[0].Days != -5 || got[1].Days != 0 || got[2].Days != 7 {
		t.Errorf("days = %d/%d/%d, want -5/0/7", got[0].Days, got[1].Days, got[2].Days)
	}
}

func TestRotationMessageContent(t *testing.T) {
	m := rotationMessage(Recipient{Name: "Alice", Email: "a@example.com", Lang: LangEN}, []RotationItem{
		{Service: "Slack", Login: "ops@example.com", Due: time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC), Days: -2},
		{Service: "Trello", Due: time.Date(2026, 11, 7, 0, 0, 0, 0, time.UTC), Days: 5},
		{Service: "Zoom", Due: time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC), Days: 0},
	})
	if m.Subject != "Account Manager: 3 passwords due for rotation" {
		t.Errorf("subject = %q", m.Subject)
	}
	for _, want := range []string{
		"Hi Alice",
		"Slack (ops@example.com): change by 2026-10-31 (2 days overdue)",
		"Trello: change by 2026-11-07 (due in 5 days)",
		"Zoom: change by 2026-11-02 (due today)",
		"Take action by changing the password and saving the new one in Account Manager, or deactivate the account.",
		"Settings → Admin → Send Emails",
	} {
		if !strings.Contains(m.Body, want) {
			t.Errorf("body is missing %q:\n%s", want, m.Body)
		}
	}
	if strings.Contains(m.Body, "password:") || strings.Contains(strings.ToLower(m.Body), "secret") {
		t.Error("a rotation email must never contain password material")
	}
	if one := rotationMessage(Recipient{Name: "", Email: "a@example.com", Lang: LangEN}, []RotationItem{{Service: "Zoom", Days: 1}}); one.Subject != "Account Manager: 1 password due for rotation" {
		t.Errorf("singular subject = %q", one.Subject)
	}
}

func TestRotationReminderDailyFlow(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()
	var audited []string
	fx.svc.Audit = func(action, _, _, result string) { audited = append(audited, action+":"+result) }
	_ = fx.svc.Admins.SetRotationReminders(ctx, fx.adminA, true, "2026-11-01") // opted in yesterday

	fx.svc.RunRotationRemindersIfDue(ctx)
	if len(fx.sent) != 1 || fx.sent[0].To[0] != "a@example.com" {
		t.Fatalf("sent = %d, want one rotation reminder to a@example.com", len(fx.sent))
	}
	m := fx.sent[0]
	// Only the upcoming rotation: overdue ones (Slack) are never reminded about.
	if m.Subject != "Account Manager: 1 password due for rotation" || len(m.Attachments) != 0 {
		t.Errorf("subject %q, attachments %d", m.Subject, len(m.Attachments))
	}
	if !strings.Contains(m.Body, "Trello: change by 2026-11-07 (due in 5 days)") || strings.Contains(m.Body, "Slack") {
		t.Errorf("unexpected body:\n%s", m.Body)
	}
	if len(audited) != 1 || audited[0] != "rotation.send:ok" {
		t.Errorf("audit = %v, want [rotation.send:ok]", audited)
	}

	fx.svc.RunRotationRemindersIfDue(ctx) // same day: no second one
	if len(fx.sent) != 1 {
		t.Fatalf("a second rotation reminder went out the same day (%d)", len(fx.sent))
	}
	fx.now = fx.now.Add(24 * time.Hour) // next morning: reminded again
	fx.svc.RunRotationRemindersIfDue(ctx)
	if len(fx.sent) != 2 {
		t.Fatalf("expected a fresh rotation reminder the next day, sent=%d", len(fx.sent))
	}
}

// Rotation and payment reminders are independent: separate switches, separate
// once-a-day markers, separate emails.
func TestRotationAndPaymentRemindersAreIndependent(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()
	_ = fx.svc.Admins.SetRotationReminders(ctx, fx.adminA, true, "2026-11-01")

	fx.svc.RunRemindersIfDue(ctx) // payment reminders are off for A
	if len(fx.sent) != 0 {
		t.Fatalf("a payment reminder went out although only rotation is on (%d)", len(fx.sent))
	}
	fx.svc.RunRotationRemindersIfDue(ctx)
	if len(fx.sent) != 1 || !strings.Contains(fx.sent[0].Subject, "for rotation") {
		t.Fatalf("expected exactly the rotation email, got %d: %v", len(fx.sent), fx.sent)
	}

	// Now turn payment reminders on too: both kinds go out once, each with its own marker.
	_ = fx.svc.Admins.SetPaymentReminders(ctx, fx.adminA, true, "2026-11-01")
	fx.svc.RunRemindersIfDue(ctx)
	fx.svc.RunRotationRemindersIfDue(ctx) // already sent today → nothing new
	if len(fx.sent) != 2 || !strings.Contains(fx.sent[1].Subject, "payment due") {
		t.Fatalf("expected one payment email on top of the rotation one, got %d", len(fx.sent))
	}
}

func TestRotationOptInSkipsTodayAndNothingDueSendsNothing(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()
	// Opting in today with the real baseline: no reminder today, one tomorrow.
	_ = fx.svc.Admins.SetRotationReminders(ctx, fx.adminB, true, fx.svc.BaselineDay())
	fx.svc.RunRotationRemindersIfDue(ctx)
	if len(fx.sent) != 0 {
		t.Fatalf("opting in triggered a same-day reminder (%d)", len(fx.sent))
	}
	fx.now = fx.now.Add(24 * time.Hour)
	fx.svc.RunRotationRemindersIfDue(ctx)
	if len(fx.sent) != 1 {
		t.Fatalf("expected the first reminder the next day, sent=%d", len(fx.sent))
	}

	// Nothing due: nothing is sent, the day is not consumed, and Send Now says 0.
	_ = fx.svc.Admins.SetRotationReminders(ctx, fx.adminA, true, "2025-12-31")
	fx.now = time.Date(2026, 1, 15, 9, 0, 0, 0, time.UTC)
	before := len(fx.sent)
	fx.svc.RunRotationRemindersIfDue(ctx)
	if n, err := fx.svc.SendRotationPreview(ctx, alice); err != nil || n != 0 || len(fx.sent) != before {
		t.Fatalf("nothing due: n=%d err=%v sent=%d", n, err, len(fx.sent)-before)
	}
}

func TestRotationRespectsMasterSwitchAndPreviewDoesNotSuppress(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()
	_ = fx.svc.Admins.SetRotationReminders(ctx, fx.adminA, true, "2026-11-01")

	_ = fx.svc.SetEnabled(ctx, false)
	fx.svc.RunRotationRemindersIfDue(ctx)
	if _, err := fx.svc.SendRotationPreview(ctx, alice); !errors.Is(err, ErrDisabled) {
		t.Errorf("preview while email is off = %v, want ErrDisabled", err)
	}
	if len(fx.sent) != 0 {
		t.Fatalf("sent while email was off (%d)", len(fx.sent))
	}
	_ = fx.svc.SetEnabled(ctx, true)

	n, err := fx.svc.SendRotationPreview(ctx, alice)
	if err != nil || n != 2 || len(fx.sent) != 1 {
		t.Fatalf("preview n=%d err=%v sent=%d", n, err, len(fx.sent))
	}
	fx.svc.RunRotationRemindersIfDue(ctx)
	if len(fx.sent) != 2 {
		t.Fatalf("the scheduled reminder was suppressed by the preview (%d)", len(fx.sent))
	}
}

// --- upcoming payments (start-of-month email, mirrors Finance → Monthly Costs) --

func upItem(service, login string, amount *float64, next string, period int, status string) store.AccountListItem {
	a := store.AccountListItem{Service: service, Status: status, BillingPeriodMonths: period, Amount: amount}
	if next != "" {
		d, _ := time.Parse("2006-01-02", next)
		a.NextPaymentDate = &d
	}
	if login != "" {
		l := login
		a.Login = &l
	}
	return a
}

func f64(v float64) *float64 { return &v }

func upNames(items []UpcomingItem) string {
	var n []string
	for _, it := range items {
		n = append(n, it.Service+"@"+it.Date.Format("01-02"))
	}
	return strings.Join(n, ",")
}

func TestProjectMonthFollowsTheMonthlyCostsRules(t *testing.T) {
	nov := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	accounts := []store.AccountListItem{
		upItem("Monthly", "", f64(10), "2026-10-15", 1, "active"),         // steps forward into Nov 15
		upItem("Annual here", "", f64(5), "2026-11-24", 12, "active"),     // pays this month
		upItem("Annual past", "", f64(8), "2025-11-24", 12, "active"),     // last year's date steps to Nov 24
		upItem("Quarterly clamp", "", f64(20), "2026-08-31", 3, "active"), // 31 Aug + 3 months = 30 Nov (clamped)
		upItem("Quarterly skip", "", f64(9), "2026-09-15", 3, "active"),   // next hit is Dec 15, not Nov
		upItem("One-time in", "", f64(30), "2026-11-10", 0, "active"),
		upItem("One-time out", "", f64(30), "2026-12-10", 0, "active"),
		upItem("Dec 1 edge", "", f64(4), "2026-12-01", 1, "active"), // first charge is after the month
		upItem("Inactive", "", f64(4), "2026-11-12", 1, "inactive"),
		upItem("No amount", "", nil, "2026-11-12", 1, "active"),
		upItem("Zero amount", "", f64(0), "2026-11-12", 1, "active"),
		upItem("No date", "", f64(4), "", 1, "active"),
	}
	got := ProjectMonth(accounts, nov)
	want := "One-time in@11-10,Monthly@11-15,Annual here@11-24,Annual past@11-24,Quarterly clamp@11-30"
	if upNames(got) != want {
		t.Fatalf("November = %s\nwant       %s", upNames(got), want)
	}

	// Short months: a 31st start lands on the last day, including leap years.
	jan31 := []store.AccountListItem{upItem("Jan31", "", f64(1), "2027-01-31", 1, "active")}
	for month, want := range map[time.Time]string{
		time.Date(2027, 2, 1, 0, 0, 0, 0, time.UTC): "Jan31@02-28",
		time.Date(2027, 4, 1, 0, 0, 0, 0, time.UTC): "Jan31@04-30",
		time.Date(2028, 2, 1, 0, 0, 0, 0, time.UTC): "Jan31@02-29", // leap year
	} {
		if got := upNames(ProjectMonth(jan31, month)); got != want {
			t.Errorf("%s = %s, want %s", month.Format("2006-01"), got, want)
		}
	}
	// A month before the first charge has nothing; the projection never goes backwards.
	if got := ProjectMonth(jan31, time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)); len(got) != 0 {
		t.Errorf("a month before the first charge should be empty, got %v", got)
	}
}

func TestUpcomingMessageContent(t *testing.T) {
	conv := NewConverter(testCurrencies())
	month := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	m := upcomingMessage(Recipient{Name: "Alice", Email: "a@example.com", Lang: LangEN}, month, []UpcomingItem{
		{Service: "Zoom", Login: "ops@example.com", Date: time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC), Amount: 12.5},
		{Service: "Figma", Date: time.Date(2026, 11, 30, 0, 0, 0, 0, time.UTC), Amount: 7.5},
	}, conv)
	if m.Subject != "Account Manager: 2 payments coming up in November 2026" {
		t.Errorf("subject = %q", m.Subject)
	}
	for _, want := range []string{
		"Hi Alice",
		"coming up in November 2026: $20.00 / SAR 75.00 in total",
		"2026-11-05: Zoom (ops@example.com) - $12.50",
		"2026-11-30: Figma - $7.50",
		"Finance → Monthly Costs",
		"Settings → Admin → Send Emails",
	} {
		if !strings.Contains(m.Body, want) {
			t.Errorf("body is missing %q:\n%s", want, m.Body)
		}
	}
	one := upcomingMessage(Recipient{Name: "", Email: "a@example.com", Lang: LangEN}, month, []UpcomingItem{{Service: "Zoom", Date: month, Amount: 1}}, conv)
	if one.Subject != "Account Manager: 1 payment coming up in November 2026" {
		t.Errorf("singular subject = %q", one.Subject)
	}
}

func TestUpcomingEmailIsSentOncePerMonth(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()
	var audited []string
	fx.svc.Audit = func(action, _, _, result string) { audited = append(audited, action+":"+result) }
	_ = fx.svc.Admins.SetUpcomingPayments(ctx, fx.adminA, true, "2026-10") // opted in last month

	fx.svc.RunUpcomingIfDue(ctx)
	if len(fx.sent) != 1 || fx.sent[0].To[0] != "a@example.com" {
		t.Fatalf("sent = %d, want one email to a@example.com", len(fx.sent))
	}
	m := fx.sent[0]
	if m.Subject != "Account Manager: 2 payments coming up in November 2026" || len(m.Attachments) != 0 {
		t.Errorf("subject %q, attachments %d", m.Subject, len(m.Attachments))
	}
	// Zoom pays on the 5th; Figma's 30 Oct charge steps to 30 Nov.
	if !strings.Contains(m.Body, "2026-11-05: Zoom - $12.50") || !strings.Contains(m.Body, "2026-11-30: Figma - $15.00") ||
		!strings.Contains(m.Body, "$27.50") {
		t.Errorf("unexpected body:\n%s", m.Body)
	}
	if len(audited) != 1 || audited[0] != "upcoming.send:ok" {
		t.Errorf("audit = %v, want [upcoming.send:ok]", audited)
	}

	fx.svc.RunUpcomingIfDue(ctx) // same month: nothing more
	if len(fx.sent) != 1 {
		t.Fatalf("a second email went out in the same month (%d)", len(fx.sent))
	}

	// 1 December: before the morning cut-off nothing; after it, December's list.
	fx.now = time.Date(2026, 12, 1, UpcomingHour-1, 30, 0, 0, time.UTC)
	fx.svc.RunUpcomingIfDue(ctx)
	if len(fx.sent) != 1 {
		t.Fatal("sent before the morning cut-off on the 1st")
	}
	fx.now = time.Date(2026, 12, 1, UpcomingHour, 0, 0, 0, time.UTC)
	fx.svc.RunUpcomingIfDue(ctx)
	if len(fx.sent) != 2 || !strings.Contains(fx.sent[1].Subject, "December 2026") ||
		!strings.Contains(fx.sent[1].Body, "2026-12-05: Zoom") || !strings.Contains(fx.sent[1].Body, "2026-12-30: Figma") {
		t.Fatalf("expected December's list on 1 December, got %d: %v", len(fx.sent), fx.sent)
	}
}

func TestUpcomingCatchesUpWhenTheAppWasOffOnThe1st(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()
	_ = fx.svc.Admins.SetUpcomingPayments(ctx, fx.adminA, true, "2026-10")
	fx.now = time.Date(2026, 11, 17, 14, 0, 0, 0, time.UTC) // app was off until the 17th
	fx.svc.RunUpcomingIfDue(ctx)
	if len(fx.sent) != 1 || !strings.Contains(fx.sent[0].Subject, "November 2026") {
		t.Fatalf("expected a catch-up email for November, got %d", len(fx.sent))
	}
}

func TestUpcomingOptInSkipsThisMonthAndEmptyMonthIsSettledQuietly(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()

	// Opting in mid-month with the real baseline: no email now, one next month.
	_ = fx.svc.Admins.SetUpcomingPayments(ctx, fx.adminB, true, fx.svc.CurrentMonth())
	fx.svc.RunUpcomingIfDue(ctx)
	if len(fx.sent) != 0 {
		t.Fatalf("opting in triggered this month's email (%d)", len(fx.sent))
	}
	fx.now = time.Date(2026, 12, 1, 9, 0, 0, 0, time.UTC)
	fx.svc.RunUpcomingIfDue(ctx)
	if len(fx.sent) != 1 {
		t.Fatalf("expected the first email next month, sent=%d", len(fx.sent))
	}

	// A month with nothing scheduled sends nothing and is settled, so it is not
	// retried every 15 minutes or sent late if something is added mid-month.
	_ = fx.svc.Admins.SetUpcomingPayments(ctx, fx.adminA, true, "2025-12")
	fx.now = time.Date(2026, 1, 15, 9, 0, 0, 0, time.UTC)
	before := len(fx.sent)
	fx.svc.RunUpcomingIfDue(ctx)
	if len(fx.sent) != before {
		t.Fatalf("sent an email for a month with no payments (%d)", len(fx.sent)-before)
	}
	if left, _ := fx.svc.Admins.ListUpcomingDue(ctx, "2026-01"); len(left) != 0 {
		t.Fatalf("an empty month should be settled, but %d admin(s) are still due", len(left))
	}
}

func TestUpcomingPreviewAndMasterSwitch(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()
	_ = fx.svc.Admins.SetUpcomingPayments(ctx, fx.adminA, true, "2026-10")

	_ = fx.svc.SetEnabled(ctx, false)
	fx.svc.RunUpcomingIfDue(ctx)
	if _, err := fx.svc.SendUpcomingPreview(ctx, alice); !errors.Is(err, ErrDisabled) {
		t.Errorf("preview while email is off = %v, want ErrDisabled", err)
	}
	if len(fx.sent) != 0 {
		t.Fatalf("sent while email was off (%d)", len(fx.sent))
	}
	_ = fx.svc.SetEnabled(ctx, true)

	n, err := fx.svc.SendUpcomingPreview(ctx, alice)
	if err != nil || n != 2 || len(fx.sent) != 1 {
		t.Fatalf("preview n=%d err=%v sent=%d", n, err, len(fx.sent))
	}
	fx.svc.RunUpcomingIfDue(ctx) // the preview must not use up the real one
	if len(fx.sent) != 2 {
		t.Fatalf("the scheduled email was suppressed by the preview (%d)", len(fx.sent))
	}

	fx.now = time.Date(2026, 1, 15, 9, 0, 0, 0, time.UTC) // nothing scheduled that month
	before := len(fx.sent)
	if n, err := fx.svc.SendUpcomingPreview(ctx, alice); err != nil || n != 0 || len(fx.sent) != before {
		t.Fatalf("empty preview n=%d err=%v sent=%d", n, err, len(fx.sent)-before)
	}
}

// --- language: emails follow each admin's own language -----------------------

func TestArabicPluralsMatchTheAppsForms(t *testing.T) {
	cases := map[int]string{
		1: "دفعة واحدة", 2: "دفعتان", 3: "3 دفعات", 10: "10 دفعات", 11: "11 دفعة",
		99: "99 دفعة", 100: "100 دفعة", 103: "103 دفعات", 111: "111 دفعة",
	}
	for n, want := range cases {
		if got := nounPayment.of(LangAR, n); got != want {
			t.Errorf("payments(%d) = %q, want %q", n, got, want)
		}
	}
	if nounPassword.of(LangAR, 2) != "كلمتا مرور" || nounPassword.of(LangAR, 5) != "5 كلمات مرور" {
		t.Error("password plurals are wrong")
	}
	if nounPayment.of(LangEN, 1) != "1 payment" || nounPayment.of(LangEN, 3) != "3 payments" {
		t.Error("English plurals changed")
	}
}

// The Arabic "due in / overdue" wording is the app's own (home.inDays_* /
// home.overdue_* in ar.json). Read it from the UI's locale file so the emails can't
// drift from what the app says.
func TestArabicWhenTextMatchesTheUI(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "web", "src", "locales", "ar.json"))
	if err != nil {
		t.Skipf("web locale not available: %v", err)
	}
	var ar struct {
		Home map[string]string `json:"home"`
	}
	if err := json.Unmarshal(raw, &ar); err != nil {
		t.Fatal(err)
	}
	form := func(n int) string { // CLDR Arabic plural category
		switch m := n % 100; {
		case n == 1:
			return "one"
		case n == 2:
			return "two"
		case m >= 3 && m <= 10:
			return "few"
		case m >= 11:
			return "many"
		}
		return "other"
	}
	for _, n := range []int{1, 2, 3, 5, 10, 11, 12, 99, 100} {
		for prefix, sign := range map[string]int{"inDays": 1, "overdue": -1} {
			want := strings.ReplaceAll(ar.Home[prefix+"_"+form(n)], "{{count, number}}", fmt.Sprint(n))
			if want == "" {
				t.Fatalf("home.%s_%s missing in ar.json", prefix, form(n))
			}
			if got := whenText(LangAR, sign*n); got != want {
				t.Errorf("%s %d: email says %q, UI says %q", prefix, n, got, want)
			}
		}
	}
	if whenText(LangAR, 0) != "مستحقة اليوم" || whenText(LangEN, 0) != "due today" {
		t.Error("due-today wording")
	}
}

func TestArabicMonthNames(t *testing.T) {
	if got := monthLabel(LangAR, time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)); got != "نوفمبر 2026" {
		t.Errorf("november = %q", got)
	}
	seen := map[string]bool{}
	for m := time.January; m <= time.December; m++ {
		name := monthLabel(LangAR, time.Date(2026, m, 1, 0, 0, 0, 0, time.UTC))
		if seen[name] || name == "" {
			t.Errorf("duplicate or empty Arabic month name %q", name)
		}
		seen[name] = true
	}
	if monthLabel(LangEN, time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)) != "November 2026" {
		t.Error("English month label changed")
	}
}

var arAdmin = Recipient{Name: "فياض", Email: "a@example.com", Lang: LangAR}

func lines(body string) []string {
	var out []string
	for _, l := range strings.Split(body, "\n") {
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}

func TestArabicEmailsAreRightToLeftWithIsolatedTokens(t *testing.T) {
	conv := NewConverter(testCurrencies())
	amt := 15.0
	month := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	due := time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC)

	msgs := map[string]struct {
		subject string
		body    string
	}{}
	rem := reminderMessage(arAdmin, []DueItem{{Service: "Figma", Due: due, Days: -3, Amount: &amt}, {Service: "Zoom", Due: due, Days: 3}})
	rot := rotationMessage(arAdmin, []RotationItem{{Service: "Slack", Login: "ops@example.com", Due: due, Days: -2}, {Service: "Trello", Due: due, Days: 5}})
	up := upcomingMessage(arAdmin, month, []UpcomingItem{{Service: "Zoom", Login: "ops@example.com", Date: due, Amount: 20}, {Service: "Figma", Date: due, Amount: 7.5}}, conv)
	rep := monthMessage(arAdmin, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), 2, []byte("x"))
	tst := testMessage(arAdmin)
	for name, m := range map[string]mailer.Message{"reminder": rem, "rotation": rot, "upcoming": up, "report": rep, "test": tst} {
		msgs[name] = struct{ subject, body string }{m.Subject, m.Body}
	}

	wantSubject := map[string]string{
		"reminder": "مدير الحسابات: دفعات مستحقة (2)",
		"rotation": "مدير الحسابات: كلمات مرور تحتاج إلى تغيير (2)",
		"upcoming": "مدير الحسابات: المدفوعات القادمة في نوفمبر 2026 (2)",
		"report":   "مدير الحسابات: مدفوعات أكتوبر 2026",
		"test":     "مدير الحسابات: بريد تجريبي",
	}
	for name, m := range msgs {
		if m.subject != wantSubject[name] {
			t.Errorf("%s subject = %q, want %q", name, m.subject, wantSubject[name])
		}
		for _, l := range lines(m.body) {
			if !strings.HasPrefix(l, rlm) {
				t.Errorf("%s: line is not marked right-to-left: %q", name, l)
			}
		}
		if !strings.Contains(m.body, "الإعدادات ← المشرف ← إرسال رسائل البريد") && name != "report" && name != "test" {
			t.Errorf("%s: footer should name the Arabic menu path:\n%s", name, m.body)
		}
	}

	// user-supplied text and always-LTR tokens are isolated so they keep their order
	for _, want := range []string{fsi + "Figma" + pdi, lri + "2026-10-30" + pdi, lri + "$15.00" + pdi, "متأخرة 3 أيام", "خلال 3 أيام"} {
		if !strings.Contains(rem.Body, want) {
			t.Errorf("reminder is missing %q:\n%s", want, rem.Body)
		}
	}
	if !strings.Contains(rot.Body, fsi+"Slack"+pdi+" ("+fsi+"ops@example.com"+pdi+")") || !strings.Contains(rot.Body, "يجب تغييرها قبل") {
		t.Errorf("rotation body:\n%s", rot.Body)
	}
	if !strings.Contains(up.Body, lri+"$27.50 / SAR 103.12"+pdi) && !strings.Contains(up.Body, lri+"$27.50 / SAR 103.13"+pdi) {
		t.Errorf("upcoming total should be one isolated token:\n%s", up.Body)
	}
	if !strings.Contains(rep.Body, "(دفعتان)") || !strings.Contains(rep.Body, "أكتوبر 2026") {
		t.Errorf("report summary:\n%s", rep.Body)
	}
	if !strings.HasPrefix(rem.Body, rlm+"هلا "+fsi+"فياض"+pdi+"،") {
		t.Errorf("greeting should use the Arabic name:\n%s", rem.Body)
	}
	if empty := monthMessage(arAdmin, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), 0, nil); !strings.Contains(empty.Body, "لم تُسجَّل أي مدفوعات") {
		t.Errorf("empty-month wording:\n%s", empty.Body)
	}
}

// English emails stay exactly as they were: no direction marks, no Arabic.
func TestEnglishEmailsCarryNoBidiMarks(t *testing.T) {
	amt := 5.0
	due := time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)
	for name, m := range map[string]mailer.Message{
		"reminder": reminderMessage(alice, []DueItem{{Service: "Zoom", Due: due, Days: 3, Amount: &amt}}),
		"rotation": rotationMessage(alice, []RotationItem{{Service: "Slack", Login: "ops@example.com", Due: due, Days: 3}}),
		"upcoming": upcomingMessage(alice, due, []UpcomingItem{{Service: "Zoom", Date: due, Amount: 5}}, NewConverter(testCurrencies())),
		"report":   monthMessage(alice, due, 1, []byte("x")),
		"test":     testMessage(alice),
	} {
		for _, bad := range []string{rlm, fsi, lri, pdi} {
			if strings.Contains(m.Body, bad) || strings.Contains(m.Subject, bad) {
				t.Errorf("%s: English email contains a direction mark", name)
			}
		}
		if strings.ContainsAny(m.Subject+m.Body, "ابتثجحخدذرزسشصضطظعغفقكلمنهوي") {
			t.Errorf("%s: English email contains Arabic text", name)
		}
	}
}

func TestArabicWorkbook(t *testing.T) {
	ar1, ar2 := "أحمد", "مارك"
	p := pay("2026-10-05", 10, "USD")
	p.Seats = []string{"Ahmed", "Mark", "Sam"}
	p.SeatsAr = []*string{&ar1, &ar2, nil} // Sam has no Arabic name → falls back to English
	cat := "Design"
	p.CategoryName = &cat

	data, err := BuildPaymentsWorkbook([]store.PaymentListItem{p}, NewConverter(testCurrencies()), LangAR)
	if err != nil {
		t.Fatal(err)
	}
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if got := f.GetSheetList(); len(got) != 1 || got[0] != "المدفوعات" {
		t.Fatalf("sheets = %v, want [المدفوعات]", got)
	}
	rows, _ := f.GetRows("المدفوعات")
	wantHeader := "التاريخ|الخدمة|الفئة|المستخدمون|المبلغ (USD)|المبلغ (SAR)|تم الدفع بـ|ملاحظة|الفاتورة"
	if got := strings.Join(rows[0], "|"); got != wantHeader {
		t.Errorf("header = %s\nwant     %s", got, wantHeader)
	}
	if rows[1][3] != "أحمد, مارك, Sam" {
		t.Errorf("users = %q, want the Arabic names with an English fallback", rows[1][3])
	}
	if rows[2][0] != "الإجمالي" {
		t.Errorf("total label = %q", rows[2][0])
	}
	if opts, err := f.GetSheetView("المدفوعات", 0); err != nil || opts.RightToLeft == nil || !*opts.RightToLeft {
		t.Errorf("an Arabic sheet should be right-to-left (opts=%+v err=%v)", opts, err)
	}
	// the English workbook is unchanged and left-to-right
	en, _ := BuildPaymentsWorkbook([]store.PaymentListItem{p}, NewConverter(testCurrencies()), LangEN)
	fe, _ := excelize.OpenReader(bytes.NewReader(en))
	if got := fe.GetSheetList(); got[0] != "Payments" {
		t.Errorf("english sheet = %v", got)
	}
	if opts, _ := fe.GetSheetView("Payments", 0); opts.RightToLeft != nil && *opts.RightToLeft {
		t.Error("the English sheet must not be right-to-left")
	}
}

func TestRecipientFollowsLanguageAndArabicSetting(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()
	nameAr := "فياض"
	if err := fx.svc.Admins.UpdateProfile(ctx, fx.adminA, "Fayyaz", &nameAr, "a@example.com"); err != nil {
		t.Fatal(err)
	}
	admin := func(id string) *store.Admin {
		a, err := fx.svc.Admins.GetByID(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		return a
	}

	// default: English, English name (nobody has picked Arabic yet)
	if r := fx.svc.RecipientForAdmin(ctx, admin(fx.adminA)); r.Lang != LangEN || r.Name != "Fayyaz" {
		t.Fatalf("default = %+v", r)
	}
	if err := fx.svc.Admins.SetLanguage(ctx, fx.adminA, "ar"); err != nil {
		t.Fatal(err)
	}
	// language is "ar" but Arabic support is off in Configuration → still English
	if r := fx.svc.RecipientForAdmin(ctx, admin(fx.adminA)); r.Lang != LangEN || r.Name != "Fayyaz" {
		t.Fatalf("arabic switched off should fall back to English, got %+v", r)
	}
	_ = fx.svc.Settings.Set(ctx, "lang_arabic_enabled", "1")
	if r := fx.svc.RecipientForAdmin(ctx, admin(fx.adminA)); r.Lang != LangAR || r.Name != "فياض" {
		t.Fatalf("arabic on = %+v", r)
	}
	// an English-language admin stays English even when Arabic is enabled
	if r := fx.svc.RecipientForAdmin(ctx, admin(fx.adminB)); r.Lang != LangEN {
		t.Fatalf("english admin = %+v", r)
	}
	// Arabic reader without an Arabic name → greeted by their English name
	_ = fx.svc.Admins.SetLanguage(ctx, fx.adminB, "ar")
	if r := fx.svc.RecipientForAdmin(ctx, admin(fx.adminB)); r.Lang != LangAR || r.Name != "Bob" {
		t.Fatalf("no arabic name = %+v", r)
	}
	if err := fx.svc.Admins.SetLanguage(ctx, fx.adminA, "fr"); err == nil {
		t.Fatal("an unsupported language must be rejected")
	}
}

// One send goes to each admin in their own language, and the monthly workbook is
// built per language (not once for everybody).
func TestScheduledEmailsFollowEachAdminsLanguage(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()
	_ = fx.svc.Settings.Set(ctx, "lang_arabic_enabled", "1")
	_ = fx.svc.Admins.SetLanguage(ctx, fx.adminA, "ar") // A reads Arabic, B English
	_ = fx.svc.Admins.SetPaymentReminders(ctx, fx.adminA, true, "2026-11-01")
	_ = fx.svc.Admins.SetPaymentReminders(ctx, fx.adminB, true, "2026-11-01")
	_ = fx.svc.Admins.SetMonthlyReport(ctx, fx.adminA, true, "2026-09")
	_ = fx.svc.Admins.SetMonthlyReport(ctx, fx.adminB, true, "2026-09")

	fx.svc.RunRemindersIfDue(ctx)
	if len(fx.sent) != 2 {
		t.Fatalf("sent = %d, want 2", len(fx.sent))
	}
	bySubject := map[string]string{}
	for _, m := range fx.sent {
		bySubject[m.To[0]] = m.Subject
	}
	if bySubject["a@example.com"] != "مدير الحسابات: دفعات مستحقة (1)" {
		t.Errorf("A's subject = %q", bySubject["a@example.com"])
	}
	if bySubject["b@example.com"] != "Account Manager: 1 payment due" {
		t.Errorf("B's subject = %q", bySubject["b@example.com"])
	}

	fx.sent = nil
	fx.svc.RunIfDue(ctx) // monthly report
	if len(fx.sent) != 2 {
		t.Fatalf("report sent = %d, want 2", len(fx.sent))
	}
	sheets := map[string]string{}
	for _, m := range fx.sent {
		f, err := excelize.OpenReader(bytes.NewReader(m.Attachments[0].Data))
		if err != nil {
			t.Fatal(err)
		}
		sheets[m.To[0]] = f.GetSheetList()[0]
	}
	if sheets["a@example.com"] != "المدفوعات" || sheets["b@example.com"] != "Payments" {
		t.Errorf("workbooks per language = %v", sheets)
	}
}

// The send hours are part of the documented behaviour: the daily reminders (payments
// and password rotation) go out from 08:00, the monthly upcoming-payments email from
// 08:00 on the 1st. Pin them so a change is deliberate.
func TestEmailHoursAreAsDocumented(t *testing.T) {
	if ReminderHour != 8 || UpcomingHour != 8 {
		t.Fatalf("ReminderHour=%d UpcomingHour=%d, want 8 and 8 (update the README if this is intended)", ReminderHour, UpcomingHour)
	}
}

func TestRotationReminderWaitsForEightAM(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()
	_ = fx.svc.Admins.SetRotationReminders(ctx, fx.adminA, true, "2026-11-01")

	fx.now = time.Date(2026, 11, 2, 7, 59, 0, 0, time.UTC)
	fx.svc.RunRotationRemindersIfDue(ctx)
	if len(fx.sent) != 0 {
		t.Fatal("rotation reminder sent before 08:00")
	}
	fx.now = time.Date(2026, 11, 2, 8, 0, 0, 0, time.UTC)
	fx.svc.RunRotationRemindersIfDue(ctx)
	if len(fx.sent) != 1 {
		t.Fatalf("expected the rotation reminder at 08:00, sent=%d", len(fx.sent))
	}
}

// An admin who picked specific reminder days is emailed only about items that fall on
// those days (Zoom is 3 days away; Figma is overdue and is never reminded about by the
// schedule); Send Now still shows everything, overdue included.
func TestReminderDaysChoiceFiltersTheDailyEmail(t *testing.T) {
	run := func(days []int) *fixture {
		fx := newFixture(t)
		ctx := context.Background()
		_ = fx.svc.Admins.SetPaymentReminders(ctx, fx.adminA, true, "2026-11-01")
		mask, err := store.DaysToMask(days)
		if err != nil {
			t.Fatal(err)
		}
		if err := fx.svc.Admins.SetPaymentReminderDays(ctx, fx.adminA, mask); err != nil {
			t.Fatal(err)
		}
		fx.svc.RunRemindersIfDue(ctx)
		return fx
	}

	fx := run([]int{3})
	if len(fx.sent) != 1 || !strings.Contains(fx.sent[0].Body, "Zoom") || strings.Contains(fx.sent[0].Body, "Figma") {
		t.Fatalf("only the 3-days-before checkpoint should be listed: %v", fx.sent)
	}
	if fx = run([]int{1, 0}); len(fx.sent) != 0 {
		t.Fatalf("nothing is due on a chosen day, so nothing should be sent: %v", fx.sent)
	}
	fx = run([]int{7, 6, 5, 4, 3, 2, 1, 0}) // the default: every day before, but never overdue
	if len(fx.sent) != 1 || !strings.Contains(fx.sent[0].Body, "Zoom") || strings.Contains(fx.sent[0].Body, "Figma") {
		t.Fatalf("overdue items must not be in the scheduled email: %v", fx.sent)
	}

	// Send Now ignores the choice and lists everything in the window, overdue included.
	to := Recipient{Name: "Alice", Email: "a@example.com", Lang: LangEN}
	n, err := fx.svc.SendReminderPreview(context.Background(), to)
	if err != nil || n != 2 {
		t.Fatalf("Send Now should include all items, got n=%d err=%v", n, err)
	}
}

func TestReminderDayMasks(t *testing.T) {
	mask, err := store.DaysToMask([]int{7, 3, 0, 3})
	if err != nil {
		t.Fatal(err)
	}
	got := store.MaskToDays(mask)
	want := []int{7, 3, 0}
	if len(got) != len(want) {
		t.Fatalf("round trip: got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("round trip: got %v want %v", got, want)
		}
	}
	for _, bad := range [][]int{{}, {8}, {-1}} {
		if _, err := store.DaysToMask(bad); err == nil {
			t.Errorf("DaysToMask(%v) should fail", bad)
		}
	}
	if d := store.MaskToDays(store.AllReminderDays); d[0] != 7 || len(d) != 8 {
		t.Errorf("the default mask should select all eight days, got %v", d)
	}
	if !store.DayWanted(mask, 3) || store.DayWanted(mask, 4) || store.DayWanted(mask, -5) || store.DayWanted(mask, 8) {
		t.Error("DayWanted gave the wrong answer")
	}
	if !store.DayWanted(store.PreviewDays, -5) {
		t.Error("the Send Now mask should include overdue")
	}
}

// An account's amount is shown in its own currency, and the month's total converts
// every item to USD through the rates (SAR 100 at 4 SAR per USD is $25).
func TestEmailsUseEachAccountsOwnCurrency(t *testing.T) {
	conv := NewConverter([]store.Currency{
		{Code: "USD", Active: true, Rate: 1},
		{Code: "SAR", Active: true, Rate: 4},
	})
	month := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	items := []UpcomingItem{
		{Service: "Zoom", Date: month.AddDate(0, 0, 4), Amount: 100, Currency: "SAR"},
		{Service: "Figma", Date: month.AddDate(0, 0, 9), Amount: 5, Currency: ""},
	}
	m := upcomingMessage(Recipient{Name: "Alice", Email: "a@example.com", Lang: LangEN}, month, items, conv)
	for _, want := range []string{"SAR 100.00", "$5.00", "$30.00 / SAR 120.00 in total"} {
		if !strings.Contains(m.Body, want) {
			t.Errorf("upcoming email missing %q:\n%s", want, m.Body)
		}
	}

	amount := 12.5
	r := reminderMessage(Recipient{Name: "Alice", Email: "a@example.com", Lang: LangEN},
		[]DueItem{{Service: "Zoom", Days: 3, Amount: &amount, Currency: "SAR"}, {Service: "Figma", Days: 1, Amount: &amount}})
	if !strings.Contains(r.Body, "SAR 12.50") || !strings.Contains(r.Body, "$12.50") {
		t.Errorf("reminder should show each amount in its own currency:\n%s", r.Body)
	}
}
