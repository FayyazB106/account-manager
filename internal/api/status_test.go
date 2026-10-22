package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/FayyazB106/account-manager/internal/db"
	"github.com/FayyazB106/account-manager/internal/store"
)

// An unlocked vault with no admin (server mode) tells the UI to offer creating the
// first admin; once one exists the flag disappears.
func TestStatusReportsWhenTheFirstAdminIsNeeded(t *testing.T) {
	d, err := db.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	admins := store.NewAdminStore(d)
	s := &Server{Admins: admins, MasterKey: []byte("0123456789abcdef0123456789abcdef")}

	get := func() map[string]string {
		rec := httptest.NewRecorder()
		s.status(rec, httptest.NewRequest(http.MethodGet, "/status", nil))
		var out map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}

	if got := get(); got["state"] != "ready" || got["needs_admin"] != "1" {
		t.Fatalf("empty vault: %v", got)
	}
	if _, err := admins.Create(context.Background(), "a@example.com", "A", nil, "h", []byte("s"), []byte("w")); err != nil {
		t.Fatal(err)
	}
	if got := get(); got["state"] != "ready" || got["needs_admin"] != "" {
		t.Fatalf("with an admin: %v", got)
	}
}
