package api

import (
	"net/http"
	"strconv"
	"unicode/utf8"
	"strings"

	"github.com/AlbaTextus/atnexus/modules/athub/internal/storage"
)

func (a *API) tags(w http.ResponseWriter, r *http.Request) error {
	if handled, err := a.catalogView(w, r); handled {
		return err
	}
	query, err := parseQuery(r.URL.RawQuery, "id", "limit", "offset")
	if err != nil {
		return err
	}
	switch r.Method {
	case http.MethodGet:
		if query.Has("id") {
			id, err := onlyID(query)
			if err != nil {
				return err
			}
			tag, err := a.store.GetTag(r.Context(), id)
			if err != nil {
				return err
			}
			return writeJSON(w, http.StatusOK, tag)
		}
		limit, offset, err := pagination(query)
		if err != nil {
			return err
		}
		tags, err := a.store.ListTags(r.Context(), limit, offset)
		if err != nil {
			return err
		}
		if tags == nil {
			tags = []storage.Tag{}
		}
		return writeJSON(w, http.StatusOK, tags)
	case http.MethodPost, http.MethodPut:
		var id int32
		if r.Method == http.MethodPost {
			if len(query) != 0 {
				return badRequest("POST /api/v5/tags does not accept query parameters")
			}
		} else {
			id, err = onlyID(query)
			if err != nil {
				return err
			}
		}
		var input storage.TagInput
		if err := decodeJSON(w, r, &input); err != nil {
			return err
		}
		input.Name = strings.TrimSpace(input.Name)
		if input.Name == "" || strings.ContainsRune(input.Name, 0) {
			return badRequest("name is required and cannot contain NUL characters")
		}
		if utf8.RuneCountInString(input.Name) > maxTagNameRunes {
			return badRequest("tag names can have at most " + strconv.Itoa(maxTagNameRunes) + " characters")
		}
		var tag storage.Tag
		status := http.StatusOK
		if r.Method == http.MethodPost {
			tag, err = a.store.CreateTag(r.Context(), input)
			status = http.StatusCreated
		} else {
			tag, err = a.store.UpdateTag(r.Context(), id, input)
		}
		if err != nil {
			return err
		}
		if status == http.StatusCreated {
			w.Header().Set("Location", "/api/v2/tags?id="+strconv.FormatInt(int64(tag.ID), 10))
		}
		return writeJSON(w, status, tag)
	case http.MethodDelete:
		id, err := onlyID(query)
		if err != nil {
			return err
		}
		if err := a.store.DeleteTag(r.Context(), id); err != nil {
			return err
		}
		w.WriteHeader(http.StatusNoContent)
		return nil
	default:
		w.Header().Set("Allow", "GET, POST, PUT, DELETE")
		return &requestError{http.StatusMethodNotAllowed, "method not allowed"}
	}
}
