package report

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/FayyazB106/account-manager/internal/store"
)

var sampleHits = []store.BreachHit{
	{ID: "1", Service: "Figma", Login: "ops@example.com", Count: 1204},
	{ID: "2", Service: "Zoom", Count: 1},
}

func TestBreachMessageEnglishAndArabic(t *testing.T) {
	en := breachMessage(Recipient{Name: "Alice", Email: "a@example.com", Lang: LangEN}, sampleHits, true)
	for _, want := range []string{"Hi Alice,", "just found", "Figma (ops@example.com): exposed 1204 times", "Zoom: exposed 1 time", "Change them as soon as possible"} {
		if !strings.Contains(en.Body, want) {
			t.Errorf("english body missing %q:\n%s", want, en.Body)
		}
	}
	if en.Subject != "Account Manager: 2 passwords found in a data breach" || en.RTL {
		t.Errorf("english subject/RTL: %q %v", en.Subject, en.RTL)
	}
	if strings.Contains(en.Body, "currently flagged") {
		t.Error("the automatic alert must not use the Send Now wording")
	}
	now := breachMessage(Recipient{Name: "Alice", Email: "a@example.com", Lang: LangEN}, sampleHits, false)
	if !strings.Contains(now.Body, "currently flagged") {
		t.Error("Send Now should say the passwords are currently flagged")
	}

	ar := breachMessage(Recipient{Name: "فياض", Email: "a@example.com", Lang: LangAR}, sampleHits, true)
	if !ar.RTL || !strings.HasPrefix(ar.Subject, "مدير الحسابات: كلمات مرور ظهرت في تسريب بيانات (2)") {
		t.Errorf("arabic subject/RTL: %q %v", ar.Subject, ar.RTL)
	}
	if !strings.Contains(ar.Body, "ظهرت 1204 مرة") || !strings.Contains(ar.Body, "ظهرت مرة واحدة") {
		t.Errorf("arabic counts missing:\n%s", ar.Body)
	}
}

// Only admins who opted in are emailed, each in their own language; nothing is sent
// when email is off, the vault is locked, or there is nothing new.
func TestNotifyBreachesGoesToOptedInAdminsOnly(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()
	hits := realHits(t, fx)

	fx.svc.NotifyBreaches(ctx, hits)
	if len(fx.sent) != 0 {
		t.Fatal("sent with nobody opted in")
	}

	_ = fx.svc.Admins.SetBreachAlerts(ctx, fx.adminB, true)
	fx.svc.NotifyBreaches(ctx, nil)
	if len(fx.sent) != 0 {
		t.Fatal("sent with nothing new")
	}
	fx.locked = true
	fx.svc.NotifyBreaches(ctx, hits)
	if len(fx.sent) != 0 {
		t.Fatal("sent while the vault was locked")
	}
	fx.locked = false

	fx.svc.NotifyBreaches(ctx, hits)
	if len(fx.sent) != 1 || fx.sent[0].To[0] != "b@example.com" || !strings.Contains(fx.sent[0].Body, "Hi Bob,") {
		t.Fatalf("expected one alert to Bob, got %d: %v", len(fx.sent), fx.sent)
	}
}

func TestSendBreachPreviewListsWhatIsFlagged(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()
	to := Recipient{Name: "Alice", Email: "a@example.com", Lang: LangEN}

	if n, err := fx.svc.SendBreachPreview(ctx, to); err != nil || n != 0 || len(fx.sent) != 0 {
		t.Fatalf("nothing flagged: n=%d err=%v sent=%d", n, err, len(fx.sent))
	}
	if _, err := fx.db.Exec(`UPDATE account SET breach_count = 7 WHERE service = 'Zoom'`); err != nil {
		t.Fatal(err)
	}
	n, err := fx.svc.SendBreachPreview(ctx, to)
	if err != nil || n != 1 || len(fx.sent) != 1 || !strings.Contains(fx.sent[0].Body, "Zoom: exposed 7 times") {
		t.Fatalf("n=%d err=%v sent=%v", n, err, fx.sent)
	}
}

