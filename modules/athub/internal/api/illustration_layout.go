package api

import (
	"context"
	"math"
	"net/http"

	"github.com/AlbaTextus/atnexus/modules/athub/internal/storage"
)

type illustrationLayoutStore interface {
	GetIllustrationLayout(context.Context, int32) (storage.IllustrationLayout, error)
	SaveIllustrationLayout(context.Context, int32, storage.IllustrationLayout) (storage.IllustrationLayout, error)
}

func (a *API) illustrationLayout(w http.ResponseWriter, r *http.Request) error {
	if r.Method != http.MethodGet && r.Method != http.MethodPut {
		w.Header().Set("Allow", "GET, PUT")
		return &requestError{http.StatusMethodNotAllowed, "method not allowed"}
	}
	query, err := parseQuery(r.URL.RawQuery, "novelId")
	if err != nil {
		return err
	}
	id, err := positiveID(query, "novelId", true)
	if err != nil {
		return err
	}
	st, ok := a.store.(illustrationLayoutStore)
	if !ok {
		return &requestError{http.StatusServiceUnavailable, "illustration layout unavailable"}
	}
	if r.Method == http.MethodGet {
		value, err := st.GetIllustrationLayout(r.Context(), id)
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusOK, value)
	}
	var input storage.IllustrationLayout
	if err := decodeJSON(w, r, &input); err != nil {
		return err
	}
	if err := validateIllustrationLayout(input, id); err != nil {
		return err
	}
	value, err := st.SaveIllustrationLayout(r.Context(), id, input)
	if err != nil {
		return err
	}
	return writeJSON(w, http.StatusOK, value)
}

func validateIllustrationLayout(input storage.IllustrationLayout, novelID int32) error {
	if input.NovelID != novelID || input.Revision < 0 || input.Blocks == nil || len(input.Blocks) > 500 {
		return badRequest("invalid illustration layout")
	}
	seen := make(map[int32]bool)
	for _, block := range input.Blocks {
		if block.ImageID <= 0 || seen[block.ImageID] {
			return badRequest("duplicate or invalid illustration")
		}
		seen[block.ImageID] = true
		for _, value := range []float64{block.X, block.Y, block.Width, block.Height} {
			if math.IsInf(value, 0) || math.IsNaN(value) {
				return badRequest("invalid illustration coordinates")
			}
		}
		// The longer side sets the minimum size so narrow images can keep
		// their original aspect ratio without padding inside the selection.
		if block.X < 0 || block.Y < 0 || block.Width <= 0 || block.Height <= 0 || math.Max(block.Width, block.Height) < 40 ||
			block.X+block.Width > 1000.001 || block.Y+block.Height > 5000.001 {
			return badRequest("illustration outside canvas")
		}
	}
	return nil
}
