package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FayyazB106/account-manager/internal/db"
	"github.com/FayyazB106/account-manager/internal/store"
)

// The settings list must never include the sealed SMTP password, while ordinary
// settings (and the non-secret SMTP fields) are still returned.
func TestListSettingsHidesSecrets(t *testing.T) {
	d, err := db.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	settings := store.NewSettingStore(d)
	ctx := context.Background()
	for k, v := range map[string]string{
		"week_start":        "1",
		"smtp_host":         "smtp.resend.com",
		"smtp_password_enc": "v1:SEALED.SECRET",
	} {
		if err := settings.Set(ctx, k, v); err != nil {
			t.Fatal(err)
		}
	}

	s := &Server{Settings: settings}
	rec := httptest.NewRecorder()
	s.listSettings(rec, httptest.NewRequest(http.MethodGet, "/settings", nil))

	if strings.Contains(rec.Body.String(), "SEALED") || strings.Contains(rec.Body.String(), "smtp_password_enc") {
		t.Fatalf("GET /settings leaked the SMTP password: %s", rec.Body.String())
	}
	var got map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["week_start"] != "1" || got["smtp_host"] != "smtp.resend.com" {
		t.Errorf("ordinary settings missing from the response: %v", got)
	}
}

// SMTP settings carry a secret and must be written as a validated set through
// PUT /smtp, never one key at a time through the generic settings endpoint.
func TestGenericSettingsEndpointRejectsSMTPKeys(t *testing.T) {
	for _, k := range []string{"smtp_host", "smtp_password_enc", "smtp_from"} {
		if err := validateSetting(k, "x"); err == nil {
			t.Errorf("validateSetting(%q) should be rejected", k)
		}
	}
	if err := validateSetting("week_start", "1"); err != nil {
		t.Errorf("ordinary setting rejected: %v", err)
	}
}
