package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/FayyazB106/account-manager/internal/db"
	"github.com/FayyazB106/account-manager/internal/store"
)

// A seat must be a user from the Users list: unknown names are refused with a 400, known
// ones are accepted (ignoring case) and rewritten to the person's exact name.
func TestCheckSeatsRequiresUsersFromTheList(t *testing.T) {
	d, err := db.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	s := &Server{People: store.NewPersonStore(d), Accounts: store.NewAccountStore(d)}
	if _, err := s.People.Create(context.Background(), "Mark", nil); err != nil {
		t.Fatal(err)
	}

	in := &createAccountInput{Seats: []string{"mark"}}
	rec := httptest.NewRecorder()
	if !s.checkSeats(rec, httptest.NewRequest(http.MethodPost, "/accounts", nil), in, "") || in.Seats[0] != "Mark" {
		t.Fatalf("a known user should pass and be stored as 'Mark': ok=%v seats=%v", rec.Code, in.Seats)
	}

	in = &createAccountInput{Seats: []string{"Mark", "user"}}
	rec = httptest.NewRecorder()
	if s.checkSeats(rec, httptest.NewRequest(http.MethodPost, "/accounts", nil), in, "") || rec.Code != http.StatusBadRequest {
		t.Fatalf("a free-text seat must be refused with 400, got ok/%d", rec.Code)
	}
}
