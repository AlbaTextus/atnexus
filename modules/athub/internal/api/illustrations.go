package api

import (
	"context"
	"encoding/hex"
	"net/http"
	"net/url"
	"strings"

	"github.com/AlbaTextus/atnexus/modules/athub/internal/storage"
)

type illustrationStore interface {
	ListIllustrations(context.Context, int32, int32, int32) ([]storage.Illustration, error)
	AddIllustration(context.Context, int32, storage.IllustrationInput) (storage.Illustration, error)
}

type illustrationDeleter interface {
	DeleteIllustration(context.Context, int32, int32) error
}

func (a *API) illustrations(w http.ResponseWriter, r *http.Request) error {
	if r.Method != http.MethodGet && r.Method != http.MethodPost && r.Method != http.MethodDelete {
		w.Header().Set("Allow", "GET, POST, DELETE")
		return &requestError{http.StatusMethodNotAllowed, "method not allowed"}
	}
	query, err := parseQuery(r.URL.RawQuery, "novelId", "limit", "offset", "id")
	if err != nil {
		return err
	}
	novelID, err := positiveID(query, "novelId", true)
	if err != nil {
		return err
	}
	if r.Method == http.MethodDelete {
		if len(query) != 2 {
			return badRequest("DELETE requires only novelId and id")
		}
		imageID, err := positiveID(query, "id", true)
		if err != nil {
			return err
		}
		st, ok := a.store.(illustrationDeleter)
		if !ok {
			return &requestError{http.StatusServiceUnavailable, "illustrations unavailable"}
		}
		if err := st.DeleteIllustration(r.Context(), novelID, imageID); err != nil {
			return err
		}
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
	if query.Has("id") {
		return badRequest("id is only supported for DELETE")
	}
	st, ok := a.store.(illustrationStore)
	if !ok {
		return &requestError{http.StatusServiceUnavailable, "illustrations unavailable"}
	}
	if r.Method == http.MethodGet {
		limit, offset, err := pagination(query)
		if err != nil {
			return err
		}
		items, err := st.ListIllustrations(r.Context(), novelID, limit, offset)
		if err != nil {
			return err
		}
		if items == nil {
			items = []storage.Illustration{}
		}
		return writeJSON(w, http.StatusOK, items)
	}
	if len(query) != 1 {
		return badRequest("POST requires only novelId")
	}
	var input storage.IllustrationInput
	if err := decodeJSON(w, r, &input); err != nil {
		return err
	}
	if err := validateIllustration(input); err != nil {
		return err
	}
	item, err := st.AddIllustration(r.Context(), novelID, input)
	if err != nil {
		return err
	}
	return writeJSON(w, http.StatusCreated, item)
}

func validateIllustration(input storage.IllustrationInput) error {
	target, err := url.Parse(input.URL)
	digest, digestErr := hex.DecodeString(input.SHA256)
	if err != nil || target == nil || (target.Scheme != "https" && target.Scheme != "http") ||
		target.Hostname() == "" || target.User != nil || target.Fragment != "" || len(input.URL) > 2048 ||
		strings.ContainsAny(input.URL, "\x00\r\n") || digestErr != nil || len(digest) != 32 ||
		input.SHA256 != strings.ToLower(input.SHA256) ||
		input.Width <= 0 || input.Height <= 0 || input.Width > 4096 || input.Height > 4096 ||
		int64(input.Width)*int64(input.Height) > 16_000_000 {
		return badRequest("invalid illustration URL, checksum or dimensions")
	}
	return nil
}
