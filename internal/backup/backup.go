// Package backup manages scheduled, retained full-database backups written to
// disk as gzipped SQLite snapshots. Attachments live in the DB, so a DB snapshot
// includes them.
//
// A backup is produced with SQLite's `VACUUM INTO` (a consistent, self-contained
// copy of the whole database), then gzipped. Restore attaches the snapshot and
// copies every table back into the live database in one transaction, so the
// running server keeps its open connection (no restart required).
package backup

import (
	"compress/gzip"
	"context"
	"database/sql"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/FayyazB106/account-manager/internal/store"
)

// fileName matches the backup files we create, guarding against path traversal.
var fileName = regexp.MustCompile(`^backup-\d{8}-\d{6}\.db\.gz$`)

// tables lists every table restore copies back, in FK-dependency order (parents
// first). Restore inserts in this order and deletes in reverse. Children must
// come after their parent (e.g. password_history/attachment after account,
// admin_backup_code after admin), or the FK insert fails.
//
// Deliberately excluded from the copy: refresh_token (sessions) and audit_log (a
// forward-only operational log; we keep the live trail, incl. the restore action
// itself, rather than reverting it).
var tables = []string{
	"category",
	"account",
	"account_user",
	"attachment",
	"payment",
	"password_history",
	"admin",
	"admin_backup_code",
	"person",
	"app_setting",
	"currency",
}

// clearOnRestore are session tables wiped (not copied) on restore, so a restore
// actually invalidates every session — the backup's own tokens would reference a
// now-replaced admin set anyway. Restoring to recover from a compromise must not
// leave old sessions valid.
var clearOnRestore = []string{"refresh_token"}

type Manager struct {
	dir      string
	db       *sql.DB
	settings *store.SettingStore
	audit    store.AuditFunc
}

func NewManager(dir string, db *sql.DB, settings *store.SettingStore) (*Manager, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return &Manager{dir: dir, db: db, settings: settings}, nil
}

// SetAuditLogger wires system-event logging for scheduled backups.
func (m *Manager) SetAuditLogger(fn store.AuditFunc) { m.audit = fn }

// Info describes a stored backup file.
type Info struct {
	Name      string    `json:"name"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"created_at"`
}

// Create writes a new gzipped SQLite snapshot, then applies retention. Returns
// the new file name.
func (m *Manager) Create(ctx context.Context) (string, error) {
	name := "backup-" + time.Now().Format("20060102-150405") + ".db.gz"
	finalPath := filepath.Join(m.dir, name)

	// VACUUM INTO can't overwrite an existing file, so snapshot to a temp path.
	tmpPath := filepath.Join(m.dir, "."+name+".tmp")
	_ = os.Remove(tmpPath)
	if _, err := m.db.ExecContext(ctx, `VACUUM INTO ?`, tmpPath); err != nil {
		return "", fmt.Errorf("snapshot: %w", err)
	}
	defer os.Remove(tmpPath)

	if err := gzipFile(tmpPath, finalPath); err != nil {
		os.Remove(finalPath)
		return "", err
	}
	m.applyRetention(ctx)
	return name, nil
}

