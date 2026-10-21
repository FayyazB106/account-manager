package report

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/mail"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/FayyazB106/account-manager/internal/crypto"
	"github.com/FayyazB106/account-manager/internal/mailer"
	"github.com/FayyazB106/account-manager/internal/store"
)

// Setting keys for the SMTP configuration (all in app_setting). The password is
// stored envelope-encrypted under KeyPasswordEnc and must never be sent to the
// browser — see SecretKeys.
const (
	KeyHost        = "smtp_host"
	KeyPort        = "smtp_port"
	KeySecurity    = "smtp_security"
	KeyUser        = "smtp_user"
	KeyFrom        = "smtp_from"
	KeyPasswordEnc = "smtp_password_enc"
	// KeyEnabled is the master on/off switch for everything email ("on"/"off").
	// Turning it off keeps the saved settings but stops all sending.
	KeyEnabled = "smtp_enabled"
)

// SecretKeys are settings that must be stripped from any API response.
var SecretKeys = []string{KeyPasswordEnc}

// IsManagedKey reports whether a setting key belongs to the SMTP configuration,
// which may only be written through the dedicated, validating endpoint.
func IsManagedKey(key string) bool { return strings.HasPrefix(key, "smtp_") }

var (
	// ErrNotConfigured means email has not been set up yet.
	ErrNotConfigured = errors.New("email is not configured")
	// ErrDisabled means email has been switched off in Settings → Email.
	ErrDisabled = errors.New("email is turned off")
	// ErrLocked means the vault master key is unavailable, so the stored SMTP
	// password cannot be read or written.
	ErrLocked = errors.New("vault is locked")
	// ErrPasswordRequired means a username was given without a password.
	ErrPasswordRequired = errors.New("smtp password is required when a username is set")
)

// ValidationError is a user-correctable problem with submitted SMTP settings.
type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }

// SMTPSettings is the safe (password-free) view of the stored configuration.
type SMTPSettings struct {
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Security    string `json:"security"`
	Username    string `json:"username"`
	From        string `json:"from"`
	HasPassword bool   `json:"has_password"`
	Configured  bool   `json:"configured"`
	Enabled     bool   `json:"enabled"` // master switch; nothing is sent while off
}

// SMTPInput is a submitted configuration. An empty Password keeps the stored one.
type SMTPInput struct {
	Host     string
	Port     int
	Security string
	Username string
	From     string
	Password string
}

// retryEvery is how long to wait before retrying a failed scheduled send to the
// same admin, so a broken SMTP setup is not hammered (or the audit log flooded).
const retryEvery = 30 * time.Minute

// checkEvery is how often the scheduler looks for a report that is due.
const checkEvery = 15 * time.Minute

// Service owns the SMTP configuration and the monthly report.
type Service struct {
	Settings   *store.SettingStore
	Admins     *store.AdminStore
	Payments   *store.PaymentStore
	Currencies *store.CurrencyStore
	Accounts   *store.AccountStore
	KeyFn      func() []byte // master key, read lazily (may be set after startup by web unlock)
	Audit      store.AuditFunc
	Now        func() time.Time
	SendFn     func(ctx context.Context, cfg mailer.Config, msg mailer.Message) error

	mu      sync.Mutex
	lastTry map[string]time.Time
}

// New wires a Service with real time and the real SMTP sender.
func New(settings *store.SettingStore, admins *store.AdminStore, payments *store.PaymentStore,
	currencies *store.CurrencyStore, accounts *store.AccountStore, keyFn func() []byte) *Service {
	return &Service{
		Settings: settings, Admins: admins, Payments: payments, Currencies: currencies,
		Accounts: accounts, KeyFn: keyFn, Now: time.Now, SendFn: mailer.Send,
		lastTry: map[string]time.Time{},
	}
}

// SetAuditLogger wires system-event logging for scheduled sends.
func (s *Service) SetAuditLogger(fn store.AuditFunc) { s.Audit = fn }

// --- SMTP configuration ------------------------------------------------------

// LoadSMTP returns the stored configuration without the password.
func (s *Service) LoadSMTP(ctx context.Context) (SMTPSettings, error) {
	all, err := s.Settings.All(ctx)
	if err != nil {
		return SMTPSettings{}, err
	}
	port, _ := strconv.Atoi(strings.TrimSpace(all[KeyPort]))
	out := SMTPSettings{
		Host:        strings.TrimSpace(all[KeyHost]),
		Port:        port,
		Security:    strings.TrimSpace(all[KeySecurity]),
		Username:    strings.TrimSpace(all[KeyUser]),
		From:        strings.TrimSpace(all[KeyFrom]),
		HasPassword: all[KeyPasswordEnc] != "",
		Enabled:     all[KeyEnabled] == "on",
	}
	out.Configured = out.Host != "" && out.Port > 0 && out.From != "" &&
		(out.Username == "" || out.HasPassword)
	return out, nil
}

