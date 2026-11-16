package api

import (
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/AlbaTextus/atnexus/modules/athub/internal/storage"
)

func (a *API) platforms(w http.ResponseWriter, r *http.Request) error {
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
			item, err := a.store.GetPlatform(r.Context(), id)
			if err != nil {
				return err
			}
			return writeJSON(w, http.StatusOK, item)
		}
		limit, offset, err := pagination(query)
		if err != nil {
			return err
		}
		items, err := a.store.ListPlatforms(r.Context(), limit, offset)
		if err != nil {
			return err
		}
		if items == nil {
			items = []storage.Platform{}
		}
		return writeJSON(w, http.StatusOK, items)
	case http.MethodPost:
		if len(query) != 0 {
			return badRequest("POST /api/v2/platforms does not accept query parameters")
		}
		var input storage.PlatformInput
		if err := decodeJSON(w, r, &input); err != nil {
			return err
		}
		input.Name = strings.TrimSpace(input.Name)
		if input.Name == "" || strings.ContainsRune(input.Name, 0) || (input.URL != nil && strings.ContainsRune(*input.URL, 0)) {
			return badRequest("name is required and text fields cannot contain NUL characters")
		}
		item, err := a.store.CreatePlatform(r.Context(), input)
		if err != nil {
			return err
		}
		w.Header().Set("Location", "/api/v2/platforms?id="+strconv.FormatInt(int64(item.ID), 10))
		return writeJSON(w, http.StatusCreated, item)
	default:
		w.Header().Set("Allow", "GET, POST")
		return &requestError{http.StatusMethodNotAllowed, "method not allowed"}
	}
}

func (a *API) authors(w http.ResponseWriter, r *http.Request) error {
	if handled, err := a.catalogView(w, r); handled {
		return err
	}
	query, err := parseQuery(r.URL.RawQuery, "id", "platformId", "limit", "offset")
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
			item, err := a.store.GetAuthor(r.Context(), id)
			if err != nil {
				return err
			}
			return writeJSON(w, http.StatusOK, item)
		}
		platformID, err := positiveID(query, "platformId", false)
		if err != nil {
			return err
		}
		limit, offset, err := pagination(query)
		if err != nil {
			return err
		}
		items, err := a.store.ListAuthors(r.Context(), platformID, limit, offset)
		if err != nil {
			return err
		}
		if items == nil {
			items = []storage.Author{}
		}
		return writeJSON(w, http.StatusOK, items)
	case http.MethodPost:
		if len(query) != 0 {
			return badRequest("POST /api/v2/authors does not accept query parameters")
		}
		var input storage.AuthorInput
		if err := decodeJSON(w, r, &input); err != nil {
			return err
		}
		if input.PlatformID <= 0 {
			return badRequest("platformId must be a positive 32-bit integer")
		}
		if err := normalizeAuthorName(&input.Name); err != nil {
			return err
		}
		item, err := a.store.CreateAuthor(r.Context(), input)
		if err != nil {
			return err
		}
		w.Header().Set("Location", "/api/v2/authors?id="+strconv.FormatInt(int64(item.ID), 10))
		return writeJSON(w, http.StatusCreated, item)
	case http.MethodPut:
		id, err := onlyID(query)
		if err != nil {
			return err
		}
		var input struct {
			Name *string `json:"name"`
		}
		if err := decodeJSON(w, r, &input); err != nil {
			return err
		}
		if err := normalizeAuthorName(&input.Name); err != nil {
			return err
		}
		item, err := a.store.UpdateAuthorName(r.Context(), id, input.Name)
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusOK, item)
	default:
		w.Header().Set("Allow", "GET, POST, PUT")
		return &requestError{http.StatusMethodNotAllowed, "method not allowed"}
	}
}

func normalizeAuthorName(name **string) error {
	if *name == nil {
		return nil
	}
	value := strings.TrimSpace(**name)
	if strings.ContainsRune(value, 0) || utf8.RuneCountInString(value) > 200 {
		return badRequest("author name must be at most 200 characters and cannot contain NUL")
	}
	if value == "" {
		*name = nil
	} else {
		*name = &value
	}
	return nil
}
