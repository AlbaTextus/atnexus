package api

import (
	"net/http"
	"net/url"

	"github.com/AlbaTextus/atnexus/modules/athub/internal/storage"
)

func (a *API) volumes(w http.ResponseWriter, r *http.Request) error {
	query, err := parseQuery(r.URL.RawQuery, "novelId", "order", "limit", "offset")
	if err != nil {
		return err
	}
	switch r.Method {
	case http.MethodGet:
		novelID, err := positiveID(query, "novelId", true)
		if err != nil {
			return err
		}
		if query.Has("order") {
			_, order, err := novelOrder(query)
			if err != nil {
				return err
			}
			item, err := a.store.GetVolume(r.Context(), novelID, order)
			if err != nil {
				return err
			}
			return writeJSON(w, http.StatusOK, item)
		}
		limit, offset, err := pagination(query)
		if err != nil {
			return err
		}
		items, err := a.store.ListVolumes(r.Context(), novelID, limit, offset)
		if err != nil {
			return err
		}
		if items == nil {
			items = []storage.Volume{}
		}
		return writeJSON(w, http.StatusOK, items)
	case http.MethodPut:
		if len(query) != 0 {
			return badRequest("PUT /api/v2/volumes does not accept query parameters")
		}
		var input storage.VolumeInput
		if err := decodeJSON(w, r, &input); err != nil {
			return err
		}
		if input.NovelID <= 0 || input.Order <= 0 {
			return badRequest("novelId and order must be positive 32-bit integers")
		}
		if err := validateTitle(&input.Title); err != nil {
			return err
		}
		item, err := a.store.SaveVolume(r.Context(), input)
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusOK, item)
	case http.MethodDelete:
		novelID, order, err := novelOrder(query)
		if err != nil {
			return err
		}
		if err := a.store.DeleteVolume(r.Context(), novelID, order); err != nil {
			return err
		}
		w.WriteHeader(http.StatusNoContent)
		return nil
	default:
		w.Header().Set("Allow", "GET, PUT, DELETE")
		return &requestError{http.StatusMethodNotAllowed, "method not allowed"}
	}
}

func novelOrder(query url.Values) (int32, int32, error) {
	if len(query) != 2 || !query.Has("novelId") || !query.Has("order") {
		return 0, 0, badRequest("only novelId and order query parameters are required")
	}
	novelID, err := positiveID(query, "novelId", true)
	if err != nil {
		return 0, 0, err
	}
	order, err := positiveID(query, "order", true)
	return novelID, order, err
}
