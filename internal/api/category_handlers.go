package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/FayyazB106/account-manager/internal/store"
)

type categoryInput struct {
	Name   string  `json:"name"`
	NameAr *string `json:"name_ar"`
	Icon   string  `json:"icon"`
}

func (in *categoryInput) clean() {
	in.Name = strings.TrimSpace(in.Name)
	in.Icon = strings.TrimSpace(in.Icon)
	if in.Icon == "" {
		in.Icon = "FaFolder"
	}
	in.NameAr = cleanOptAr(in.NameAr)
}

// cleanOptAr trims an optional Arabic string, returning nil when it's blank so
// empty values are stored as NULL rather than "".
func cleanOptAr(v *string) *string {
	if v == nil {
		return nil
	}
	t := strings.TrimSpace(*v)
	if t == "" {
		return nil
	}
	return &t
}

func (s *Server) listCategories(w http.ResponseWriter, r *http.Request) {
	cats, err := s.Categories.List(r.Context())
	if err != nil {
		logErr("listCategories", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if cats == nil {
		cats = []store.Category{}
	}
	writeJSON(w, http.StatusOK, cats)
}

func (s *Server) createCategory(w http.ResponseWriter, r *http.Request) {
	var in categoryInput
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	in.clean()
	if in.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	id, err := s.Categories.Create(r.Context(), in.Name, in.NameAr, in.Icon)
	if err != nil {
		if store.IsUniqueViolation(err) {
			writeError(w, http.StatusConflict, "a category with that name already exists")
			return
		}
		logErr("createCategory", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, store.Category{ID: id, Name: in.Name, NameAr: in.NameAr, Icon: in.Icon})
}

func (s *Server) updateCategory(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var in categoryInput
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	in.clean()
	if in.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	if err := s.Categories.Update(r.Context(), id, in.Name, in.NameAr, in.Icon); err != nil {
		switch {
		case errors.Is(err, store.ErrNotFound):
			writeError(w, http.StatusNotFound, "category not found")
		case store.IsUniqueViolation(err):
			writeError(w, http.StatusConflict, "a category with that name already exists")
		default:
			logErr("updateCategory", err)
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// reorderCategories handles POST /categories/reorder with body {"ids": [...]}.
func (s *Server) reorderCategories(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IDs []string `json:"ids"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := s.Categories.Reorder(r.Context(), body.IDs); err != nil {
		logErr("reorderCategories", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) deleteCategory(w http.ResponseWriter, r *http.Request) {
	err := s.Categories.Delete(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "category not found")
		return
	}
	if errors.Is(err, store.ErrCategoryInUse) {
		writeError(w, http.StatusConflict, "cannot delete a category that still has accounts; move or remove them first")
		return
	}
	if err != nil {
		logErr("deleteCategory", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
