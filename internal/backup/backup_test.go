package backup

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/FayyazB106/account-manager/internal/db"
	"github.com/FayyazB106/account-manager/internal/store"
)

// notCopied are tables intentionally NOT copied back on restore: the forward-only
// audit log (kept as-is). Session tables are handled by clearOnRestore (wiped).
var notCopied = map[string]bool{
	"audit_log": true,
}

// TestRestoreCoversEverySchemaTable guards against the subtle data-loss bug where
// a table exists in the schema (so a VACUUM INTO backup contains it) but is left
// out of the restore `tables` list — meaning restore silently drops it (and
// cascading deletes can empty it). Every real table must be either restored or
// explicitly excluded.
func TestRestoreCoversEverySchemaTable(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer database.Close()

	rows, err := database.Query(
		`SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'`)
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}
	defer rows.Close()

	covered := map[string]bool{}
	for _, t := range tables {
		covered[t] = true
	}
	for _, t := range clearOnRestore {
		covered[t] = true
	}

	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		if !covered[name] && !notCopied[name] {
			t.Errorf("table %q is in the schema but is neither restored, cleared, nor "+
				"in notCopied — restore would silently drop it. Add it to `tables` (in "+
				"FK order), clearOnRestore, or notCopied.", name)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
}

// A schedule where only the frequency was chosen (the time was never edited, so it was
// never saved) must still run, at the 02:00 the settings page shows.
func TestScheduledBackupRunsWithTheDefaultTime(t *testing.T) {
	d, err := db.Open(filepath.Join(t.TempDir(), "b.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	settings := store.NewSettingStore(d)
	m, err := NewManager(filepath.Join(t.TempDir(), "backups"), d, settings)
	if err != nil {
		t.Fatal(err)
	}
	if err := settings.Set(context.Background(), "backup_frequency", "daily"); err != nil {
		t.Fatal(err)
	}

	day := time.Date(2026, 11, 3, 0, 0, 0, 0, time.Local)
	m.now = func() time.Time { return day.Add(1*time.Hour + 59*time.Minute) } // 01:59, before 02:00
	m.runIfDue()
	if list, _ := m.List(); len(list) != 0 {
		t.Fatalf("a backup ran before the default 02:00 (%d)", len(list))
	}
	m.now = func() time.Time { return day.Add(2 * time.Hour) } // 02:00
	m.runIfDue()
	if list, _ := m.List(); len(list) != 1 {
		t.Fatalf("expected the scheduled backup at the default time, got %d", len(list))
	}
}
