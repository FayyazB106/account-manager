package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/FayyazB106/account-manager/internal/crypto"
	"github.com/FayyazB106/account-manager/internal/store"
	"github.com/FayyazB106/account-manager/internal/totp"
)

// setAccountTOTP (POST /accounts/{id}/totp) stores a 2FA secret for a saved
// service. Accepts either a base32 secret or a full otpauth:// URI.
func (s *Server) setAccountTOTP(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		Secret string `json:"secret"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	secret, err := totp.ParseSecret(body.Secret)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid TOTP secret")
		return
	}
	enc, err := crypto.Encrypt(s.MasterKeyValue(), []byte(secret))
	if err != nil {
		logErr("setAccountTOTP: encrypt", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if err := s.Accounts.SetTOTP(r.Context(), id, enc); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "account not found")
			return
		}
		logErr("setAccountTOTP: store", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// clearAccountTOTP (DELETE /accounts/{id}/totp) removes a saved 2FA secret.
func (s *Server) clearAccountTOTP(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.Accounts.ClearTOTP(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "account not found")
			return
		}
		logErr("clearAccountTOTP", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// allAccountTOTPCodes (GET /accounts/totp/codes) returns the current code +
// seconds remaining for every account that has a stored 2FA secret, keyed by
// account id — for the authenticator list. Polled, so not audited.
func (s *Server) allAccountTOTPCodes(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	list, err := s.Accounts.AllTOTPSecrets(r.Context())
	if err != nil {
		logErr("allAccountTOTPCodes", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := make(map[string]map[string]any, len(list))
	for _, a := range list {
		secret, derr := crypto.Decrypt(s.MasterKeyValue(), a.Secret)
		if derr != nil {
			continue
		}
		code, remaining, cerr := totp.Code(string(secret), time.Now())
		if cerr != nil {
			continue
		}
		out[a.ID] = map[string]any{"code": code, "expires_in": remaining}
	}
	writeJSON(w, http.StatusOK, out)
}

// accountTOTPCode (GET /accounts/{id}/totp/code) returns the current 6-digit code
// for a saved account plus the seconds left in its window (for a countdown).
func (s *Server) accountTOTPCode(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	id := r.PathValue("id")
	enc, err := s.Accounts.GetTOTP(r.Context(), id)
	if errors.Is(err, store.ErrNoTOTP) {
		writeError(w, http.StatusNotFound, "no TOTP for this account")
		return
	}
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "account not found")
		return
	}
	if err != nil {
		logErr("accountTOTPCode: get", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	secret, err := crypto.Decrypt(s.MasterKeyValue(), enc)
	if err != nil {
		logErr("accountTOTPCode: decrypt", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	code, remaining, err := totp.Code(string(secret), time.Now())
	if err != nil {
		logErr("accountTOTPCode: code", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": code, "expires_in": remaining})
}
