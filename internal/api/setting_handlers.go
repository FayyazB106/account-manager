package api

import (
	"net/http"
	"strconv"
	"strings"
)

func (s *Server) listSettings(w http.ResponseWriter, r *http.Request) {
	all, err := s.Settings.All(r.Context())
	if err != nil {
		logErr("listSettings", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, all)
}

// branding handles GET /branding — public (no auth) org name + logo for the
// login screen. Only these two non-sensitive fields are exposed.
func (s *Server) branding(w http.ResponseWriter, r *http.Request) {
	all, err := s.Settings.All(r.Context())
	if err != nil {
		logErr("branding", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"org_name": all["org_name"],
		"org_logo": all["org_logo"],
	})
}

// setSetting handles PUT /settings/{key} with body {"value": "..."}.
func (s *Server) setSetting(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	var body struct {
		Value string `json:"value"`
	}
	// Settings can hold a base64 logo image, so allow a larger body (~10 MiB).
	if err := decodeJSONLimit(w, r, &body, 10<<20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := validateSetting(key, body.Value); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.Settings.Set(r.Context(), key, body.Value); err != nil {
		logErr("setSetting", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "saved"})
}

// validateSetting rejects values for keys that must stay well-formed because the
// server reads them at startup (Deployment settings). A bad port or toggle could
// otherwise prevent the app from binding on the next restart.
func validateSetting(key, value string) error {
	switch key {
	case "deploy_port":
		n, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || n < 1 || n > 65535 {
			return errBadSetting("port must be a number between 1 and 65535")
		}
	case "deploy_network":
		if value != "on" && value != "off" {
			return errBadSetting("network must be 'on' or 'off'")
		}
	case "clipboard_clear_seconds":
		n, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || n < 0 || n > 3600 {
			return errBadSetting("clipboard clear must be between 0 and 3600 seconds")
		}
	case "breach_monitor":
		if value != "off" && value != "daily" && value != "weekly" {
			return errBadSetting("breach monitor must be 'off', 'daily', or 'weekly'")
		}
	}
	return nil
}

type errBadSetting string

func (e errBadSetting) Error() string { return string(e) }

// purgeArabic handles POST /settings/purge-arabic: permanently clear every
// Arabic display name (categories, seats, admins). Used when an admin turns off
// Arabic support and explicitly opts to delete the Arabic data.
func (s *Server) purgeArabic(w http.ResponseWriter, r *http.Request) {
	if err := s.Categories.PurgeArabicNames(r.Context()); err != nil {
		logErr("purgeArabic", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "purged"})
}
