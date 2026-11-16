package api

import (
	"fmt"
	"net/http"
	"strconv"

	"albatextus/athub/internal/storage"
)

const (
	MaxUploadVolumes  = 200
	MaxUploadChapters = 1000
)

func (a *API) upload(w http.ResponseWriter, r *http.Request) error {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		return &requestError{http.StatusMethodNotAllowed, "method not allowed"}
	}
	if _, err := parseQuery(r.URL.RawQuery); err != nil {
		return err
	}
	var input storage.UploadInput
	if err := decodeJSON(w, r, &input); err != nil {
		return err
	}
	if err := validateUpload(&input); err != nil {
		return err
	}
	result, err := a.store.Upload(r.Context(), input)
	if err != nil {
		return err
	}
	status := http.StatusOK
	if input.NovelID == nil {
		status = http.StatusCreated
		w.Header().Set("Location", "/api/v2/novelinfo?id="+strconv.FormatInt(int64(result.Novel.ID), 10))
	}
	return writeJSON(w, status, result)
}

func validateUpload(input *storage.UploadInput) error {
	if (input.NovelID == nil) == (input.Novel == nil) {
		return badRequest("provide exactly one of novel or novelId")
	}
	if input.NovelID != nil && *input.NovelID <= 0 {
		return badRequest("novelId must be a positive 32-bit integer")
	}
	if err := normalizeAuthorName(&input.AuthorName); err != nil {
		return err
	}
	if input.AuthorName != nil && (input.Novel == nil || input.Novel.AuthorID != nil) {
		return badRequest("authorName requires a new novel without authorId")
	}
	if input.AuthorName != nil && input.Novel.AuthorName != nil {
		return badRequest("provide authorName at the upload or novel level, not both")
	}
	if input.Novel != nil {
		if err := validateNovel(input.Novel); err != nil {
			return badRequest("novel: " + err.Error())
		}
	}
	if len(input.Volumes) > MaxUploadVolumes || len(input.Chapters) > MaxUploadChapters {
		return badRequest("an upload allows at most 200 volumes and 1000 chapters; split larger uploads into batches")
	}
	if input.NovelID != nil && len(input.Volumes) == 0 && len(input.Chapters) == 0 {
		return badRequest("an existing-novel upload requires at least one volume or chapter")
	}
	orders := make(map[int32]bool)
	for i := range input.Volumes {
		v := &input.Volumes[i]
		if v.Order <= 0 || orders[v.Order] {
			return badRequest(fmt.Sprintf("volumes[%d].order must be positive and unique within the batch", i))
		}
		orders[v.Order] = true
		if err := validateTitle(&v.Title); err != nil {
			return badRequest(fmt.Sprintf("volumes[%d]: %s", i, err))
		}
	}
	orders = make(map[int32]bool)
	for i := range input.Chapters {
		c := &input.Chapters[i]
		if c.Order <= 0 || orders[c.Order] {
			return badRequest(fmt.Sprintf("chapters[%d].order must be positive and unique within the batch", i))
		}
		orders[c.Order] = true
		if c.VolumeOrder != nil && *c.VolumeOrder <= 0 {
			return badRequest(fmt.Sprintf("chapters[%d].volumeOrder must be a positive 32-bit integer or null", i))
		}
		if err := validateChapterText(&c.Title, c.Content); err != nil {
			return badRequest(fmt.Sprintf("chapters[%d]: %s", i, err))
		}
	}
	return nil
}
