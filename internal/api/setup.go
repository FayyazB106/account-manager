package api

import (
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/FayyazB106/account-manager/internal/crypto"
	"github.com/FayyazB106/account-manager/internal/store"
)

const minPassphraseLen = 8

// appState reports the onboarding state the browser needs:
//   - "ready"  — vault unlocked (master key available); normal login/app.
//   - "locked" — a sealed key file exists but isn't unlocked this run → Unlock page.
//   - "setup"  — no key at all → first-run Setup page (passphrase + first admin).
func (s *Server) appState() string {
	if s.ready() {
		return "ready"
	}
	if _, err := os.Stat(s.KeyPath); err == nil {
		return "locked"
	}
	return "setup"
}

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	out := map[string]string{"state": s.appState()}
	// Server mode (MASTER_KEY in the environment) starts unlocked with no admin and
	// skips the browser setup, so tell the UI to offer creating the first admin.
	if out["state"] == "ready" {
		if n, err := s.Admins.Count(r.Context()); err == nil && n == 0 {
			out["needs_admin"] = "1"
		}
	}
	writeJSON(w, http.StatusOK, out)
}

type setupInput struct {
	Passphrase string `json:"passphrase"`
	Email      string `json:"email"`
	Password   string `json:"password"`
}

// setup handles POST /setup — the first-run flow: choose a passphrase (which
// seals a freshly generated master key) and create the first admin, all from the
// browser. Only valid while uninitialized. On success the caller is logged in.
func (s *Server) setup(w http.ResponseWriter, r *http.Request) {
	if s.appState() != "setup" {
		writeError(w, http.StatusConflict, "already initialized")
		return
	}
	var in setupInput
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	in.Email = strings.TrimSpace(strings.ToLower(in.Email))
	if in.Email == "" || len(in.Password) < minPasswordLen {
		writeError(w, http.StatusBadRequest, "email required and password must be at least 12 characters")
		return
	}
	if len(in.Passphrase) < minPassphraseLen {
		writeError(w, http.StatusBadRequest, "passphrase must be at least 8 characters")
		return
	}

	// Generate + seal the master key under the chosen passphrase.
	key, err := crypto.RandomBytes(crypto.KeySize)
	if err != nil {
		logErr("setup: gen key", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	sk, err := crypto.SealKey(key, in.Passphrase)
	if err != nil {
		logErr("setup: seal key", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if err := crypto.SaveSealedKeyFile(s.KeyPath, sk); err != nil {
		logErr("setup: save key", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	s.setMasterKey(key)

	// Create the first admin. On failure, roll back the key so a retry is clean.
	authHash, kekSalt, wrappedVaultKey, err := adminCredentials(in.Password)
	if err != nil {
		logErr("setup: credentials", err)
		s.rollbackSetup()
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	id, err := s.Admins.CreateFirst(r.Context(), in.Email, "", nil, authHash, kekSalt, wrappedVaultKey)
	if err != nil {
		s.rollbackSetup()
		if errors.Is(err, store.ErrRegistrationClosed) {
			writeError(w, http.StatusConflict, "already initialized")
			return
		}
		logErr("setup: create admin", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	s.auditRaw(r, in.Email, "auth.login", "", "first-run setup", "ok")
	s.issueSession(w, r, id) // log the new admin straight in
}

// rollbackSetup undoes a partial setup so the next attempt starts from "setup".
func (s *Server) rollbackSetup() {
	_ = os.Remove(s.KeyPath)
	s.setMasterKey(nil)
}

// unlock handles POST /unlock — enter the passphrase to unseal an existing vault
// (personal mode after a restart). Rate-limited to slow passphrase guessing.
func (s *Server) unlock(w http.ResponseWriter, r *http.Request) {
	if s.appState() != "locked" {
		writeError(w, http.StatusConflict, "the vault is not locked")
		return
	}
	if ok, retry := loginRL.allowed("unlock"); !ok {
		w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())+1))
		writeError(w, http.StatusTooManyRequests, "too many attempts, try again later")
		return
	}

	var in struct {
		Passphrase string `json:"passphrase"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	sk, err := crypto.LoadSealedKeyFile(s.KeyPath)
	if err != nil {
		logErr("unlock: load key", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	key, err := crypto.UnsealKey(sk, in.Passphrase)
	if err != nil {
		loginRL.fail("unlock")
		s.auditRaw(r, "", "auth.unlock", "", "", "fail")
		writeError(w, http.StatusUnauthorized, "incorrect passphrase")
		return
	}
	loginRL.reset("unlock")
	s.setMasterKey(key)
	s.auditRaw(r, "", "auth.unlock", "", "", "ok")
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
