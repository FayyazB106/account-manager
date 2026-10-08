package api

import (
	"net/http"
	"os"
)

// listBackups handles GET /backups — stored backup files, newest first.
func (s *Server) listBackups(w http.ResponseWriter, r *http.Request) {
	list, err := s.Backups.List()
	if err != nil {
		logErr("listBackups", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if list == nil {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// createBackup handles POST /backups — make a backup now.
func (s *Server) createBackup(w http.ResponseWriter, r *http.Request) {
	name, err := s.Backups.Create(r.Context())
	if err != nil {
		logErr("createBackup", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"name": name})
}

// downloadBackup handles GET /backups/{name}/download.
func (s *Server) downloadBackup(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	path, err := s.Backups.Path(name)
	if err != nil {
		writeError(w, http.StatusNotFound, "backup not found")
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		logErr("downloadBackup", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+name+"\"")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Write(data)
}

// restoreBackup handles POST /backups/{name}/restore — DESTRUCTIVE.
func (s *Server) restoreBackup(w http.ResponseWriter, r *http.Request) {
	if err := s.Backups.Restore(r.Context(), r.PathValue("name")); err != nil {
		logErr("restoreBackup", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "restored"})
}

// deleteBackup handles DELETE /backups/{name}.
func (s *Server) deleteBackup(w http.ResponseWriter, r *http.Request) {
	if err := s.Backups.Delete(r.PathValue("name")); err != nil {
		writeError(w, http.StatusNotFound, "backup not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
