package api

import (
	"errors"
	"net/http"

	"github.com/FayyazB106/account-manager/internal/store"
)

func (s *Server) listCurrencies(w http.ResponseWriter, r *http.Request) {
	cs, err := s.Currencies.List(r.Context())
	if err != nil {
		logErr("listCurrencies", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if cs == nil {
		cs = []store.Currency{}
	}
	writeJSON(w, http.StatusOK, cs)
}

// setCurrency handles PATCH /currencies/{code} with body {"active": bool}.
func (s *Server) setCurrency(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	if code == "USD" {
		writeError(w, http.StatusBadRequest, "USD is the base currency and can't be changed")
		return
	}
	var body struct {
		Active bool `json:"active"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	// A currency that accounts are priced in can't be switched off, or their amounts
	// would lose their conversion columns and picker entry.
	if !body.Active {
		used, err := s.Accounts.ServicesUsingCurrency(r.Context(), code)
		if err != nil {
			logErr("setCurrency: check usage", err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if len(used) > 0 {
			writeError(w, http.StatusConflict, "currency is still used by accounts; change their currency first")
			return
		}
	}
	if err := s.Currencies.SetActive(r.Context(), code, body.Active); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "currency not found")
			return
		}
		logErr("setCurrency", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"active": body.Active})
}
