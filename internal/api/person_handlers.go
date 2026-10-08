package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/FayyazB106/account-manager/internal/store"
)

func (s *Server) listPeople(w http.ResponseWriter, r *http.Request) {
	people, err := s.People.List(r.Context())
	if err != nil {
		logErr("listPeople", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if people == nil {
		people = []store.Person{}
	}
	writeJSON(w, http.StatusOK, people)
}

type personInput struct {
	Name   string  `json:"name"`
	NameAr *string `json:"name_ar"`
}

func (s *Server) createPerson(w http.ResponseWriter, r *http.Request) {
	var in personInput
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	in.NameAr = cleanOptAr(in.NameAr)
	if in.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	id, err := s.People.Create(r.Context(), in.Name, in.NameAr)
	if err != nil {
		logErr("createPerson", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, store.Person{ID: id, Name: in.Name, NameAr: in.NameAr})
}

func (s *Server) updatePerson(w http.ResponseWriter, r *http.Request) {
	var in personInput
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	in.NameAr = cleanOptAr(in.NameAr)
	if in.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if err := s.People.Update(r.Context(), r.PathValue("id"), in.Name, in.NameAr); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		logErr("updatePerson", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (s *Server) deletePerson(w http.ResponseWriter, r *http.Request) {
	err := s.People.Delete(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	if err != nil {
		logErr("deletePerson", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// deletePeople handles POST /people/delete with body {"ids": [...]} — bulk delete.
func (s *Server) deletePeople(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IDs []string `json:"ids"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	n, err := s.People.DeleteMany(r.Context(), body.IDs)
	if err != nil {
		logErr("deletePeople", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"deleted": n})
}

// importPeople handles POST /people/import with body {"people": [{name, name_ar}]}
// — bulk add. name_ar is optional (only used when Arabic support is on).
func (s *Server) importPeople(w http.ResponseWriter, r *http.Request) {
	var body struct {
		People []struct {
			Name   string  `json:"name"`
			NameAr *string `json:"name_ar"`
		} `json:"people"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	// Clean: trim, drop empty names, normalize blank Arabic to nil.
	var people []store.PersonInput
	for _, p := range body.People {
		name := strings.TrimSpace(p.Name)
		if name == "" {
			continue
		}
		people = append(people, store.PersonInput{Name: name, NameAr: cleanOptAr(p.NameAr)})
	}
	n, err := s.People.CreateMany(r.Context(), people)
	if err != nil {
		logErr("importPeople", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]int{"added": n})
}
