package api

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/FayyazB106/account-manager/internal/crypto"
	"github.com/FayyazB106/account-manager/internal/store"
)

// maxUploadBytes caps attachment size (10 MiB) to prevent memory abuse.
const maxUploadBytes = 10 << 20

// uploadAttachment handles POST /accounts/{id}/attachments (multipart form).
// The file is read, envelope-encrypted, and stored. Plaintext bytes never touch
// the database.
func (s *Server) uploadAttachment(w http.ResponseWriter, r *http.Request) {
	accountID := r.PathValue("id")

	// Cap the request body, then parse the multipart form.
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes+1024)
	if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
		writeError(w, http.StatusBadRequest, "file too large or invalid form")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing 'file' field")
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		logErr("uploadAttachment: read", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Detect content type from the first 512 bytes (don't trust the client).
	mime := http.DetectContentType(data)

	env, err := crypto.EnvelopeEncrypt(s.MasterKeyValue(), data)
	if err != nil {
		logErr("uploadAttachment: encrypt", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	id, err := s.Attachments.Create(r.Context(), accountID, header.Filename, mime, env.WrappedDEK, env.Ciphertext)
	if err != nil {
		logErr("uploadAttachment: store", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{"id": id, "filename": header.Filename})
}

// listAttachments handles GET /accounts/{id}/attachments — metadata only.
func (s *Server) listAttachments(w http.ResponseWriter, r *http.Request) {
	items, err := s.Attachments.ListByAccount(r.Context(), r.PathValue("id"))
	if err != nil {
		logErr("listAttachments: query", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if items == nil {
		items = []store.AttachmentMeta{}
	}
	writeJSON(w, http.StatusOK, items)
}

// exportAttachments handles GET /accounts/{id}/attachments/export — decrypts all
// of an account's attachments and streams them as a single .zip.
func (s *Server) exportAttachments(w http.ResponseWriter, r *http.Request) {
	blobs, err := s.Attachments.ListBlobsByAccount(r.Context(), r.PathValue("id"))
	if err != nil {
		logErr("exportAttachments: list", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if len(blobs) == 0 {
		writeError(w, http.StatusNotFound, "no attachments to export")
		return
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="attachments.zip"`)
	w.Header().Set("X-Content-Type-Options", "nosniff")

	zw := zip.NewWriter(w)
	defer zw.Close()

	seen := map[string]int{} // de-duplicate identical filenames within the zip
	for _, b := range blobs {
		plaintext, err := crypto.EnvelopeDecrypt(s.MasterKeyValue(), &crypto.Envelope{WrappedDEK: b.WrappedDEK, Ciphertext: b.Ciphertext})
		if err != nil {
			logErr("exportAttachments: decrypt", err)
			continue // skip a bad file rather than aborting the whole zip
		}
		name := uniqueName(b.Filename, seen)
		f, err := zw.Create(name)
		if err != nil {
			logErr("exportAttachments: zip entry", err)
			return
		}
		f.Write(plaintext)
	}
}

// uniqueName appends " (n)" before the extension if a name repeats in the zip.
func uniqueName(name string, seen map[string]int) string {
	if seen[name] == 0 {
		seen[name]++
		return name
	}
	ext := path.Ext(name)
	base := strings.TrimSuffix(name, ext)
	n := seen[name]
	seen[name]++
	return base + " (" + strconv.Itoa(n) + ")" + ext
}

// deleteAttachment handles DELETE /attachments/{id}.
func (s *Server) deleteAttachment(w http.ResponseWriter, r *http.Request) {
	err := s.Attachments.Delete(r.Context(), r.PathValue("id"))
	if err == store.ErrNotFound {
		writeError(w, http.StatusNotFound, "attachment not found")
		return
	}
	if err != nil {
		logErr("deleteAttachment", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// downloadAttachment handles GET /attachments/{id}/download — decrypt and stream
// the original file back.
func (s *Server) downloadAttachment(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	filename, mime, wrappedDEK, ciphertext, err := s.Attachments.GetForDownload(r.Context(), id)
	if err == store.ErrNotFound {
		writeError(w, http.StatusNotFound, "attachment not found")
		return
	}
	if err != nil {
		logErr("downloadAttachment: get", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	plaintext, err := crypto.EnvelopeDecrypt(s.MasterKeyValue(), &crypto.Envelope{
		WrappedDEK: wrappedDEK,
		Ciphertext: ciphertext,
	})
	if err != nil {
		logErr("downloadAttachment: decrypt", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.Header().Set("Content-Type", mime)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	w.Write(plaintext)
}
