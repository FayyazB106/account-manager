package backup

import (
	"path/filepath"
	"testing"

	"github.com/FayyazB106/account-manager/internal/db"
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
