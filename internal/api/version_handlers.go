package api

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"runtime"
	"time"

	"github.com/FayyazB106/account-manager/internal/update"
)

// updateRepo is the ONLY GitHub repo the updater ever checks/downloads from. It's
// a compile-time constant on purpose — NEVER read from a DB setting. The apply
// path downloads and executes an installer, so letting an admin (or a restored
// setting) redirect it to an arbitrary repo would be remote code execution; the
// SHA256 check can't prevent that (checksums come from the same chosen release).
const updateRepo = "FayyazB106/account-manager"

// updateDownloadClient allows generous time for a desktop installer/dmg download.
var updateDownloadClient = &http.Client{Timeout: 10 * time.Minute}

// updateStatus resolves the current update status, honoring the `update_check`
// setting (default on). The source repo is a fixed compile-time constant. Errors
// are returned but are non-fatal — callers fall back to "no update".
func (s *Server) updateStatus(ctx context.Context, force bool) (update.Status, error) {
	if s.Updater == nil {
		return update.Status{Current: s.Version, Checked: time.Now()}, nil
	}
	all, _ := s.Settings.All(ctx)
	if all["update_check"] == "off" {
		return update.Status{Current: s.Version, Checked: time.Now()}, nil
	}
	// Source repo is the compile-time constant, never a DB setting (see updateRepo).
	return s.Updater.Check(ctx, updateRepo, s.Version, force)
}

// canApply reports whether an update can be applied in-place here: only on an
// interactive desktop session (-tray) on Windows/macOS. Servers and Linux are
// notify-only (never self-restart a service unattended).
func (s *Server) canApply() bool {
	return s.Updater != nil && s.Desktop && platformAsset() != ""
}

// platformAsset is the release asset name to download for this OS ("" = not
// self-updatable here).
func platformAsset() string {
	switch runtime.GOOS {
	case "windows":
		return "account-manager-setup.exe"
	case "darwin":
		return "account-manager-macos.dmg"
	default:
		return ""
	}
}

type versionResponse struct {
	Current         string    `json:"current"`
	Latest          string    `json:"latest"`
	UpdateAvailable bool      `json:"updateAvailable"`
	URL             string    `json:"url"`
	Notes           string    `json:"notes"`
	Checked         time.Time `json:"checked"`
	CanApply        bool      `json:"canApply"`
	OS              string    `json:"os"` // runtime.GOOS — the client tailors apply messaging
}

func (s *Server) versionResp(st update.Status) versionResponse {
	return versionResponse{
		Current:         s.Version,
		Latest:          st.Latest,
		UpdateAvailable: st.UpdateAvailable,
		URL:             st.URL,
		Notes:           st.Notes,
		Checked:         st.Checked,
		CanApply:        s.canApply() && st.UpdateAvailable,
		OS:              runtime.GOOS,
	}
}

// getVersion (GET /version) returns the current build version and the (cached)
// update status.
func (s *Server) getVersion(w http.ResponseWriter, r *http.Request) {
	st, err := s.updateStatus(r.Context(), false)
	if err != nil {
		logErr("update check", err) // non-fatal
	}
	writeJSON(w, http.StatusOK, s.versionResp(st))
}

// checkUpdate (POST /update/check) forces a fresh check, bypassing the cache.
func (s *Server) checkUpdate(w http.ResponseWriter, r *http.Request) {
	st, err := s.updateStatus(r.Context(), true)
	if err != nil {
		logErr("update check (forced)", err)
	}
	writeJSON(w, http.StatusOK, s.versionResp(st))
}

// applyUpdate (POST /update/apply) downloads + verifies the platform installer and
// launches it. Desktop (Windows/macOS) only; the download is SHA256-verified
// against the release's SHA256SUMS before it is run.
func (s *Server) applyUpdate(w http.ResponseWriter, r *http.Request) {
	if !s.canApply() {
		writeError(w, http.StatusForbidden, "in-place update isn't available here; download the new release manually")
		return
	}
	// Ensure we have a current view of the release (and its assets).
	st, err := s.updateStatus(r.Context(), true)
	if err != nil {
		logErr("apply update: check", err)
		writeError(w, http.StatusBadGateway, "could not reach the update server")
		return
	}
	if !st.UpdateAvailable {
		writeError(w, http.StatusConflict, "already up to date")
		return
	}

	asset := platformAsset()
	path, err := st.DownloadVerified(r.Context(), updateDownloadClient, asset)
	if err != nil {
		logErr("apply update: download/verify", err)
		writeError(w, http.StatusInternalServerError, "update download failed verification")
		return
	}
	if err := launchInstaller(path); err != nil {
		logErr("apply update: launch", err)
		writeError(w, http.StatusInternalServerError, "could not start the installer")
		return
	}
	// The action is recorded by the audit middleware (system.update).
	writeJSON(w, http.StatusOK, map[string]string{"status": "started"})
}

// launchInstaller runs the verified download. On Windows it runs the Inno
// installer **silently** (/SILENT = a progress bar, no wizard pages or clicks);
// the installer stops this process, upgrades in place, and relaunches the app
// (its post-install launch no longer skips on silent installs). On macOS it opens
// the .dmg for the user to drag-replace. Detached so it survives this process
// being replaced/killed.
func launchInstaller(path string) error {
	switch runtime.GOOS {
	case "windows":
		// /SILENT: progress only, no wizard. /SUPPRESSMSGBOXES: no prompts (e.g.
		// "file in use"). /NORESTART: never reboot. Previous task choices (desktop/
		// startup shortcuts) are preserved via Inno's UsePreviousTasks (default on).
		return exec.Command(path, "/SILENT", "/SUPPRESSMSGBOXES", "/NORESTART").Start()
	case "darwin":
		return exec.Command("open", path).Start()
	default:
		return fmt.Errorf("in-place update not supported on %s", runtime.GOOS)
	}
}
