package db

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func columnExists(t *testing.T, d *sql.DB, table, col string) bool {
	t.Helper()
	rows, err := d.Query(`PRAGMA table_info("` + table + `")`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notnull, pk int
		var name, typ string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			t.Fatal(err)
		}
		if name == col {
			return true
		}
	}
	return false
}

func userVersion(t *testing.T, d *sql.DB) int {
	t.Helper()
	var v int
	if err := d.QueryRow("PRAGMA user_version").Scan(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

// A fresh database lands on the latest version with the latest columns.
func TestFreshDatabaseIsLatest(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "a.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if v := userVersion(t, d); v != schemaVersion {
		t.Fatalf("user_version = %d, want %d", v, schemaVersion)
	}
	for _, c := range []string{"monthly_report", "monthly_report_last", "payment_reminders", "payment_reminder_last", "rotation_reminders", "rotation_reminder_last", "upcoming_payments", "upcoming_payments_last", "language", "breach_alerts"} {
		if !columnExists(t, d, "admin", c) {
			t.Errorf("fresh schema is missing admin.%s", c)
		}
	}
}

// An install on the previous schema (v8, before the monthly report columns)
// upgrades in place, keeps its rows, and the new columns default to "off".
func TestUpgradeFromV8AddsReportColumns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	d, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(`INSERT INTO admin (email, name, auth_hash, kek_salt, wrapped_vault_key) VALUES ('a@example.com','A','h',x'01',x'02')`); err != nil {
		t.Fatal(err)
	}
	// Rewind to what a v8 database looked like.
	for _, stmt := range []string{
		`ALTER TABLE admin DROP COLUMN monthly_report`,
		`ALTER TABLE admin DROP COLUMN monthly_report_last`,
		`ALTER TABLE admin DROP COLUMN payment_reminders`,
		`ALTER TABLE admin DROP COLUMN payment_reminder_last`,
		`ALTER TABLE admin DROP COLUMN rotation_reminders`,
		`ALTER TABLE admin DROP COLUMN rotation_reminder_last`,
		`ALTER TABLE admin DROP COLUMN upcoming_payments`,
		`ALTER TABLE admin DROP COLUMN upcoming_payments_last`,
		`ALTER TABLE admin DROP COLUMN language`,
		`ALTER TABLE admin DROP COLUMN breach_alerts`,
		`PRAGMA user_version = 8`,
	} {
		if _, err := d.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	d.Close()

	d, err = Open(path) // runs migration 9
	if err != nil {
		t.Fatalf("upgrade failed: %v", err)
	}
	defer d.Close()
	if v := userVersion(t, d); v != schemaVersion {
		t.Fatalf("user_version after upgrade = %d, want %d", v, schemaVersion)
	}
	var on int
	var last string
	if err := d.QueryRow(`SELECT monthly_report, monthly_report_last FROM admin WHERE email='a@example.com'`).Scan(&on, &last); err != nil {
		t.Fatalf("existing admin lost or new columns missing: %v", err)
	}
	if on != 0 || last != "" {
		t.Errorf("new columns should default to off/empty, got %d/%q", on, last)
	}
}

// An install already on v9 (monthly report columns, no reminders yet) upgrades in
// place: its monthly-report settings are kept and reminders default to "off".
func TestUpgradeFromV9AddsReminderColumns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v9.db")
	d, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(`INSERT INTO admin (email, name, auth_hash, kek_salt, wrapped_vault_key, monthly_report, monthly_report_last)
		VALUES ('a@example.com','A','h',x'01',x'02',1,'2026-10')`); err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		`ALTER TABLE admin DROP COLUMN payment_reminders`,
		`ALTER TABLE admin DROP COLUMN payment_reminder_last`,
		`ALTER TABLE admin DROP COLUMN rotation_reminders`,
		`ALTER TABLE admin DROP COLUMN rotation_reminder_last`,
		`ALTER TABLE admin DROP COLUMN upcoming_payments`,
		`ALTER TABLE admin DROP COLUMN upcoming_payments_last`,
		`ALTER TABLE admin DROP COLUMN language`,
		`ALTER TABLE admin DROP COLUMN breach_alerts`,
		`PRAGMA user_version = 9`,
	} {
		if _, err := d.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	d.Close()

	d, err = Open(path) // runs migration 10
	if err != nil {
		t.Fatalf("upgrade failed: %v", err)
	}
	defer d.Close()
	if v := userVersion(t, d); v != schemaVersion {
		t.Fatalf("user_version after upgrade = %d, want %d", v, schemaVersion)
	}
	var monthly, reminders int
	var monthlyLast, reminderLast string
	if err := d.QueryRow(`SELECT monthly_report, monthly_report_last, payment_reminders, payment_reminder_last FROM admin`).
		Scan(&monthly, &monthlyLast, &reminders, &reminderLast); err != nil {
		t.Fatal(err)
	}
	if monthly != 1 || monthlyLast != "2026-10" {
		t.Errorf("existing monthly-report settings were lost: %d/%q", monthly, monthlyLast)
	}
	if reminders != 0 || reminderLast != "" {
		t.Errorf("reminders should default to off/empty, got %d/%q", reminders, reminderLast)
	}
}

