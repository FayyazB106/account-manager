package api

import "net/http"

// breachStatus (GET /breach) returns the last scan time and how many accounts
// are currently flagged as breached — for the Configuration UI.
func (s *Server) breachStatus(w http.ResponseWriter, r *http.Request) {
	last, breached, err := s.Accounts.BreachSummary(r.Context())
	if err != nil {
		logErr("breachStatus", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := map[string]any{"breached": breached, "last_check": ""}
	if !last.IsZero() {
		out["last_check"] = last.UTC()
	}
	writeJSON(w, http.StatusOK, out)
}

// scanBreaches (POST /breach/scan) runs a breach check immediately and returns
// how many accounts were checked.
func (s *Server) scanBreaches(w http.ResponseWriter, r *http.Request) {
	n, skipped, err := s.Breach.Scan(r.Context())
	if err != nil {
		logErr("scanBreaches", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"checked": n, "skipped": skipped})
}
