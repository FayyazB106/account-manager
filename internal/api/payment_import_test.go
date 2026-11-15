package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FayyazB106/account-manager/internal/db"
	"github.com/FayyazB106/account-manager/internal/store"
)

func importReq(id, body string) (*httptest.ResponseRecorder, *http.Request) {
	r := httptest.NewRequest(http.MethodPost, "/accounts/"+id+"/payments/import", strings.NewReader(body))
	r.SetPathValue("id", id)
	return httptest.NewRecorder(), r
}

// Imported rows land in the account's history; rows with no currency take the account's
// own; importing the same rows again skips them; bad rows are refused with a 400 and add
// nothing.
func TestImportPayments(t *testing.T) {
	d, err := db.Open(filepath.Join(t.TempDir(), "i.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	s := &Server{Accounts: store.NewAccountStore(d), Payments: store.NewPaymentStore(d)}
	ctx := context.Background()
	id, err := s.Accounts.Create(ctx, &store.Account{Service: "Zoom", Currency: "SAR", PasswordWrappedDEK: []byte{1}, PasswordCiphertext: []byte{1}})
	if err != nil {
		t.Fatal(err)
	}

	good := `{"payments":[
		{"paid_on":"2025-01-15","amount":49.5,"note":"January"},
		{"paid_on":"2025-02-15","amount":10,"currency":"usd"}]}`
	rec, req := importReq(id, good)
	s.importPayments(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"added":2`) {
		t.Fatalf("first import: %d %s", rec.Code, rec.Body.String())
	}
	list, err := s.Payments.ListByAccount(ctx, id)
	if err != nil || len(list) != 2 {
		t.Fatalf("history should have 2 payments: %v %v", list, err)
	}
	cur := map[string]string{}
	for _, p := range list {
		cur[p.PaidOn.Format("2006-01-02")] = p.Currency
	}
	if cur["2025-01-15"] != "SAR" || cur["2025-02-15"] != "USD" {
		t.Errorf("currency should default to the account's (SAR) and be upper-cased: %v", cur)
	}

	rec, req = importReq(id, good) // the same file again
	s.importPayments(rec, req)
	if !strings.Contains(rec.Body.String(), `"added":0`) || !strings.Contains(rec.Body.String(), `"skipped":2`) {
		t.Fatalf("a repeat import should skip everything: %s", rec.Body.String())
	}

	for name, body := range map[string]string{
		"bad date":     `{"payments":[{"paid_on":"15/01/2025","amount":1}]}`,
		"no amount":    `{"payments":[{"paid_on":"2025-03-01"}]}`,
		"negative":     `{"payments":[{"paid_on":"2025-03-01","amount":-5}]}`,
		"bad currency": `{"payments":[{"paid_on":"2025-03-01","amount":5,"currency":"DOLLAR"}]}`,
		"empty":        `{"payments":[]}`,
	} {
		rec, req = importReq(id, body)
		s.importPayments(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s should be refused with 400, got %d", name, rec.Code)
		}
	}
	if list, _ = s.Payments.ListByAccount(ctx, id); len(list) != 2 {
		t.Errorf("refused imports must add nothing, history has %d", len(list))
	}

	rec, req = importReq("missing", good)
	s.importPayments(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("an unknown account should be a 404, got %d", rec.Code)
	}
}
