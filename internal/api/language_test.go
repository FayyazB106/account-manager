package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/FayyazB106/account-manager/internal/auth"
	"github.com/FayyazB106/account-manager/internal/db"
	"github.com/FayyazB106/account-manager/internal/store"
)

func newLangServer(t *testing.T) (*Server, string) {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "lang.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	admins := store.NewAdminStore(d)
	id, err := admins.Create(context.Background(), "a@example.com", "A", nil, "h", []byte("s"), []byte("w"))
	if err != nil {
		t.Fatal(err)
	}
	return &Server{Admins: admins, JWTSecret: []byte("test-secret-test-secret-test-secret")}, id
}

func langOf(t *testing.T, s *Server, id string) string {
	t.Helper()
	a, err := s.Admins.GetByID(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return a.Language
}

func reqWithLang(lang string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	if lang != "" {
		r.Header.Set("X-App-Language", lang)
	}
	return r
}

// The language the app says it is showing becomes the admin's email language; junk or
// a missing header never changes it; and the database is only written on a change.
func TestNoteLanguageRecordsTheDisplayedLanguage(t *testing.T) {
	s, id := newLangServer(t)
	ctx := context.Background()
	if langOf(t, s, id) != "en" {
		t.Fatal("a new admin should default to English")
	}

	s.noteLanguage(reqWithLang("ar"), id)
	if langOf(t, s, id) != "ar" {
		t.Fatal("X-App-Language: ar was not recorded")
	}

	for _, junk := range []string{"", "fr", "AR", "ar<script>", "en,ar"} {
		s.noteLanguage(reqWithLang(junk), id)
		if langOf(t, s, id) != "ar" {
			t.Fatalf("header %q changed the stored language", junk)
		}
	}

	// Same language again must not write: change it behind the server's back, repeat
	// the header, and the value stays put (proving the in-memory check short-circuits).
	if err := s.Admins.SetLanguage(ctx, id, "en"); err != nil {
		t.Fatal(err)
	}
	s.noteLanguage(reqWithLang("ar"), id)
	if langOf(t, s, id) != "en" {
		t.Fatal("an unchanged language still hit the database")
	}
	// A real change is written.
	s.noteLanguage(reqWithLang("en"), id)
	s.noteLanguage(reqWithLang("ar"), id)
	if langOf(t, s, id) != "ar" {
		t.Fatal("a language change was not recorded")
	}

	// An admin that no longer exists must not panic or be cached.
	s.noteLanguage(reqWithLang("ar"), "does-not-exist")
	if _, cached := s.langSeen["does-not-exist"]; cached {
		t.Fatal("a failed write was cached")
	}
}

// Through the real auth middleware: the language is already stored when the handler
// runs, so a "Send Now" in the same request is written in the language on screen.
func TestRequireAuthNotesTheLanguageBeforeTheHandlerRuns(t *testing.T) {
	s, id := newLangServer(t)
	token, err := auth.IssueToken(id, s.JWTSecret)
	if err != nil {
		t.Fatal(err)
	}

	var seenInHandler string
	h := s.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		seenInHandler = langOf(t, s, id)
		w.WriteHeader(http.StatusNoContent)
	})

	r := reqWithLang("ar")
	r.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h(rec, r)
	if rec.Code != http.StatusNoContent || seenInHandler != "ar" {
		t.Fatalf("status %d, language seen by the handler = %q, want ar", rec.Code, seenInHandler)
	}

	// An unauthenticated request must not be able to set anyone's language.
	s2, id2 := newLangServer(t)
	rec = httptest.NewRecorder()
	s2.requireAuth(func(http.ResponseWriter, *http.Request) {})(rec, reqWithLang("ar"))
	if rec.Code != http.StatusUnauthorized || langOf(t, s2, id2) != "en" {
		t.Fatalf("unauthenticated request: status %d, language %q", rec.Code, langOf(t, s2, id2))
	}
}
