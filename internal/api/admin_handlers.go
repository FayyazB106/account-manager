package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/FayyazB106/account-manager/internal/crypto"
	"github.com/FayyazB106/account-manager/internal/store"
)

// listAdmins handles GET /admins — secret-free list of admins.
func (s *Server) listAdmins(w http.ResponseWriter, r *http.Request) {
	admins, err := s.Admins.List(r.Context())
	if err != nil {
		logErr("listAdmins", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if admins == nil {
		admins = []store.AdminInfo{}
	}
	writeJSON(w, http.StatusOK, admins)
}

type newAdminInput struct {
	Email    string  `json:"email"`
	Name     string  `json:"name"`
	NameAr   *string `json:"name_ar"`
	Password string  `json:"password"`
}

// createAdminHandler handles POST /admins — add another admin (name, email,
// password). Requires an authenticated admin.
func (s *Server) createAdminHandler(w http.ResponseWriter, r *http.Request) {
	var in newAdminInput
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	in.Email = strings.TrimSpace(strings.ToLower(in.Email))
	in.Name = strings.TrimSpace(in.Name)
	in.NameAr = cleanOptAr(in.NameAr)
	if in.Email == "" || len(in.Password) < minPasswordLen {
		writeError(w, http.StatusBadRequest, "email required and password must be at least 12 characters")
		return
	}

	id, err := s.createAdmin(r.Context(), in.Email, in.Name, in.NameAr, in.Password)
	if err != nil {
		if store.IsUniqueViolation(err) {
			writeError(w, http.StatusConflict, "an admin with that email already exists")
			return
		}
		logErr("createAdminHandler", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, store.AdminInfo{ID: id, Email: in.Email, Name: in.Name, NameAr: in.NameAr})
}

// deleteAdmin handles DELETE /admins/{id}. An admin cannot delete themselves
// (prevents locking yourself out).
func (s *Server) deleteAdmin(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if me, _ := adminIDFrom(r.Context()); me == id {
		writeError(w, http.StatusBadRequest, "you cannot delete your own account")
		return
	}
	err := s.Admins.Delete(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "admin not found")
		return
	}
	if errors.Is(err, store.ErrLastAdmin) {
		writeError(w, http.StatusBadRequest, "cannot delete the last admin")
		return
	}
	if err != nil {
		logErr("deleteAdmin", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// getProfile handles GET /profile — the logged-in admin's own info.
func (s *Server) getProfile(w http.ResponseWriter, r *http.Request) {
	adminID, ok := adminIDFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	a, err := s.Admins.GetByID(r.Context(), adminID)
	if err != nil {
		logErr("getProfile", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, store.AdminInfo{ID: a.ID, Email: a.Email, Name: a.Name, NameAr: a.NameAr, CreatedAt: a.CreatedAt})
}

type updateProfileInput struct {
	Name   string  `json:"name"`
	NameAr *string `json:"name_ar"`
	Email  string  `json:"email"`
}

// updateProfile handles PUT /profile — change the logged-in admin's name/email.
func (s *Server) updateProfile(w http.ResponseWriter, r *http.Request) {
	adminID, ok := adminIDFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	var in updateProfileInput
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	in.Email = strings.TrimSpace(strings.ToLower(in.Email))
	in.Name = strings.TrimSpace(in.Name)
	in.NameAr = cleanOptAr(in.NameAr)
	if in.Email == "" {
		writeError(w, http.StatusBadRequest, "email is required")
		return
	}
	if err := s.Admins.UpdateProfile(r.Context(), adminID, in.Name, in.NameAr, in.Email); err != nil {
		if store.IsUniqueViolation(err) {
			writeError(w, http.StatusConflict, "that email is already in use")
			return
		}
		logErr("updateProfile", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

type changePasswordInput struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// changePassword handles POST /profile/password for the logged-in admin. It
// verifies the current password, then RE-KEYS: unwraps the Vault Key with the
// old password-derived key and re-wraps it with the new one, so the key
// hierarchy stays intact (changing only the login hash would orphan the key).
func (s *Server) changePassword(w http.ResponseWriter, r *http.Request) {
	adminID, ok := adminIDFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var in changePasswordInput
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(in.NewPassword) < minPasswordLen {
		writeError(w, http.StatusBadRequest, "new password must be at least 12 characters")
		return
	}

	admin, err := s.Admins.GetByID(r.Context(), adminID)
	if err != nil {
		logErr("changePassword: get admin", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	ok, _ = crypto.VerifyPassword(in.CurrentPassword, admin.AuthHash)
	if !ok {
		writeError(w, http.StatusUnauthorized, "current password is incorrect")
		return
	}

	// Re-key: unwrap the Vault Key with the old KEK, re-wrap with a new one.
	oldKEK := crypto.DeriveKey(in.CurrentPassword, admin.KEKSalt)
	vaultKey, err := crypto.Decrypt(oldKEK, admin.WrappedVaultKey)
	if err != nil {
		logErr("changePassword: unwrap vault key", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	newAuthHash, err := crypto.HashPassword(in.NewPassword)
	if err != nil {
		logErr("changePassword: hash", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	newKEKSalt, err := crypto.RandomBytes(crypto.SaltSize)
	if err != nil {
		logErr("changePassword: salt", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	newKEK := crypto.DeriveKey(in.NewPassword, newKEKSalt)
	newWrapped, err := crypto.Encrypt(newKEK, vaultKey)
	if err != nil {
		logErr("changePassword: rewrap", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := s.Admins.UpdateCredentials(r.Context(), adminID, newAuthHash, newKEKSalt, newWrapped); err != nil {
		logErr("changePassword: update", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	// Changing the password signs out all OTHER sessions, then re-issues a fresh
	// refresh cookie for this tab so the person who just changed it stays logged in.
	if err := s.Refresh.RevokeAllForAdmin(r.Context(), adminID); err != nil {
		logErr("changePassword: revoke sessions", err)
	}
	if cookie, err := s.Refresh.Issue(r.Context(), adminID, refreshTTL); err != nil {
		logErr("changePassword: reissue session", err)
	} else {
		s.setRefreshCookie(w, r, cookie)
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "password changed"})
}
