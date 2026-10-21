package breach

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/FayyazB106/account-manager/internal/crypto"
	"github.com/FayyazB106/account-manager/internal/store"
)

// Monitor periodically checks stored passwords against HIBP and records each
// account's breach count. It decrypts passwords with the master key (the server
// can already do this in the current envelope model), SHA-1s them, and queries
// the k-anonymity range API — so no password ever leaves the server.
type Monitor struct {
	accounts *store.AccountStore
	settings *store.SettingStore
	keyFn    func() []byte // master key, read lazily (may be set after startup via web unlock)
	client   *http.Client
	audit    store.AuditFunc
	// OnNew is called after a scan with the accounts whose password was found in a
	// breach that was not known before (nil = nobody to tell).
	OnNew func(ctx context.Context, hits []store.BreachHit)
}

func NewMonitor(accounts *store.AccountStore, settings *store.SettingStore, keyFn func() []byte) *Monitor {
	return &Monitor{
		accounts: accounts,
		settings: settings,
		keyFn:    keyFn,
		client:   &http.Client{Timeout: 15 * time.Second},
	}
}

// SetAuditLogger wires system-event logging for scheduled breach scans.
func (m *Monitor) SetAuditLogger(fn store.AuditFunc) { m.audit = fn }

// Scan checks every active account's password and stores the breach count.
// Returns how many were checked and how many were skipped (a decrypt or HIBP
// error on an individual account is skipped and retried next run rather than
// aborting the whole scan). A high skip count usually means the server can't
// reach HIBP.
func (m *Monitor) Scan(ctx context.Context) (checked, skipped int, err error) {
	key := m.keyFn()
	if len(key) == 0 {
		return 0, 0, nil // vault locked / not yet initialized — nothing to scan
	}
	secrets, err := m.accounts.ListActiveSecrets(ctx)
	if err != nil {
		return 0, 0, err
	}
	var fresh []store.BreachHit
	for _, sec := range secrets {
		pt, err := crypto.EnvelopeDecrypt(key, &crypto.Envelope{
			WrappedDEK: sec.PasswordWrappedDEK,
			Ciphertext: sec.PasswordCiphertext,
		})
		if err != nil {
			skipped++
			continue
		}
		n, err := Check(ctx, m.client, string(pt))
		if err != nil {
			skipped++
			continue
		}
		if err := m.accounts.SetBreach(ctx, sec.ID, n); err != nil {
			skipped++
			continue
		}
		// New = the password now appears in more breaches than the last time it was
		// checked (a never-checked or since-changed password counts from zero).
		prev := 0
		if sec.PrevBreach != nil {
			prev = *sec.PrevBreach
		}
		if n > prev {
			fresh = append(fresh, store.BreachHit{ID: sec.ID, Service: sec.Service, Login: sec.Login, Count: n})
		}
		checked++
		time.Sleep(200 * time.Millisecond) // be polite to HIBP
	}
	if len(fresh) > 0 && m.OnNew != nil {
		m.OnNew(ctx, fresh)
	}
	return checked, skipped, nil
}

// StartScheduler runs the scan on the stored cadence. It checks every 30 minutes
// whether a scan is due; settings changes take effect on the next check.
func (m *Monitor) StartScheduler() {
	go func() {
		t := time.NewTicker(30 * time.Minute)
		defer t.Stop()
		for {
			m.runIfDue()
			<-t.C
		}
	}()
}

func (m *Monitor) runIfDue() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	all, err := m.settings.All(ctx)
	if err != nil {
		return
	}
	var interval time.Duration
	switch strings.TrimSpace(all["breach_monitor"]) { // off | daily | weekly
	case "daily":
		interval = 24 * time.Hour
	case "weekly":
		interval = 7 * 24 * time.Hour
	default:
		return
	}
	last, err := m.accounts.LastBreachCheck(ctx)
	if err != nil {
		return
	}
	if !last.IsZero() && time.Since(last) < interval {
		return
	}
	if n, skipped, err := m.Scan(ctx); err != nil {
		log.Printf("breach scan failed: %v", err)
		if m.audit != nil {
			m.audit("breach.scan", "", "scheduled", "fail")
		}
	} else {
		log.Printf("breach scan checked %d accounts, skipped %d", n, skipped)
		if m.audit != nil {
			detail := fmt.Sprintf("scheduled / %d checked / %d skipped", n, skipped)
			result := "ok"
			if n == 0 && skipped > 0 {
				result = "fail" // couldn't reach HIBP for any account
			}
			m.audit("breach.scan", "", detail, result)
		}
	}
}
