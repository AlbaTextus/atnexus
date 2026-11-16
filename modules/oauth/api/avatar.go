package httpapi

import (
	"errors"
	"net/http"

	"github.com/AlbaTextus/atnexus/internal/store"
)

func (a *API) updateAvatar(w http.ResponseWriter, r *http.Request) {
	user, ok := a.AccountActor(w, r)
	if !ok {
		return
	}
	var input struct {
		Source string `json:"source"`
	}
	if !readAccountJSON(w, r, &input) {
		return
	}
	if input.Source == "" || len(input.Source) > 40 {
		writeJSON(w, 400, map[string]string{"error": "invalid_avatar_source", "error_description": "请选择头像来源。"})
		return
	}
	st, ok := a.Store.(store.AvatarStore)
	if !ok {
		writeJSON(w, 503, map[string]string{"error": "service_unavailable"})
		return
	}
	updated, err := st.UpdateAvatarSource(r.Context(), user.ID, input.Source)
	if errors.Is(err, store.ErrAvatarUnavailable) {
		writeJSON(w, 400, map[string]string{"error": "avatar_unavailable", "error_description": "该平台尚未绑定或没有提供头像，请重新选择。"})
		return
	}
	if err != nil {
		a.ApplicationError(w, err)
		return
	}
	writeJSON(w, 200, userView(updated))
}
