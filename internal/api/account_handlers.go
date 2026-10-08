package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/FayyazB106/account-manager/internal/crypto"
	"github.com/FayyazB106/account-manager/internal/store"
)

type createAccountInput struct {
	CategoryID       *string `json:"category_id"`
	Service             string `json:"service"`
	Website             string `json:"website"`
	Login               string `json:"login"`
	Password            string `json:"password"`
	BillingPeriodMonths *int     `json:"billing_period_months"` // 0 = free
	Amount              *float64 `json:"amount"`                // optional billing amount ($)
	NextPaymentDate     string   `json:"next_payment_date"`     // "YYYY-MM-DD", optional
	Notes               string   `json:"notes"`
	RotateEveryDays     *int     `json:"rotate_every_days"` // optional rotation policy
	Seats               []string  `json:"seats"`             // one entry per seat; name may be empty
	SeatsAr             []*string `json:"seats_ar"`          // optional Arabic seat names, aligned by position
}

// billingMonths returns the validated period (0 = free) or an error if negative.
func (in *createAccountInput) billingMonths() (int, bool) {
	if in.BillingPeriodMonths == nil {
		return 0, true
	}
	if *in.BillingPeriodMonths < 0 {
		return 0, false
	}
	return *in.BillingPeriodMonths, true
}

