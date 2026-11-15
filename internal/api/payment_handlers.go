package api

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/FayyazB106/account-manager/internal/store"
)

// paymentInput is the JSON body for creating/updating a manual payment.
type paymentInput struct {
	PaidOn       string   `json:"paid_on"` // "YYYY-MM-DD"
	Amount       *float64 `json:"amount"`
	Currency     string   `json:"currency"`
	Note         string   `json:"note"`
	AttachmentID string   `json:"attachment_id"` // optional; must belong to the same account
}

// toPayment validates the input and builds a store.Payment for the given account.
// The caller supplies accountID (from the path for create, from the existing row
// for update). It returns a client-facing error message on invalid input.
func (in *paymentInput) toPayment(accountID string) (*store.Payment, string) {
	d, err := time.Parse("2006-01-02", strings.TrimSpace(in.PaidOn))
	if err != nil {
		return nil, "paid_on must be YYYY-MM-DD"
	}
	if in.Amount == nil {
		return nil, "amount is required"
	}
	p := &store.Payment{
		AccountID: accountID,
		PaidOn:    d,
		Amount:    *in.Amount,
		Currency:  strings.ToUpper(strings.TrimSpace(in.Currency)),
	}
	if p.Currency == "" {
		p.Currency = "USD"
	}
	if n := strings.TrimSpace(in.Note); n != "" {
		p.Note = &n
	}
	if a := strings.TrimSpace(in.AttachmentID); a != "" {
		p.AttachmentID = &a
	}
	return p, ""
}

// verifyAttachmentOwnership checks that a linked attachment (if any) belongs to
// the payment's account. Returns false (after writing the response) on mismatch.
func (s *Server) verifyAttachmentOwnership(w http.ResponseWriter, r *http.Request, p *store.Payment) bool {
	if p.AttachmentID == nil {
		return true
	}
	owner, err := s.Attachments.AccountOf(r.Context(), *p.AttachmentID)
	if errors.Is(err, store.ErrNotFound) || (err == nil && owner != p.AccountID) {
		writeError(w, http.StatusBadRequest, "attachment not found")
		return false
	}
	if err != nil {
		logErr("payment: attachment owner", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return false
	}
	return true
}

// listAccountPayments handles GET /accounts/{id}/payments.
func (s *Server) listAccountPayments(w http.ResponseWriter, r *http.Request) {
	items, err := s.Payments.ListByAccount(r.Context(), r.PathValue("id"))
	if err != nil {
		logErr("listAccountPayments", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if items == nil {
		items = []store.Payment{}
	}
	writeJSON(w, http.StatusOK, items)
}

// listAllPayments handles GET /payments — the global ledger across all accounts.
func (s *Server) listAllPayments(w http.ResponseWriter, r *http.Request) {
	items, err := s.Payments.ListAll(r.Context())
	if err != nil {
		logErr("listAllPayments", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if items == nil {
		items = []store.PaymentListItem{}
	}
	writeJSON(w, http.StatusOK, items)
}

// createPayment handles POST /accounts/{id}/payments (manual entry).
func (s *Server) createPayment(w http.ResponseWriter, r *http.Request) {
	accountID := r.PathValue("id")
	var in paymentInput
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	p, msg := in.toPayment(accountID)
	if msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	if !s.verifyAttachmentOwnership(w, r, p) {
		return
	}
	id, err := s.Payments.Create(r.Context(), p)
	if err != nil {
		logErr("createPayment", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

// updatePayment handles PUT /payments/{id}. The account is taken from the stored
// row (payments can't be moved between accounts).
func (s *Server) updatePayment(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	accountID, err := s.Payments.AccountOf(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "payment not found")
		return
	}
	if err != nil {
		logErr("updatePayment: account of", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	var in paymentInput
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	p, msg := in.toPayment(accountID)
	if msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	p.ID = id
	if !s.verifyAttachmentOwnership(w, r, p) {
		return
	}
	if err := s.Payments.Update(r.Context(), p); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "payment not found")
			return
		}
		logErr("updatePayment", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// deletePayment handles DELETE /payments/{id}.
func (s *Server) deletePayment(w http.ResponseWriter, r *http.Request) {
	err := s.Payments.Delete(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "payment not found")
		return
	}
	if err != nil {
		logErr("deletePayment", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// maxImportRows bounds one payment-history import.
const maxImportRows = 5000

// importPayments handles POST /accounts/{id}/payments/import with body
// {"payments": [{paid_on, amount, currency, note}]}: bulk-add payments to the account's history.
// A row with no currency uses the account's own. Rows the account already has are skipped.
// Responds {"added": n, "skipped": m}.
func (s *Server) importPayments(w http.ResponseWriter, r *http.Request) {
	accountID := r.PathValue("id")
	var body struct {
		Payments []paymentInput `json:"payments"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(body.Payments) == 0 {
		writeError(w, http.StatusBadRequest, "no payments to import")
		return
	}
	if len(body.Payments) > maxImportRows {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("at most %d payments can be imported at once", maxImportRows))
		return
	}
	accCurrency, err := s.Accounts.CurrencyOf(r.Context(), accountID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "account not found")
		return
	}
	if err != nil {
		logErr("importPayments: account currency", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	ps := make([]store.Payment, 0, len(body.Payments))
	for i := range body.Payments {
		in := &body.Payments[i]
		if strings.TrimSpace(in.Currency) == "" {
			in.Currency = accCurrency
		}
		in.AttachmentID = "" // an import never links invoices
		p, msg := in.toPayment(accountID)
		if msg != "" {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("row %d: %s", i+1, msg))
			return
		}
		if p.Amount < 0 {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("row %d: amount can't be negative", i+1))
			return
		}
		if c, ok := normalizeCurrency(p.Currency); ok {
			p.Currency = c
		} else {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("row %d: currency must be a 3-letter code", i+1))
			return
		}
		ps = append(ps, *p)
	}

	added, skipped, err := s.Payments.ImportMany(r.Context(), accountID, ps)
	if err != nil {
		logErr("importPayments", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"added": added, "skipped": skipped})
}