// An alert that can't be delivered (network/mail server down) is kept and retried until
// it goes through; the first failure is the only one audited, and a retry waits.
func TestBreachAlertIsRetriedUntilDelivered(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()
	_ = fx.svc.Admins.SetBreachAlerts(ctx, fx.adminA, true)
	if _, err := fx.db.Exec(`UPDATE account SET breach_count = 4 WHERE service = 'Zoom'`); err != nil {
		t.Fatal(err)
	}
	var zoom string
	if err := fx.db.QueryRow(`SELECT id FROM account WHERE service = 'Zoom'`).Scan(&zoom); err != nil {
		t.Fatal(err)
	}
	var audits []string
	fx.svc.Audit = func(action, target, detail, result string) { audits = append(audits, action+":"+result) }

	fx.failN = 3 // the network is down for the first three attempts
	fx.svc.NotifyBreaches(ctx, []store.BreachHit{{ID: zoom, Service: "Zoom", Count: 4}})
	if len(fx.sent) != 0 {
		t.Fatal("sent despite the failure")
	}

	fx.svc.RetryBreachAlerts(ctx) // too soon
	if fx.failN != 2 {
		t.Fatalf("retried before the wait was over (failN=%d)", fx.failN)
	}
	fx.now = fx.now.Add(breachRetryEvery + time.Minute)
	fx.svc.RetryBreachAlerts(ctx) // fails again
	fx.now = fx.now.Add(breachRetryEvery + time.Minute)
	fx.svc.RetryBreachAlerts(ctx) // fails again
	fx.now = fx.now.Add(breachRetryEvery + time.Minute)
	fx.svc.RetryBreachAlerts(ctx) // goes through
	if len(fx.sent) != 1 || !strings.Contains(fx.sent[0].Body, "Zoom: exposed 4 times") {
		t.Fatalf("expected the queued alert to be delivered, sent=%d", len(fx.sent))
	}
	if strings.Join(audits, ",") != "breach.alert:fail,breach.alert:ok" {
		t.Fatalf("expected one failure then one success in the audit log, got %v", audits)
	}

	fx.now = fx.now.Add(breachRetryEvery + time.Minute)
	fx.svc.RetryBreachAlerts(ctx)
	if len(fx.sent) != 1 {
		t.Fatal("a delivered alert was sent again")
	}
}

// Queued alerts are dropped when they stop being true or the admin opts out.
func TestQueuedBreachAlertsAreDroppedWhenStale(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()
	_ = fx.svc.Admins.SetBreachAlerts(ctx, fx.adminA, true)
	if _, err := fx.db.Exec(`UPDATE account SET breach_count = 4 WHERE service = 'Zoom'`); err != nil {
		t.Fatal(err)
	}
	var zoom string
	_ = fx.db.QueryRow(`SELECT id FROM account WHERE service = 'Zoom'`).Scan(&zoom)

	fx.failN = 1
	fx.svc.NotifyBreaches(ctx, []store.BreachHit{{ID: zoom, Service: "Zoom", Count: 4}})
	// The password gets changed before the retry (breach flag reset): nothing to send.
	if _, err := fx.db.Exec(`UPDATE account SET breach_count = NULL WHERE id = $1`, zoom); err != nil {
		t.Fatal(err)
	}
	fx.now = fx.now.Add(breachRetryEvery + time.Minute)
	fx.svc.RetryBreachAlerts(ctx)
	if len(fx.sent) != 0 {
		t.Fatal("sent an alert for a password that is no longer flagged")
	}
	var n int
	_ = fx.db.QueryRow(`SELECT COUNT(*) FROM breach_alert_pending`).Scan(&n)
	if n != 0 {
		t.Fatalf("stale queue entry was kept (%d rows)", n)
	}

	// Opting out clears anything still waiting.
	_, _ = fx.db.Exec(`UPDATE account SET breach_count = 4 WHERE id = $1`, zoom)
	fx.failN = 1
	fx.svc.NotifyBreaches(ctx, []store.BreachHit{{ID: zoom, Service: "Zoom", Count: 4}})
	_ = fx.svc.Admins.SetBreachAlerts(ctx, fx.adminA, false)
	_ = fx.db.QueryRow(`SELECT COUNT(*) FROM breach_alert_pending`).Scan(&n)
	if n != 0 {
		t.Fatalf("opting out left %d queued alerts", n)
	}
}

// realHits is sampleHits pointed at real account rows (the retry queue references them).
func realHits(t *testing.T, fx *fixture) []store.BreachHit {
	t.Helper()
	out := append([]store.BreachHit(nil), sampleHits...)
	for i := range out {
		if err := fx.db.QueryRow(`SELECT id FROM account WHERE service = $1`, out[i].Service).Scan(&out[i].ID); err != nil {
			t.Fatal(err)
		}
		if _, err := fx.db.Exec(`UPDATE account SET breach_count = $2 WHERE id = $1`, out[i].ID, out[i].Count); err != nil {
			t.Fatal(err)
		}
	}
	return out
}
