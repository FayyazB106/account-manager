package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/FayyazB106/account-manager/internal/auth"
	"github.com/FayyazB106/account-manager/internal/crypto"
	"github.com/FayyazB106/account-manager/internal/store"
)

// minPasswordLen enforces a strong master password. Length beats complexity for
// entropy, so we require a long passphrase rather than fiddly character rules.
const minPasswordLen = 12

type credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// register handles POST /register.
//
// BOOTSTRAP ONLY: it succeeds only when no admin exists yet, creating the first
// admin and generating the shared Vault Key. Once an admin exists, registration
// is closed — further admins are added via an invite flow (a logged-in admin
// re-wraps the Vault Key for the newcomer), built in a later phase.
func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	var c credentials
	if err := decodeJSON(w, r, &c); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	c.Email = strings.TrimSpace(strings.ToLower(c.Email))
	if c.Email == "" || len(c.Password) < minPasswordLen {
		writeError(w, http.StatusBadRequest, "email required and password must be at least 12 characters")
		return
	}

	ctx := r.Context()
	authHash, kekSalt, wrappedVaultKey, err := adminCredentials(c.Password)
	if err != nil {
		logErr("register: build credentials", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// CreateFirst inserts only when no admin exists — an atomic guard so two
	// concurrent bootstrap registrations can't both succeed.
	id, err := s.Admins.CreateFirst(ctx, c.Email, "", nil, authHash, kekSalt, wrappedVaultKey)
	if errors.Is(err, store.ErrRegistrationClosed) {
		writeError(w, http.StatusForbidden, "registration is closed; ask an existing admin to invite you")
		return
	}
	if err != nil {
		logErr("register: create admin", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{"id": id, "email": c.Email})
}

// createAdmin builds a new admin: hashes the password for login, derives a KEK
// from the password, generates a Vault Key, and wraps it with the KEK.
//
// NOTE (shared-vault caveat): each admin currently gets their OWN Vault Key.
// True shared access (all admins unwrapping the SAME key) requires an existing
// admin's password to re-wrap that key for the newcomer — deferred until the
// client-side encryption layer exists. Today secrets are protected by the
// server-side envelope (MASTER_KEY), so this doesn't affect confidentiality.
func (s *Server) createAdmin(ctx context.Context, email, name string, nameAr *string, password string) (string, error) {
	authHash, kekSalt, wrappedVaultKey, err := adminCredentials(password)
	if err != nil {
		return "", err
	}
	return s.Admins.Create(ctx, email, name, nameAr, authHash, kekSalt, wrappedVaultKey)
}

// adminCredentials derives the stored login/encryption material from a password:
// an Argon2id login hash, a KEK salt, and a per-admin Vault Key wrapped with the
// password-derived KEK. No DB access — so callers can do this before a short
// transaction.
func adminCredentials(password string) (authHash string, kekSalt, wrappedVaultKey []byte, err error) {
	authHash, err = crypto.HashPassword(password)
	if err != nil {
		return "", nil, nil, err
	}
	kekSalt, err = crypto.RandomBytes(crypto.SaltSize)
	if err != nil {
		return "", nil, nil, err
	}
	kek := crypto.DeriveKey(password, kekSalt)
	vaultKey, err := crypto.RandomBytes(crypto.KeySize)
	if err != nil {
		return "", nil, nil, err
	}
	wrappedVaultKey, err = crypto.Encrypt(kek, vaultKey)
	if err != nil {
		return "", nil, nil, err
	}
	return authHash, kekSalt, wrappedVaultKey, nil
}

// login handles POST /login: verify the password, then issue a session JWT.
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var c credentials
	if err := decodeJSON(w, r, &c); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	c.Email = strings.TrimSpace(strings.ToLower(c.Email))

	// Throttle repeated failed logins for this email (online brute-force guard).
	if ok, retry := loginRL.allowed(c.Email); !ok {
		w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())+1))
		writeError(w, http.StatusTooManyRequests, "too many attempts, try again later")
		return
	}

	ctx := r.Context()
	admin, err := s.Admins.GetByEmail(ctx, c.Email)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		logErr("login: lookup admin", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Guard against user enumeration: whether or not the email exists, we do a
	// password verification and return the SAME generic error on failure, so an
	// attacker can't tell which emails are registered (and timing stays similar).
	valid := false
	if admin != nil {
		valid, _ = crypto.VerifyPassword(c.Password, admin.AuthHash)
	} else {
		// Dummy verify against a throwaway hash to equalize work/timing.
		_, _ = crypto.VerifyPassword(c.Password, dummyHash)
	}

	if !valid {
		loginRL.fail(c.Email)
		s.auditRaw(r, c.Email, "auth.login", c.Email, "", "fail")
		writeError(w, http.StatusUnauthorized, "invalid email or password")
		return
	}
	loginRL.reset(c.Email)

	// If this admin has MFA enabled, the password is only the first factor: return
	// a short-lived MFA token to be exchanged for a session via POST /login/totp.
	if admin.TOTPEnabled {
		mfaToken, err := auth.IssueMFAToken(admin.ID, s.JWTSecret)
		if err != nil {
			logErr("login: issue mfa token", err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"mfa_required": true, "mfa_token": mfaToken})
		return
	}

	s.auditRaw(r, admin.Email, "auth.login", "", "", "ok")
	s.issueSession(w, r, admin.ID)
}

// loginTOTP handles POST /login/totp: it exchanges the MFA-pending token plus a
// valid authenticator code (or backup code) for a full session token.
func (s *Server) loginTOTP(w http.ResponseWriter, r *http.Request) {
	var body struct {
		MFAToken string `json:"mfa_token"`
		Code     string `json:"code"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	adminID, err := auth.ParseMFAToken(body.MFAToken, s.JWTSecret)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid or expired token")
		return
	}

	// Throttle code guessing per admin.
	rlKey := "mfa:" + adminID
	if ok, retry := loginRL.allowed(rlKey); !ok {
		w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())+1))
		writeError(w, http.StatusTooManyRequests, "too many attempts, try again later")
		return
	}

	admin, err := s.Admins.GetByID(r.Context(), adminID)
	if err != nil {
		logErr("loginTOTP: get admin", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !admin.TOTPEnabled || !s.verifyMFACode(r, admin, body.Code) {
		loginRL.fail(rlKey)
		s.auditRaw(r, admin.Email, "auth.mfa", "", "", "fail")
		writeError(w, http.StatusUnauthorized, "invalid code")
		return
	}
	loginRL.reset(rlKey)

	s.auditRaw(r, admin.Email, "auth.login", "", "via 2FA", "ok")
	s.issueSession(w, r, admin.ID)
}

// dummyHash is a valid Argon2id hash of a random value, used to spend
// comparable CPU time when an email doesn't exist (anti-enumeration).
var dummyHash, _ = crypto.HashPassword("this-is-never-a-real-password")
