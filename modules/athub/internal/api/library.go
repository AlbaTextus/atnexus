package api

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"albatextus/athub/internal/storage"
)

// Opt-in representations preserve the existing array responses for API clients.
func (a *API) catalogView(w http.ResponseWriter, r *http.Request) (bool, error) {
	query := r.URL.Query()
	if !query.Has("view") && !query.Has("ids") {
		return false, nil
	}
	if r.Method != http.MethodGet {
		return false, nil
	}
	switch {
	case query.Get("view") == "count":
		query, err := parseQuery(r.URL.RawQuery, "view")
		if err != nil || len(query) != 1 {
			return true, badRequest("count requires only view=count")
		}
		reader, ok := a.store.(interface {
			CatalogCount(context.Context, string) (int64, error)
		})
		if !ok {
			return true, &requestError{http.StatusServiceUnavailable, "catalog counts unavailable"}
		}
		count, err := reader.CatalogCount(r.Context(), strings.TrimPrefix(r.URL.Path, "/api/v2/"))
		if err != nil {
			return true, err
		}
		return true, writeJSON(w, http.StatusOK, map[string]int64{"total": count})
	case r.URL.Path == "/api/v2/authors" && query.Has("ids"):
		query, err := parseQuery(r.URL.RawQuery, "ids")
		if err != nil {
			return true, err
		}
		parts := strings.Split(query.Get("ids"), ",")
		if len(parts) > 100 {
			return true, badRequest("ids must contain at most 100 author IDs")
		}
		ids := make([]int32, 0, len(parts))
		for _, part := range parts {
			id, err := strconv.ParseInt(part, 10, 32)
			if err != nil || id <= 0 {
				return true, badRequest("ids must contain positive 32-bit integers")
			}
			ids = append(ids, int32(id))
		}
		reader, ok := a.store.(interface {
			AuthorsByIDs(context.Context, []int32) ([]storage.Author, error)
		})
		if !ok {
			return true, &requestError{http.StatusServiceUnavailable, "author lookup unavailable"}
		}
		authors, err := reader.AuthorsByIDs(r.Context(), ids)
		if err != nil {
			return true, err
		}
		return true, writeJSON(w, http.StatusOK, authors)
	case r.URL.Path == "/api/v2/novelinfo" && query.Get("view") == "page":
		query, err := parseQuery(r.URL.RawQuery, "view", "platformId", "authorId", "tagId", "q", "sort", "limit", "offset")
		if err != nil {
			return true, err
		}
		var filter storage.NovelPageFilter
		for _, field := range []struct {
			name string
			dest *int32
		}{{"platformId", &filter.PlatformID}, {"authorId", &filter.AuthorID}, {"tagId", &filter.TagID}} {
			*field.dest, err = positiveID(query, field.name, false)
			if err != nil {
				return true, err
			}
		}
		filter.Limit, filter.Offset, err = pagination(query)
		if err != nil {
			return true, err
		}
		filter.Search = strings.TrimSpace(query.Get("q"))
		filter.Sort = query.Get("sort")
		if utf8.RuneCountInString(filter.Search) > 200 || strings.ContainsRune(filter.Search, 0) {
			return true, badRequest("q must be at most 200 characters and cannot contain NUL")
		}
		if filter.Sort != "" && filter.Sort != "updated" && filter.Sort != "title" {
			return true, badRequest("sort must be updated or title")
		}
		reader, ok := a.store.(interface {
			ListNovelPage(context.Context, storage.NovelPageFilter) (storage.NovelPage, error)
		})
		if !ok {
			return true, &requestError{http.StatusServiceUnavailable, "novel pagination unavailable"}
		}
		page, err := reader.ListNovelPage(r.Context(), filter)
		if err != nil {
			return true, err
		}
		return true, writeJSON(w, http.StatusOK, page)
	default:
		return true, badRequest("invalid catalogue view")
	}
}