// SaveSMTP validates and stores the configuration. The password is sealed with the
// master key; leaving it empty keeps the stored one, and clearing the username
// (no login) also clears the stored password.
func (s *Service) SaveSMTP(ctx context.Context, in SMTPInput) error {
	in.Host = strings.TrimSpace(in.Host)
	in.Username = strings.TrimSpace(in.Username)
	in.From = strings.TrimSpace(in.From)
	in.Security = strings.ToLower(strings.TrimSpace(in.Security))

	cfg := mailer.Config{Host: in.Host, Port: in.Port, Security: in.Security,
		Username: in.Username, Password: in.Password, From: in.From}
	if err := cfg.Validate(); err != nil {
		return &ValidationError{Msg: err.Error()}
	}

	cur, err := s.LoadSMTP(ctx)
	if err != nil {
		return err
	}
	enc := ""
	switch {
	case in.Username == "":
		enc = "" // no login → no stored password
	case in.Password != "":
		key := s.KeyFn()
		if len(key) == 0 {
			return ErrLocked
		}
		if enc, err = crypto.SealString(key, in.Password); err != nil {
			return err
		}
	case cur.HasPassword:
		all, err := s.Settings.All(ctx)
		if err != nil {
			return err
		}
		enc = all[KeyPasswordEnc] // keep what is stored
	default:
		return &ValidationError{Msg: ErrPasswordRequired.Error()}
	}

	for k, v := range map[string]string{
		KeyHost: in.Host, KeyPort: strconv.Itoa(in.Port), KeySecurity: in.Security,
		KeyUser: in.Username, KeyFrom: in.From, KeyPasswordEnc: enc,
	} {
		if err := s.Settings.Set(ctx, k, v); err != nil {
			return err
		}
	}
	return nil
}

// SetEnabled flips the master switch. The saved configuration is untouched.
func (s *Service) SetEnabled(ctx context.Context, on bool) error {
	v := "off"
	if on {
		v = "on"
	}
	return s.Settings.Set(ctx, KeyEnabled, v)
}

// MailerConfig returns a ready-to-use mailer.Config with the password decrypted.
// It fails with ErrDisabled while the master switch is off, so every send path
// (scheduled report, test email, preview) honours it.
func (s *Service) MailerConfig(ctx context.Context) (mailer.Config, error) {
	cur, err := s.LoadSMTP(ctx)
	if err != nil {
		return mailer.Config{}, err
	}
	if !cur.Enabled {
		return mailer.Config{}, ErrDisabled
	}
	if !cur.Configured {
		return mailer.Config{}, ErrNotConfigured
	}
	cfg := mailer.Config{Host: cur.Host, Port: cur.Port, Security: cur.Security,
		Username: cur.Username, From: cur.From}
	if cur.Username != "" {
		key := s.KeyFn()
		if len(key) == 0 {
			return mailer.Config{}, ErrLocked
		}
		all, err := s.Settings.All(ctx)
		if err != nil {
			return mailer.Config{}, err
		}
		if cfg.Password, err = crypto.OpenString(key, all[KeyPasswordEnc]); err != nil {
			return mailer.Config{}, errors.New("the stored email password can't be decrypted — please re-enter it")
		}
	}
	return cfg, nil
}

// --- Recipients --------------------------------------------------------------

// recipient resolves who an email is for. It is written in the admin's own
// language, unless Arabic support has been switched off in Settings → Configuration
// — then everyone gets English, like the app itself. An Arabic reader is greeted by
// their Arabic name when they have one.
func (s *Service) recipient(ctx context.Context, name, nameAr, email, language string) Recipient {
	lang := LangEN
	if language == string(LangAR) {
		if all, err := s.Settings.All(ctx); err == nil && all["lang_arabic_enabled"] == "1" {
			lang = LangAR
		}
	}
	greet := name
	if lang == LangAR && strings.TrimSpace(nameAr) != "" {
		greet = nameAr
	}
	return Recipient{Name: greet, Email: email, Lang: lang}
}

// RecipientForAdmin is the recipient for a signed-in admin (Send Now, test email).
func (s *Service) RecipientForAdmin(ctx context.Context, a *store.Admin) Recipient {
	nameAr := ""
	if a.NameAr != nil {
		nameAr = *a.NameAr
	}
	return s.recipient(ctx, a.Name, nameAr, a.Email, a.Language)
}

// recipientFor is the recipient for an opted-in admin picked by the scheduler.
func (s *Service) recipientFor(ctx context.Context, r store.ReportRecipient) Recipient {
	return s.recipient(ctx, r.Name, r.NameAr, r.Email, r.Language)
}

// --- Reports -----------------------------------------------------------------