// An install already on v10 (payment reminders set up, no rotation reminders yet)
// upgrades in place: its reminder settings are kept and rotation reminders start off.
func TestUpgradeFromV10AddsRotationColumns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v10.db")
	d, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(`INSERT INTO admin (email, name, auth_hash, kek_salt, wrapped_vault_key, payment_reminders, payment_reminder_last)
		VALUES ('a@example.com','A','h',x'01',x'02',1,'2026-11-01')`); err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		`ALTER TABLE admin DROP COLUMN rotation_reminders`,
		`ALTER TABLE admin DROP COLUMN rotation_reminder_last`,
		`ALTER TABLE admin DROP COLUMN upcoming_payments`,
		`ALTER TABLE admin DROP COLUMN upcoming_payments_last`,
		`ALTER TABLE admin DROP COLUMN language`,
		`ALTER TABLE admin DROP COLUMN breach_alerts`,
		`PRAGMA user_version = 10`,
	} {
		if _, err := d.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	d.Close()

	d, err = Open(path) // runs migration 11
	if err != nil {
		t.Fatalf("upgrade failed: %v", err)
	}
	defer d.Close()
	if v := userVersion(t, d); v != schemaVersion {
		t.Fatalf("user_version after upgrade = %d, want %d", v, schemaVersion)
	}
	var pay, rot int
	var payLast, rotLast string
	if err := d.QueryRow(`SELECT payment_reminders, payment_reminder_last, rotation_reminders, rotation_reminder_last FROM admin`).
		Scan(&pay, &payLast, &rot, &rotLast); err != nil {
		t.Fatal(err)
	}
	if pay != 1 || payLast != "2026-11-01" {
		t.Errorf("existing payment-reminder settings were lost: %d/%q", pay, payLast)
	}
	if rot != 0 || rotLast != "" {
		t.Errorf("rotation reminders should default to off/empty, got %d/%q", rot, rotLast)
	}
}

// An install already on v11 (rotation reminders set up, no upcoming-payments email
// yet) upgrades in place: its settings are kept and the new email starts off.
func TestUpgradeFromV11AddsUpcomingColumns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v11.db")
	d, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(`INSERT INTO admin (email, name, auth_hash, kek_salt, wrapped_vault_key, rotation_reminders, rotation_reminder_last)
		VALUES ('a@example.com','A','h',x'01',x'02',1,'2026-11-01')`); err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		`ALTER TABLE admin DROP COLUMN upcoming_payments`,
		`ALTER TABLE admin DROP COLUMN upcoming_payments_last`,
		`ALTER TABLE admin DROP COLUMN language`,
		`ALTER TABLE admin DROP COLUMN breach_alerts`,
		`PRAGMA user_version = 11`,
	} {
		if _, err := d.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	d.Close()

	d, err = Open(path) // runs migration 12
	if err != nil {
		t.Fatalf("upgrade failed: %v", err)
	}
	defer d.Close()
	if v := userVersion(t, d); v != schemaVersion {
		t.Fatalf("user_version after upgrade = %d, want %d", v, schemaVersion)
	}
	var rot, up int
	var rotLast, upLast string
	if err := d.QueryRow(`SELECT rotation_reminders, rotation_reminder_last, upcoming_payments, upcoming_payments_last FROM admin`).
		Scan(&rot, &rotLast, &up, &upLast); err != nil {
		t.Fatal(err)
	}
	if rot != 1 || rotLast != "2026-11-01" {
		t.Errorf("existing rotation-reminder settings were lost: %d/%q", rot, rotLast)
	}
	if up != 0 || upLast != "" {
		t.Errorf("the upcoming-payments email should default to off/empty, got %d/%q", up, upLast)
	}
}

// An install already on v12 (all the email settings, no language yet) upgrades in
// place: existing admins keep their settings and default to English.
func TestUpgradeFromV12AddsLanguageColumn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v12.db")
	d, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(`INSERT INTO admin (email, name, auth_hash, kek_salt, wrapped_vault_key, upcoming_payments, upcoming_payments_last)
		VALUES ('a@example.com','A','h',x'01',x'02',1,'2026-11')`); err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{`ALTER TABLE admin DROP COLUMN language`, `ALTER TABLE admin DROP COLUMN breach_alerts`, `PRAGMA user_version = 12`} {
		if _, err := d.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	d.Close()

	d, err = Open(path) // runs migration 13
	if err != nil {
		t.Fatalf("upgrade failed: %v", err)
	}
	defer d.Close()
	if v := userVersion(t, d); v != schemaVersion {
		t.Fatalf("user_version after upgrade = %d, want %d", v, schemaVersion)
	}
	var up int
	var upLast, lang string
	if err := d.QueryRow(`SELECT upcoming_payments, upcoming_payments_last, language FROM admin`).Scan(&up, &upLast, &lang); err != nil {
		t.Fatal(err)
	}
	if up != 1 || upLast != "2026-11" {
		t.Errorf("existing settings were lost: %d/%q", up, upLast)
	}
	if lang != "en" {
		t.Errorf("language should default to en, got %q", lang)
	}
}

// An install already on v13 (language present, no breach alerts yet) upgrades in
// place; the new switch defaults to off for every existing admin.
func TestUpgradeFromV13AddsBreachAlertsColumn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v13.db")
	d, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(`INSERT INTO admin (email, name, auth_hash, kek_salt, wrapped_vault_key) VALUES ('a@example.com','A','h',x'00',x'00')`); err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{`ALTER TABLE admin DROP COLUMN breach_alerts`, `PRAGMA user_version = 13`} {
		if _, err := d.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	d.Close()

	d, err = Open(path)
	if err != nil {
		t.Fatalf("upgrade from v13 failed: %v", err)
	}
	defer d.Close()
	var on int
	if err := d.QueryRow(`SELECT breach_alerts FROM admin`).Scan(&on); err != nil {
		t.Fatal(err)
	}
	if on != 0 {
		t.Errorf("breach_alerts should default to off, got %d", on)
	}
}
