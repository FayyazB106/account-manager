package api

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/FayyazB106/account-manager/internal/auth"
	"github.com/FayyazB106/account-manager/internal/store"
)

// Session model (Option C): a short-lived access token (JWT, kept in memory by
// the SPA and sent as a Bearer header) plus a long-lived refresh token stored in
// an httpOnly, SameSite=Strict cookie the JS can't read. The client silently
// exchanges the cookie for a fresh access token via POST /refresh, so there's no
// hard logout while active and no long-lived secret sits in web storage.

const refreshCookieName = "am_refresh"

// refreshTTL is how long a refresh token (and thus an idle session that isn't
// used) stays valid. Rotated on every use.
const refreshTTL = 14 * 24 * time.Hour

// isHTTPS reports whether the request arrived over TLS — directly or via a
// TLS-terminating reverse proxy. Secure cookies are only set when true (so
// localhost/personal HTTP mode still works).
func isHTTPS(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func (s *Server) setRefreshCookie(w http.ResponseWriter, r *http.Request, value string) {
	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookieName,
		Value:    value,
		Path:     "/api",
		HttpOnly: true,
		Secure:   isHTTPS(r),
		SameSite: http.SameSiteStrictMode,
		Expires:  time.Now().Add(refreshTTL),
		MaxAge:   int(refreshTTL.Seconds()),
	})
}

func clearRefreshCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookieName,
		Value:    "",
		Path:     "/api",
		HttpOnly: true,
		Secure:   isHTTPS(r),
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})
}

func refreshCookie(r *http.Request) string {
	c, err := r.Cookie(refreshCookieName)
	if err != nil {
		return ""
	}
	return c.Value
}

// sameOrigin is a CSRF guard for the cookie-authenticated endpoints (/refresh,
// /logout). The refresh cookie is SameSite=Strict (so it isn't sent cross-site
// at all); this is belt-and-suspenders: if an Origin header is present it must
// match the request host. Header-authenticated (Bearer) endpoints don't need
// this — a browser can't attach the Authorization header cross-origin.
func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true // non-browser or same-origin request without an Origin header
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	host := r.Host
	if xf := r.Header.Get("X-Forwarded-Host"); xf != "" {
		host = xf
	}
	return u.Host == host
}

// issueSession mints an access token and a rotating refresh cookie for a fully
// authenticated admin, and writes {token} for the client to hold in memory.
func (s *Server) issueSession(w http.ResponseWriter, r *http.Request, adminID string) {
	access, err := auth.IssueToken(adminID, s.JWTSecret)
	if err != nil {
		logErr("issueSession: access token", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	cookie, err := s.Refresh.Issue(r.Context(), adminID, refreshTTL)
	if err != nil {
		logErr("issueSession: refresh token", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	s.setRefreshCookie(w, r, cookie)
	writeJSON(w, http.StatusOK, map[string]string{"token": access})
}

// refresh handles POST /refresh: exchange the refresh cookie for a new access
// token, rotating the cookie. Used silently by the client on startup and when an
// access token expires.
func (s *Server) refresh(w http.ResponseWriter, r *http.Request) {
	if !sameOrigin(r) {
		writeError(w, http.StatusForbidden, "cross-origin request rejected")
		return
	}
	cookie := refreshCookie(r)
	if cookie == "" {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	adminID, newCookie, err := s.Refresh.Rotate(r.Context(), cookie, refreshTTL)
	if err != nil {
		clearRefreshCookie(w, r)
		if errors.Is(err, store.ErrRefreshReuse) {
			// A spent token was replayed — the family was just revoked.
			s.auditRaw(r, "", "auth.refresh", "", "reuse detected", "fail")
			writeError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		if errors.Is(err, store.ErrRefreshInvalid) {
			writeError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		logErr("refresh: rotate", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	access, err := auth.IssueToken(adminID, s.JWTSecret)
	if err != nil {
		logErr("refresh: access token", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	s.setRefreshCookie(w, r, newCookie)
	writeJSON(w, http.StatusOK, map[string]string{"token": access})
}

// logout handles POST /logout: revoke this session's refresh token and clear the
// cookie. Idempotent — a missing/invalid cookie still returns ok.
func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if !sameOrigin(r) {
		writeError(w, http.StatusForbidden, "cross-origin request rejected")
		return
	}
	if cookie := refreshCookie(r); cookie != "" {
		if err := s.Refresh.Revoke(r.Context(), cookie); err != nil {
			logErr("logout: revoke", err)
		}
	}
	clearRefreshCookie(w, r)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