// BuildReport returns the workbook bytes for [from, to] in the given language plus
// the number of payments it contains.
func (s *Service) BuildReport(ctx context.Context, from, to time.Time, lang Lang) (data []byte, count int, err error) {
	all, err := s.Payments.ListAll(ctx)
	if err != nil {
		return nil, 0, err
	}
	rows := FilterByRange(all, from, to)
	currencies, err := s.Currencies.List(ctx)
	if err != nil {
		return nil, 0, err
	}
	data, err = BuildPaymentsWorkbook(rows, NewConverter(currencies), lang)
	return data, len(rows), err
}

// SendTest sends a short plain test message to one recipient.
func (s *Service) SendTest(ctx context.Context, to Recipient) error {
	cfg, err := s.MailerConfig(ctx)
	if err != nil {
		return err
	}
	return s.SendFn(ctx, cfg, testMessage(to))
}

// SendPreview emails last month's report to one recipient right now. It does not
// touch the scheduler's markers, so it never suppresses the real monthly send.
func (s *Service) SendPreview(ctx context.Context, to Recipient) (count int, err error) {
	cfg, err := s.MailerConfig(ctx)
	if err != nil {
		return 0, err
	}
	from, until, _ := PreviousMonth(s.Now())
	xlsx, count, err := s.BuildReport(ctx, from, until, to.Lang)
	if err != nil {
		return 0, err
	}
	return count, s.SendFn(ctx, cfg, monthMessage(to, from, count, xlsx))
}

// BaselineMonth is the "YYYY-MM" recorded as already-sent when an admin opts in, so
// the first report arrives when the next month completes (no backfill).
func (s *Service) BaselineMonth() string {
	_, _, label := PreviousMonth(s.Now())
	return label
}

// RunIfDue sends last month's report to every opted-in admin who has not had it.
// Safe to call often: each admin is marked once their copy is delivered, and a
// failed send is retried no more than every retryEvery. A missed 1st (app closed)
// is caught up on the next check. Each admin's copy is written in their language.
func (s *Service) RunIfDue(ctx context.Context) {
	if len(s.KeyFn()) == 0 {
		return // vault locked — can't read the SMTP password yet
	}
	from, until, month := PreviousMonth(s.Now())
	due, err := s.Admins.ListReportDue(ctx, month)
	if err != nil || len(due) == 0 {
		return
	}
	cfg, err := s.MailerConfig(ctx)
	if err != nil {
		return // not configured / undecryptable: stay quiet, retry next check
	}

	// The workbook only depends on the language, so build it once per language.
	type built struct {
		data  []byte
		count int
	}
	byLang := map[Lang]built{}

	for _, r := range due {
		s.mu.Lock()
		last, tried := s.lastTry[r.ID]
		if tried && s.Now().Sub(last) < retryEvery {
			s.mu.Unlock()
			continue
		}
		s.lastTry[r.ID] = s.Now()
		s.mu.Unlock()

		if _, perr := mail.ParseAddress(r.Email); perr != nil {
			s.audit("report.send", r.Email, month+": invalid address", "fail")
			continue
		}
		to := s.recipientFor(ctx, r)
		b, ok := byLang[to.Lang]
		if !ok {
			data, count, err := s.BuildReport(ctx, from, until, to.Lang)
			if err != nil {
				log.Printf("monthly report: build failed: %v", err)
				s.audit("report.send", r.Email, month+": build failed", "fail")
				continue
			}
			b = built{data: data, count: count}
			byLang[to.Lang] = b
		}
		if err := s.SendFn(ctx, cfg, monthMessage(to, from, b.count, b.data)); err != nil {
			log.Printf("monthly report to %s failed: %v", r.Email, err)
			if !tried { // only the first failure is audited; retries just log
				s.audit("report.send", r.Email, month+": "+truncate(err.Error(), 200)+" (will retry)", "fail")
			}
			continue
		}
		s.mu.Lock()
		delete(s.lastTry, r.ID)
		s.mu.Unlock()
		if err := s.Admins.MarkReportSent(ctx, r.ID, month); err != nil {
			log.Printf("monthly report: mark sent for %s: %v", r.Email, err)
		}
		s.audit("report.send", r.Email, fmt.Sprintf("%s / %d payments", month, b.count), "ok")
	}
}

// StartScheduler checks periodically whether any scheduled email is due: the
// monthly report, the monthly upcoming-payments list, or the daily payment and
// password-rotation reminders.
func (s *Service) StartScheduler() {
	go func() {
		t := time.NewTicker(checkEvery)
		defer t.Stop()
		for {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			s.RunIfDue(ctx)
			s.RunRemindersIfDue(ctx)
			s.RunRotationRemindersIfDue(ctx)
			s.RunUpcomingIfDue(ctx)
			s.RetryBreachAlerts(ctx)
			cancel()
			<-t.C
		}
	}()
}

func (s *Service) audit(action, target, detail, result string) {
	if s.Audit != nil {
		s.Audit(action, target, detail, result)
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
