package api

import (
	"net/http"

	"albatextus/athub/internal/storage"
)

func (a *API) toc(w http.ResponseWriter, r *http.Request) error {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		return &requestError{http.StatusMethodNotAllowed, "method not allowed"}
	}
	query, err := parseQuery(r.URL.RawQuery, "novelId")
	if err != nil {
		return err
	}
	novelID, err := positiveID(query, "novelId", true)
	if err != nil {
		return err
	}
	toc, err := a.store.GetTOC(r.Context(), novelID)
	if err != nil {
		return err
	}
	if toc.Volumes == nil {
		toc.Volumes = []storage.Volume{}
	}
	if toc.Chapters == nil {
		toc.Chapters = []storage.ChapterSummary{}
	}
	return writeJSON(w, http.StatusOK, toc)
}