// createAccount handles POST /accounts: encrypt the password with envelope
// encryption, then store the account.
func (s *Server) createAccount(w http.ResponseWriter, r *http.Request) {
	var in createAccountInput
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	in.Service = strings.TrimSpace(in.Service)
	if in.Service == "" || in.Password == "" {
		writeError(w, http.StatusBadRequest, "service and password are required")
		return
	}

	months, ok := in.billingMonths()
	if !ok {
		writeError(w, http.StatusBadRequest, "billing_period_months must be 0 or greater")
		return
	}

	// Encrypt the password with the server-side envelope (outer lock).
	env, err := crypto.EnvelopeEncrypt(s.MasterKeyValue(), []byte(in.Password))
	if err != nil {
		logErr("createAccount: envelope encrypt", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if in.RotateEveryDays != nil && *in.RotateEveryDays <= 0 {
		writeError(w, http.StatusBadRequest, "rotate_every_days must be positive")
		return
	}

	acc := &store.Account{
		CategoryID:          in.CategoryID,
		Service:             in.Service,
		BillingPeriodMonths: months,
		Amount:              in.Amount,
		RotateEveryDays:     in.RotateEveryDays,
		PasswordWrappedDEK:  env.WrappedDEK,
		PasswordCiphertext:  env.Ciphertext,
	}
	if in.Website = strings.TrimSpace(in.Website); in.Website != "" {
		acc.Website = &in.Website
	}
	if in.Login = strings.TrimSpace(in.Login); in.Login != "" {
		acc.Login = &in.Login
	}
	if in.Notes != "" {
		acc.Notes = &in.Notes
	}
	if in.NextPaymentDate != "" {
		d, err := time.Parse("2006-01-02", in.NextPaymentDate)
		if err != nil {
			writeError(w, http.StatusBadRequest, "next_payment_date must be YYYY-MM-DD")
			return
		}
		acc.NextPaymentDate = &d
	}

	id, err := s.Accounts.Create(r.Context(), acc)
	if err != nil {
		logErr("createAccount: insert", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := s.Accounts.ReplaceSeats(r.Context(), id, in.Seats, in.SeatsAr); err != nil {
		logErr("createAccount: seats", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

// revealAccount handles GET /accounts/{id}/reveal: decrypt and return the
// plaintext password for one account. Only reachable behind requireAuth.
//
// NOTE: for now the plaintext is returned over the API (protected by TLS in
// production). In Phase 4 the client adds the INNER lock, so the server will
// hand back a still-client-encrypted blob instead of raw plaintext.
func (s *Server) revealAccount(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing account id")
		return
	}

	wrappedDEK, ciphertext, err := s.Accounts.GetSecret(r.Context(), id)
	if err == store.ErrNotFound {
		writeError(w, http.StatusNotFound, "account not found")
		return
	}
	if err != nil {
		logErr("revealAccount: get secret", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	plaintext, err := crypto.EnvelopeDecrypt(s.MasterKeyValue(), &crypto.Envelope{
		WrappedDEK: wrappedDEK,
		Ciphertext: ciphertext,
	})
	if err != nil {
		logErr("revealAccount: decrypt", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"password": string(plaintext)})
}

// revealAllSecrets (GET /accounts/secrets) returns every account's decrypted
// password as {id: password}, for the Vault Health scan. Audited as a single
// "vault.scan" event (not one reveal per account).
func (s *Server) revealAllSecrets(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	secrets, err := s.Accounts.AllSecrets(r.Context())
	if err != nil {
		logErr("revealAllSecrets", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := make(map[string]string, len(secrets))
	for _, sec := range secrets {
		pt, derr := crypto.EnvelopeDecrypt(s.MasterKeyValue(), &crypto.Envelope{
			WrappedDEK: sec.PasswordWrappedDEK,
			Ciphertext: sec.PasswordCiphertext,
		})
		if derr != nil {
			continue
		}
		out[sec.ID] = string(pt)
	}
	writeJSON(w, http.StatusOK, out)
}

// listPasswordHistory (GET /accounts/{id}/history) returns previous passwords'
// metadata (id + changed_at), newest first — no secrets.
func (s *Server) listPasswordHistory(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	list, err := s.Accounts.ListHistory(r.Context(), id)
	if err != nil {
		logErr("listPasswordHistory", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if list == nil {
		list = []store.PasswordHistoryMeta{}
	}
	writeJSON(w, http.StatusOK, list)
}

// revealPasswordHistory (GET /accounts/{id}/history/{hid}/reveal) decrypts one
// archived password.
func (s *Server) revealPasswordHistory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	id := r.PathValue("id")
	hid := r.PathValue("hid")
	wrappedDEK, ciphertext, err := s.Accounts.GetHistorySecret(r.Context(), id, hid)
	if err == store.ErrNotFound {
		writeError(w, http.StatusNotFound, "account not found")
		return
	}
	if err != nil {
		logErr("revealPasswordHistory: get secret", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	plaintext, err := crypto.EnvelopeDecrypt(s.MasterKeyValue(), &crypto.Envelope{WrappedDEK: wrappedDEK, Ciphertext: ciphertext})
	if err != nil {
		logErr("revealPasswordHistory: decrypt", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"password": string(plaintext)})
}

// updateAccount handles PUT /accounts/{id}. All metadata is replaced. The
// password is optional: if a non-empty password is provided, it is re-encrypted
// and password_changed_at resets (restarting the rotation clock).
func (s *Server) updateAccount(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var in createAccountInput
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	in.Service = strings.TrimSpace(in.Service)
	if in.Service == "" {
		writeError(w, http.StatusBadRequest, "service is required")
		return
	}
	months, ok := in.billingMonths()
	if !ok {
		writeError(w, http.StatusBadRequest, "billing_period_months must be 0 or greater")
		return
	}
	if in.RotateEveryDays != nil && *in.RotateEveryDays <= 0 {
		writeError(w, http.StatusBadRequest, "rotate_every_days must be positive")
		return
	}

	acc := &store.Account{
		ID:                  id,
		CategoryID:          in.CategoryID,
		Service:             in.Service,
		BillingPeriodMonths: months,
		Amount:              in.Amount,
		RotateEveryDays:     in.RotateEveryDays,
	}
	if in.Website = strings.TrimSpace(in.Website); in.Website != "" {
		acc.Website = &in.Website
	}
	if in.Login = strings.TrimSpace(in.Login); in.Login != "" {
		acc.Login = &in.Login
	}
	if in.Notes != "" {
		acc.Notes = &in.Notes
	}
	if in.NextPaymentDate != "" {
		d, err := time.Parse("2006-01-02", in.NextPaymentDate)
		if err != nil {
			writeError(w, http.StatusBadRequest, "next_payment_date must be YYYY-MM-DD")
			return
		}
		acc.NextPaymentDate = &d
	}

	if err := s.Accounts.UpdateMeta(r.Context(), acc); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "account not found")
			return
		}
		logErr("updateAccount: update meta", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := s.Accounts.ReplaceSeats(r.Context(), id, in.Seats, in.SeatsAr); err != nil {
		logErr("updateAccount: seats", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Optional password change.
	if in.Password != "" {
		env, err := crypto.EnvelopeEncrypt(s.MasterKeyValue(), []byte(in.Password))
		if err != nil {
			logErr("updateAccount: encrypt", err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if err := s.Accounts.UpdatePassword(r.Context(), id, env.WrappedDEK, env.Ciphertext); err != nil {
			logErr("updateAccount: update password", err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// setAccountStatus handles PATCH /accounts/{id}/status with body {"status": ...}.
func (s *Server) setAccountStatus(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var body struct {
		Status string `json:"status"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.Status != "active" && body.Status != "inactive" {
		writeError(w, http.StatusBadRequest, "status must be 'active' or 'inactive'")
		return
	}

	if err := s.Accounts.SetStatus(r.Context(), id, body.Status); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "account not found")
			return
		}
		logErr("setAccountStatus", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": body.Status})
}

// deleteAccount handles DELETE /accounts/{id}. Only inactive accounts may be
// deleted; active ones must be deactivated first.
func (s *Server) deleteAccount(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	err := s.Accounts.Delete(r.Context(), id)
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "account not found")
	case errors.Is(err, store.ErrNotInactive):
		writeError(w, http.StatusConflict, "account must be inactive before it can be deleted")
	case err != nil:
		logErr("deleteAccount", err)
		writeError(w, http.StatusInternalServerError, "internal error")
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

// extendAccount handles POST /accounts/{id}/extend: advance the next payment
// date by one billing period.
func (s *Server) extendAccount(w http.ResponseWriter, r *http.Request) {
	next, err := s.Accounts.Extend(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusBadRequest, "account has no payment date or billing period to extend")
		return
	}
	if err != nil {
		logErr("extendAccount", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"next_payment_date": next.Format("2006-01-02")})
}

// listAccounts handles GET /accounts: returns secret-free metadata only. The
// encrypted password is NEVER included in a list response.
func (s *Server) listAccounts(w http.ResponseWriter, r *http.Request) {
	items, err := s.Accounts.List(r.Context())
	if err != nil {
		logErr("listAccounts: query", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if items == nil {
		items = []store.AccountListItem{} // return [] not null
	}
	writeJSON(w, http.StatusOK, items)
}
