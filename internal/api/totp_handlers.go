package api

import (
	"net/http"
	"time"

	"github.com/FayyazB106/account-manager/internal/crypto"
	"github.com/FayyazB106/account-manager/internal/store"
	"github.com/FayyazB106/account-manager/internal/totp"
)

const (
	totpIssuer      = "Account Manager"
	backupCodeCount = 10
)

// totpStatus (GET /profile/totp) reports whether MFA is on and how many backup
// codes remain, for the Settings → Admin UI.
func (s *Server) totpStatus(w http.ResponseWriter, r *http.Request) {
	adminID, ok := adminIDFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	a, err := s.Admins.GetByID(r.Context(), adminID)
	if err != nil {
		logErr("totpStatus", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	remaining := 0
	if a.TOTPEnabled {
		remaining, _ = s.Admins.BackupCodesRemaining(r.Context(), adminID)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled":                a.TOTPEnabled,
		"backup_codes_remaining": remaining,
	})
}

// setupTOTP (POST /profile/totp/setup) generates a new secret (MFA stays off
// until confirmed) and returns the provisioning URI + secret for the QR code.
func (s *Server) setupTOTP(w http.ResponseWriter, r *http.Request) {
	adminID, ok := adminIDFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	a, err := s.Admins.GetByID(r.Context(), adminID)
	if err != nil {
		logErr("setupTOTP: get admin", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	secret, err := totp.GenerateSecret()
	if err != nil {
		logErr("setupTOTP: gen secret", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	enc, err := crypto.Encrypt(s.MasterKeyValue(), []byte(secret))
	if err != nil {
		logErr("setupTOTP: encrypt", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if err := s.Admins.SetTOTPSecret(r.Context(), adminID, enc); err != nil {
		logErr("setupTOTP: store", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"secret":      secret,
		"otpauth_url": totp.OtpauthURL(secret, a.Email, totpIssuer),
	})
}

// enableTOTP (POST /profile/totp/enable) confirms a code, turns MFA on, and
// returns freshly generated single-use backup codes (shown once).
func (s *Server) enableTOTP(w http.ResponseWriter, r *http.Request) {
	adminID, ok := adminIDFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	var body struct {
		Code string `json:"code"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	a, err := s.Admins.GetByID(r.Context(), adminID)
	if err != nil {
		logErr("enableTOTP: get admin", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if a.TOTPSecret == nil {
		writeError(w, http.StatusBadRequest, "two-factor is not set up")
		return
	}
	secret, err := crypto.Decrypt(s.MasterKeyValue(), a.TOTPSecret)
	if err != nil {
		logErr("enableTOTP: decrypt", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !totp.Verify(string(secret), body.Code, time.Now()) {
		writeError(w, http.StatusBadRequest, "invalid code")
		return
	}
	codes := make([]string, backupCodeCount)
	hashes := make([]string, backupCodeCount)
	for i := range codes {
		c, err := totp.GenerateBackupCode()
		if err != nil {
			logErr("enableTOTP: backup code", err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		codes[i] = c
		hashes[i] = totp.HashCode(c)
	}
	if err := s.Admins.ReplaceBackupCodes(r.Context(), adminID, hashes); err != nil {
		logErr("enableTOTP: store codes", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if err := s.Admins.EnableTOTP(r.Context(), adminID); err != nil {
		logErr("enableTOTP: enable", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"backup_codes": codes})
}

// disableTOTP (POST /profile/totp/disable) turns MFA off after verifying a
// current code (or a backup code), so a hijacked session can't silently disable it.
func (s *Server) disableTOTP(w http.ResponseWriter, r *http.Request) {
	adminID, ok := adminIDFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	var body struct {
		Code string `json:"code"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	a, err := s.Admins.GetByID(r.Context(), adminID)
	if err != nil {
		logErr("disableTOTP: get admin", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !a.TOTPEnabled {
		writeJSON(w, http.StatusOK, map[string]string{"status": "disabled"})
		return
	}
	if !s.verifyMFACode(r, a, body.Code) {
		writeError(w, http.StatusBadRequest, "invalid code")
		return
	}
	if err := s.Admins.DisableTOTP(r.Context(), adminID); err != nil {
		logErr("disableTOTP: disable", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "disabled"})
}

// verifyMFACode accepts either a valid current TOTP code or an unused backup
// code (which it consumes). Returns false if neither matches.
func (s *Server) verifyMFACode(r *http.Request, a *store.Admin, code string) bool {
	if a.TOTPSecret != nil {
		if secret, err := crypto.Decrypt(s.MasterKeyValue(), a.TOTPSecret); err == nil {
			if totp.Verify(string(secret), code, time.Now()) {
				return true
			}
		}
	}
	used, err := s.Admins.UseBackupCode(r.Context(), a.ID, totp.HashCode(code))
	return err == nil && used
}
