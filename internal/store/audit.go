package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// AuditEntry is one row of the append-only audit log.
type AuditEntry struct {
	ID         string    `json:"id"`
	At         time.Time `json:"at"`
	ActorEmail string    `json:"actor_email"`
	Action     string    `json:"action"`
	Target     string    `json:"target"`
	Detail     string    `json:"detail"`
	IP         string    `json:"ip"`
	Result     string    `json:"result"`
}

// AuditFilter narrows a listing; empty fields are ignored.
type AuditFilter struct {
	Actor  string // matched as a substring of actor_email
	Action string // matched as an action prefix (e.g. "account")
	Limit  int
	Offset int
}

type AuditStore struct {
	db *sql.DB
}

func NewAuditStore(db *sql.DB) *AuditStore {
	return &AuditStore{db: db}
}

// AuditFunc records a system-originated audit event. Background jobs (backups,
// breach scans, rate updates, startup) take one so their activity shows up in
// the in-app log alongside user actions.
type AuditFunc func(action, target, detail, result string)

// SysLogger returns an AuditFunc that records events as the "system" actor.
// Failures to insert are ignored (a dropped log line must never break a job).
func (s *AuditStore) SysLogger() AuditFunc {
	return func(action, target, detail, result string) {
		_ = s.Insert(context.Background(), AuditEntry{
			ActorEmail: "system",
			Action:     action,
			Target:     target,
			Detail:     detail,
			IP:         "local",
			Result:     result,
		})
	}
}

// Insert appends an entry. `at` defaults to now.
func (s *AuditStore) Insert(ctx context.Context, e AuditEntry) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO audit_log (actor_email, action, target, detail, ip, result)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		e.ActorEmail, e.Action, e.Target, e.Detail, e.IP, e.Result)
	return err
}

// List returns a page of entries (newest first) plus the total count matching
// the filter (for pagination).
func (s *AuditStore) List(ctx context.Context, f AuditFilter) ([]AuditEntry, int, error) {
	where := "WHERE 1=1"
	var args []any
	if f.Actor != "" {
		args = append(args, "%"+f.Actor+"%")
		where += fmt.Sprintf(" AND actor_email LIKE $%d", len(args))
	}
	if f.Action != "" {
		args = append(args, f.Action+"%")
		where += fmt.Sprintf(" AND action LIKE $%d", len(args))
	}

	var total int
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM audit_log "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	pageArgs := append(append([]any{}, args...), limit, f.Offset)
	q := "SELECT id, at, actor_email, action, target, detail, ip, result FROM audit_log " + where +
		fmt.Sprintf(" ORDER BY at DESC, id DESC LIMIT $%d OFFSET $%d", len(args)+1, len(args)+2)
	rows, err := s.db.QueryContext(ctx, q, pageArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []AuditEntry
	for rows.Next() {
		var e AuditEntry
		if err := rows.Scan(&e.ID, &e.At, &e.ActorEmail, &e.Action, &e.Target, &e.Detail, &e.IP, &e.Result); err != nil {
			return nil, 0, err
		}
		out = append(out, e)
	}
	return out, total, rows.Err()
}
