package api

import (
	"net/http"
	"strconv"

	"github.com/FayyazB106/account-manager/internal/store"
)

// listAudit handles GET /audit?actor=&action=&limit=&offset= — a read-only,
// paginated view of the audit log (admins only, behind requireAuth).
func (s *Server) listAudit(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := store.AuditFilter{
		Actor:  q.Get("actor"),
		Action: q.Get("action"),
		Limit:  atoiDefault(q.Get("limit"), 100),
		Offset: atoiDefault(q.Get("offset"), 0),
	}
	entries, total, err := s.Audit.List(r.Context(), f)
	if err != nil {
		logErr("listAudit", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if entries == nil {
		entries = []store.AuditEntry{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": entries, "total": total})
}

func atoiDefault(s string, def int) int {
	if n, err := strconv.Atoi(s); err == nil && n >= 0 {
		return n
	}
	return def
}
