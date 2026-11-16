package httpapi

import (
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/AlbaTextus/atnexus/internal/store"
)

func (a *API) updateBio(w http.ResponseWriter, r *http.Request) {
	user, ok := a.AccountActor(w, r)
	if !ok {
		return
	}
	var input struct {
		Bio *string `json:"bio"`
	}
	if !readAccountJSON(w, r, &input) {
		return
	}
	if input.Bio == nil || !utf8.ValidString(*input.Bio) || strings.ContainsRune(*input.Bio, 0) || utf8.RuneCountInString(*input.Bio) > 3000 {
		writeJSON(w, 400, map[string]string{"error": "invalid_bio", "error_description": "简介最多 3000 个字符，请检查内容。"})
		return
	}
	st, ok := a.Store.(store.BioStore)
	if !ok {
		writeJSON(w, 503, map[string]string{"error": "service_unavailable"})
		return
	}
	updated, err := st.UpdateBio(r.Context(), user.ID, strings.TrimSpace(*input.Bio))
	if err != nil {
		a.ApplicationError(w, err)
		return
	}
	writeJSON(w, 200, userView(updated))
}
