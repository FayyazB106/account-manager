package store

import (
	"database/sql"
	"strings"
	"time"
)

// dateParam formats an optional calendar date for a DATE column as YYYY-MM-DD —
// SQLite's canonical form, which its date() functions can operate on. Binding a
// time.Time directly makes modernc write Go's "... +0000 UTC" string, which
// SQLite's date math cannot parse. nil stays NULL.
func dateParam(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.Format("2006-01-02")
}

// tsParam formats a timestamp for a TIMESTAMP column in SQLite's canonical form
// (UTC "YYYY-MM-DD HH:MM:SS"), matching what CURRENT_TIMESTAMP produces.
func tsParam(t time.Time) string {
	return t.UTC().Format("2006-01-02 15:04:05")
}

// parseSQLiteTS parses a timestamp string as returned for an *aggregate* (e.g.
// MAX(...)), which — unlike a typed column — modernc hands back as raw text.
// Returns the zero time for empty/unparseable input.
func parseSQLiteTS(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	for _, layout := range []string{"2006-01-02 15:04:05", time.RFC3339, "2006-01-02T15:04:05Z"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

// affected returns the RowsAffected count, ignoring the (SQLite-never-returns-it)
// error, so call sites can keep the simple `if affected(res) == 0` check.
func affected(res sql.Result) int64 {
	n, _ := res.RowsAffected()
	return n
}

// IsUniqueViolation reports whether err is a SQLite UNIQUE-constraint failure
// (e.g. a duplicate category name), so handlers can return a friendly 409.
func IsUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}
