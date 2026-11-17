package httpapi

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (a *API) SetATHubNovelEditor(edit func(http.ResponseWriter, *http.Request, uuid.UUID, uuid.UUID, int32)) {
	a.athubNovelEditor = edit
}

func (a *API) athubEditNovel(w http.ResponseWriter, r *http.Request) {
	a.editNovelContent(w, r, a.athubNovelEditor)
}

func (a *API) SetATHubIllustrationLayoutEditor(edit func(http.ResponseWriter, *http.Request, uuid.UUID, uuid.UUID, int32)) {
	a.athubIllustrationLayoutEditor = edit
}

func (a *API) editNovelIllustrationLayout(w http.ResponseWriter, r *http.Request) {
	a.editNovelContent(w, r, a.athubIllustrationLayoutEditor)
}

func (a *API) editNovelContent(w http.ResponseWriter, r *http.Request, edit func(http.ResponseWriter, *http.Request, uuid.UUID, uuid.UUID, int32)) {
	user, session, ok := a.ATHubSessionActor(w, r)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 32)
	if err != nil || id <= 0 || r.URL.RawQuery != "" {
		writeJSON(w, 400, map[string]string{"error": "invalid_novel_id", "error_description": "请选择有效的作品。"})
		return
	}
	if edit == nil {
		writeJSON(w, 503, map[string]string{"error": "editing_unavailable", "error_description": "编辑服务暂时不可用，请稍后重试。"})
		return
	}
	edit(w, r, user.ID, session.ID, int32(id))
}