// List returns stored backups, newest first.
func (m *Manager) List() ([]Info, error) {
	entries, err := os.ReadDir(m.dir)
	if err != nil {
		return nil, err
	}
	var out []Info
	for _, e := range entries {
		if e.IsDir() || !fileName.MatchString(e.Name()) {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, Info{Name: e.Name(), Size: fi.Size(), CreatedAt: fi.ModTime()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name > out[j].Name })
	return out, nil
}

// Path returns the absolute path of a named backup, or an error if the name is
// invalid or the file is missing.
func (m *Manager) Path(name string) (string, error) {
	if !fileName.MatchString(name) {
		return "", fmt.Errorf("invalid backup name")
	}
	path := filepath.Join(m.dir, name)
	if _, err := os.Stat(path); err != nil {
		return "", err
	}
	return path, nil
}

// Delete removes a named backup file.
func (m *Manager) Delete(name string) error {
	path, err := m.Path(name)
	if err != nil {
		return err
	}
	return os.Remove(path)
}

// Restore replaces all data from a named backup file. It attaches the snapshot
// and copies every table into the live DB in one transaction (with foreign keys
// off during the swap), so the server's open connection stays valid.
func (m *Manager) Restore(ctx context.Context, name string) error {
	path, err := m.Path(name)
	if err != nil {
		return err
	}

	// Decompress to a temp .db the SQLite engine can attach.
	tmpPath := filepath.Join(m.dir, ".restore-"+strconv.FormatInt(time.Now().UnixNano(), 10)+".db")
	if err := gunzipFile(path, tmpPath); err != nil {
		return err
	}
	defer os.Remove(tmpPath)

	// Use one dedicated connection so ATTACH and the pragmas apply consistently.
	conn, err := m.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, `PRAGMA foreign_keys = OFF`); err != nil {
		return err
	}
	// Re-enable FK enforcement on this connection regardless of outcome.
	defer conn.ExecContext(ctx, `PRAGMA foreign_keys = ON`)

	if _, err := conn.ExecContext(ctx, `ATTACH DATABASE ? AS bak`, tmpPath); err != nil {
		return fmt.Errorf("attach backup: %w", err)
	}
	defer conn.ExecContext(ctx, `DETACH DATABASE bak`)

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Clear existing rows children-first.
	for i := len(tables) - 1; i >= 0; i-- {
		if _, err := tx.ExecContext(ctx, `DELETE FROM main.`+tables[i]); err != nil {
			return fmt.Errorf("clear %s: %w", tables[i], err)
		}
	}
	// Copy backup rows parents-first. Copy only the columns present in BOTH the
	// backup and the current schema (by name, not position), so a backup made
	// under an older schema still restores — columns a later migration added get
	// their defaults instead of a `SELECT *` count/position mismatch.
	for _, t := range tables {
		mainCols, err := tableColumns(ctx, tx, "main", t)
		if err != nil {
			return fmt.Errorf("columns main.%s: %w", t, err)
		}
		bakCols, err := tableColumns(ctx, tx, "bak", t)
		if err != nil {
			return fmt.Errorf("columns bak.%s: %w", t, err)
		}
		cols := intersectCols(mainCols, bakCols)
		if len(cols) == 0 {
			continue
		}
		list := quoteCols(cols)
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO main.`+t+` (`+list+`) SELECT `+list+` FROM bak.`+t); err != nil {
			return fmt.Errorf("restore %s: %w", t, err)
		}
	}
	// Invalidate all sessions (wipe, don't copy) so no old token survives a restore.
	for _, t := range clearOnRestore {
		if _, err := tx.ExecContext(ctx, `DELETE FROM main.`+t); err != nil {
			return fmt.Errorf("clear %s: %w", t, err)
		}
	}
	return tx.Commit()
}

// applyRetention deletes the oldest backups beyond the configured keep count.
func (m *Manager) applyRetention(ctx context.Context) {
	keep := m.keepCount(ctx)
	list, err := m.List()
	if err != nil || len(list) <= keep {
		return
	}
	for _, old := range list[keep:] {
		if err := m.Delete(old.Name); err != nil {
			log.Printf("backup retention: %v", err)
		}
	}
}

func (m *Manager) keepCount(ctx context.Context) int {
	all, _ := m.settings.All(ctx)
	if n, err := strconv.Atoi(strings.TrimSpace(all["backup_keep"])); err == nil && n > 0 {
		return n
	}
	return 7
}

// StartScheduler runs due backups based on the stored schedule settings. It
// checks once a minute; settings changes take effect on the next check.
func (m *Manager) StartScheduler() {
	go func() {
		t := time.NewTicker(time.Minute)
		defer t.Stop()
		for {
			m.runIfDue()
			<-t.C
		}
	}()
}

func (m *Manager) runIfDue() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	all, err := m.settings.All(ctx)
	if err != nil {
		return
	}
	freq := strings.TrimSpace(all["backup_frequency"]) // "off" | "daily" | "weekly"
	if freq != "daily" && freq != "weekly" {
		return
	}
	hh, mm, ok := parseHM(all["backup_time"])
	if !ok {
		return
	}
	now := time.Now()
	sched := time.Date(now.Year(), now.Month(), now.Day(), hh, mm, 0, 0, now.Location())
	if now.Before(sched) {
		return
	}
	last := m.lastBackupTime()
	due := false
	switch freq {
	case "daily":
		due = last.Before(sched)
	case "weekly":
		// Run on the chosen weekday (0=Sunday), once, after the scheduled time.
		wd, _ := strconv.Atoi(strings.TrimSpace(all["backup_weekday"]))
		due = int(now.Weekday()) == wd && last.Before(sched)
	}
	if !due {
		return
	}
	if name, err := m.Create(ctx); err != nil {
		log.Printf("scheduled backup failed: %v", err)
		if m.audit != nil {
			m.audit("backup.create", "", "scheduled", "fail")
		}
	} else {
		log.Printf("scheduled backup created: %s", name)
		if m.audit != nil {
			m.audit("backup.create", name, "scheduled", "ok")
		}
	}
}

func (m *Manager) lastBackupTime() time.Time {
	list, err := m.List()
	if err != nil || len(list) == 0 {
		return time.Time{}
	}
	return list[0].CreatedAt
}

func parseHM(s string) (int, int, bool) {
	parts := strings.SplitN(strings.TrimSpace(s), ":", 2)
	if len(parts) != 2 {
		return 0, 0, false
	}
	h, err1 := strconv.Atoi(parts[0])
	mi, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil || h < 0 || h > 23 || mi < 0 || mi > 59 {
		return 0, 0, false
	}
	return h, mi, true
}

// gzipFile gzip-compresses src into dst.
func gzipFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	gz := gzip.NewWriter(out)
	_, copyErr := io.Copy(gz, in)
	gzErr := gz.Close()
	outErr := out.Close()
	return firstErr(copyErr, gzErr, outErr)
}

// gunzipFile decompresses a gzipped src into dst.
func gunzipFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	gz, err := gzip.NewReader(in)
	if err != nil {
		return err
	}
	defer gz.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, gz)
	outErr := out.Close()
	return firstErr(copyErr, outErr)
}

// tableColumns returns the column names of schema.table (e.g. "main"/"bak").
func tableColumns(ctx context.Context, tx *sql.Tx, schema, table string) ([]string, error) {
	rows, err := tx.QueryContext(ctx, `PRAGMA `+schema+`.table_info("`+table+`")`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var cols []string
	for rows.Next() {
		var cid, notnull, pk int
		var name, typ string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			return nil, err
		}
		cols = append(cols, name)
	}
	return cols, rows.Err()
}

// intersectCols returns the columns in `a` that also appear in `b`, preserving a's order.
func intersectCols(a, b []string) []string {
	set := make(map[string]bool, len(b))
	for _, c := range b {
		set[c] = true
	}
	var out []string
	for _, c := range a {
		if set[c] {
			out = append(out, c)
		}
	}
	return out
}

// quoteCols renders a column list as `"a", "b"` for SQL.
func quoteCols(cols []string) string {
	q := make([]string, len(cols))
	for i, c := range cols {
		q[i] = `"` + c + `"`
	}
	return strings.Join(q, ", ")
}

func firstErr(errs ...error) error {
	for _, e := range errs {
		if e != nil {
			return e
		}
	}
	return nil
}
