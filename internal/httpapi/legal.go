package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/AlbaTextus/connect/internal/store"
	"github.com/go-chi/chi/v5"
)

func (a *API) legalStore(w http.ResponseWriter, r *http.Request) (store.LegalStore, string, bool) {
	w.Header().Set("Cache-Control", "no-store")
	slug := chi.URLParam(r, "slug")
	if slug != "privacy" && slug != "terms" {
		writeJSON(w, 404, map[string]string{"error": "legal_not_found", "error_description": "协议不存在。"})
		return nil, slug, false
	}
	st, ok := a.store.(store.LegalStore)
	if !ok {
		writeJSON(w, 503, map[string]string{"error": "service_unavailable"})
	}
	return st, slug, ok
}

func (a *API) legalAdmin(w http.ResponseWriter, r *http.Request) bool {
	user, ok := a.accountActor(w, r)
	if !ok {
		return false
	}
	if !isAdmin(user) {
		writeJSON(w, 403, map[string]string{"error": "legal_admin_required", "error_description": "协议管理仅限管理员使用。"})
		return false
	}
	return true
}

func (a *API) getAdminLegalDocument(w http.ResponseWriter, r *http.Request) {
	if a.legalAdmin(w, r) {
		a.getLegalDocument(w, r)
	}
}

func (a *API) getLegalDocument(w http.ResponseWriter, r *http.Request) {
	st, slug, ok := a.legalStore(w, r)
	if !ok {
		return
	}
	document, err := st.LegalDocument(r.Context(), slug)
	if err != nil {
		a.legalError(w, err)
		return
	}
	writeJSON(w, 200, document)
}

func (a *API) updateLegalDocument(w http.ResponseWriter, r *http.Request) {
	if !a.legalAdmin(w, r) {
		return
	}
	st, slug, ok := a.legalStore(w, r)
	if !ok {
		return
	}
	var input struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		Content     string `json:"content"`
		Revision    int    `json:"revision"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") || d.Decode(&input) != nil || d.Decode(&struct{}{}) != io.EOF {
		writeJSON(w, 400, map[string]string{"error": "invalid_request", "error_description": "请提交有效的协议内容。"})
		return
	}
	input.Title = strings.TrimSpace(input.Title)
	input.Description = strings.TrimSpace(input.Description)
	message := ""
	switch {
	case input.Title == "" || utf8.RuneCountInString(input.Title) > 100:
		message = "标题需为 1–100 个字符。"
	case utf8.RuneCountInString(input.Description) > 300:
		message = "简介不能超过 300 个字符。"
	case strings.TrimSpace(input.Content) == "" || utf8.RuneCountInString(input.Content) > 100000:
		message = "正文不能为空，且不能超过 100,000 个字符。"
	case strings.ContainsRune(input.Title+input.Description+input.Content, '\x00'):
		message = "协议中含有无效字符。"
	case input.Revision < 1:
		message = "请先加载协议的当前版本。"
	}
	if message != "" {
		writeJSON(w, 400, map[string]string{"error": "invalid_legal_document", "error_description": message})
		return
	}
	document, err := st.UpdateLegalDocument(r.Context(), store.LegalDocument{Slug: slug, Title: input.Title, Description: input.Description, Content: input.Content, Revision: input.Revision})
	if err != nil {
		a.legalError(w, err)
		return
	}
	writeJSON(w, 200, document)
}

func (a *API) legalError(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrConflict) {
		writeJSON(w, 409, map[string]string{"error": "legal_conflict", "error_description": "这份协议已被其他管理员更新。请先复制当前草稿，再重新加载最新版本。"})
		return
	}
	if errors.Is(err, store.ErrNotFound) {
		writeJSON(w, 404, map[string]string{"error": "legal_not_found", "error_description": "协议不存在。"})
		return
	}
	a.log.Error("legal document operation failed", "error", err)
	writeJSON(w, 500, map[string]string{"error": "server_error"})
}
