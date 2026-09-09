package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/AlbaTextus/connect/internal/email"
	"github.com/AlbaTextus/connect/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (a *API) communicationAdmin(w http.ResponseWriter, r *http.Request) (store.User, store.CommunicationStore, bool) {
	user, ok := a.accountActor(w, r)
	if !ok {
		return user, nil, false
	}
	if !isAdmin(user) {
		writeJSON(w, 403, map[string]string{"error": "admin_required", "error_description": "消息管理仅限管理员使用。"})
		return user, nil, false
	}
	st, ok := a.store.(store.CommunicationStore)
	if !ok {
		writeJSON(w, 503, map[string]string{"error": "service_unavailable"})
	}
	return user, st, ok
}

func (a *API) previewCommunication(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := a.communicationAdmin(w, r); !ok {
		return
	}
	var in struct {
		Title   string `json:"title"`
		Content string `json:"content"`
		Channel string `json:"channel"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") || decoder.Decode(&in) != nil || decoder.Decode(&struct{}{}) != io.EOF || utf8.RuneCountInString(in.Title) > 100 || utf8.RuneCountInString(in.Content) > 10000 || strings.ContainsAny(in.Title, "\r\n\x00") || strings.ContainsRune(in.Content, '\x00') || (in.Channel != "" && in.Channel != "email" && in.Channel != "announcement") {
		writeJSON(w, 400, map[string]string{"error": "invalid_request", "error_description": "请填写有效的消息内容。"})
		return
	}
	if strings.TrimSpace(in.Title) == "" {
		in.Title = "你的消息标题"
	}
	if strings.TrimSpace(in.Content) == "" {
		in.Content = "从一句问候开始。\n写下你的消息，它会在这里呈现。"
	}
	render := email.RenderMessage
	if in.Channel == "announcement" {
		render = email.RenderAnnouncement
	}
	html, err := render(in.Title, in.Content)
	if err != nil {
		a.communicationError(w, err)
		return
	}
	writeJSON(w, 200, map[string]string{"html": html})
}

func (a *API) publicAnnouncements(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	st, ok := a.store.(store.CommunicationStore)
	if !ok {
		writeJSON(w, 503, map[string]string{"error": "service_unavailable"})
		return
	}
	items, err := st.ListCommunications(r.Context(), true)
	if err != nil {
		a.communicationError(w, err)
		return
	}
	viewer := "guest"
	if user, ok := a.currentUser(r); ok {
		viewer = user.ID.String()
	}
	public := make([]map[string]any, 0, len(items))
	for _, item := range items {
		public = append(public, map[string]any{"id": item.ID, "title": item.Title, "content": item.Content, "revision": item.Revision, "published_at": item.PublishedAt, "updated_at": item.UpdatedAt})
	}
	writeJSON(w, 200, map[string]any{"items": public, "viewer": viewer})
}

func (a *API) publicAnnouncement(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	st, ok := a.store.(store.CommunicationStore)
	if !ok {
		writeJSON(w, 503, map[string]string{"error": "service_unavailable"})
		return
	}
	item, err := st.PublicAnnouncement(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeJSON(w, 404, map[string]string{"error": "announcement_not_found", "error_description": "公告已撤下或不存在。"})
		return
	}
	if err != nil {
		a.communicationError(w, err)
		return
	}
	html, err := email.RenderAnnouncement(item.Title, item.Content)
	if err != nil {
		a.communicationError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"html": html, "revision": item.Revision})
}

func (a *API) listCommunications(w http.ResponseWriter, r *http.Request) {
	actor, st, ok := a.communicationAdmin(w, r)
	if !ok {
		return
	}
	items, err := st.ListCommunications(r.Context(), false)
	if err != nil {
		a.communicationError(w, err)
		return
	}
	audience, err := st.CommunicationAudience(r.Context(), actor.ID)
	if err != nil {
		a.communicationError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"items": items, "audience": audience, "smtp_ready": a.cfg.SMTP.Enabled()})
}

type communicationInput struct {
	Kind     string `json:"kind"`
	Title    string `json:"title"`
	Content  string `json:"content"`
	Audience string `json:"audience"`
	Status   string `json:"status"`
	Revision int    `json:"revision"`
}

func validCommunication(in communicationInput) bool {
	return utf8.RuneCountInString(in.Title) > 0 && utf8.RuneCountInString(in.Title) <= 100 && !strings.ContainsAny(in.Title, "\r\n\x00") && strings.TrimSpace(in.Content) != "" && utf8.RuneCountInString(in.Content) <= 10000 && !strings.ContainsRune(in.Content, '\x00') &&
		((in.Kind == "announcement" && in.Audience == "all" && (in.Status == "draft" || in.Status == "published")) || ((in.Kind == "email" || in.Kind == "both") && in.Status == "draft" && (in.Audience == "all" || in.Audience == "admins" || in.Audience == "self")))
}

func (a *API) saveCommunication(w http.ResponseWriter, r *http.Request) {
	actor, st, ok := a.communicationAdmin(w, r)
	if !ok {
		return
	}
	var in communicationInput
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") || d.Decode(&in) != nil || d.Decode(&struct{}{}) != io.EOF {
		writeJSON(w, 400, map[string]string{"error": "invalid_request"})
		return
	}
	in.Title = strings.TrimSpace(in.Title)
	in.Content = strings.TrimSpace(in.Content)
	if !validCommunication(in) {
		writeJSON(w, 400, map[string]string{"error": "invalid_communication", "error_description": "请填写 1–100 字标题、1–10,000 字正文，并选择有效的发送范围和状态。"})
		return
	}
	id := uuid.Nil
	if raw := chi.URLParam(r, "id"); raw != "" {
		var err error
		id, err = uuid.Parse(raw)
		if err != nil || in.Revision < 1 {
			writeJSON(w, 400, map[string]string{"error": "invalid_revision"})
			return
		}
	}
	item, err := st.SaveCommunication(r.Context(), store.Communication{ID: id, Kind: in.Kind, Title: in.Title, Content: in.Content, Audience: in.Audience, Status: in.Status, Revision: in.Revision}, actor.ID)
	if err != nil {
		a.communicationError(w, err)
		return
	}
	writeJSON(w, 200, item)
}

func (a *API) sendCommunication(w http.ResponseWriter, r *http.Request) {
	actor, st, ok := a.communicationAdmin(w, r)
	if !ok {
		return
	}
	if !a.cfg.SMTP.Enabled() {
		writeJSON(w, 503, map[string]string{"error": "email_unavailable", "error_description": "请先配置服务器 SMTP 邮件服务。"})
		return
	}
	var in struct {
		Revision int `json:"revision"`
	}
	if !readAccountJSON(w, r, &in) {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil || in.Revision < 1 {
		writeJSON(w, 400, map[string]string{"error": "invalid_request"})
		return
	}
	if err := st.QueueCommunication(r.Context(), id, in.Revision, actor.ID); err != nil {
		a.communicationError(w, err)
		return
	}
	writeJSON(w, 202, map[string]string{"status": "queued"})
}

func (a *API) archiveCommunication(w http.ResponseWriter, r *http.Request) {
	_, st, ok := a.communicationAdmin(w, r)
	if !ok {
		return
	}
	var in struct {
		Revision int `json:"revision"`
	}
	if !readAccountJSON(w, r, &in) {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil || in.Revision < 1 {
		writeJSON(w, 400, map[string]string{"error": "invalid_request"})
		return
	}
	if err := st.ArchiveCommunication(r.Context(), id, in.Revision); err != nil {
		a.communicationError(w, err)
		return
	}
	writeJSON(w, 200, map[string]string{"status": "archived"})
}

func (a *API) communicationError(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrConflict) {
		writeJSON(w, 409, map[string]string{"error": "conflict", "error_description": "内容已被更新或邮件已发送，请刷新后重试。"})
		return
	}
	if errors.Is(err, store.ErrNotFound) {
		writeJSON(w, 404, map[string]string{"error": "not_found", "error_description": "记录不存在，或没有符合条件的收件人。"})
		return
	}
	a.log.Error("communication operation failed", "error", err)
	writeJSON(w, 500, map[string]string{"error": "server_error", "error_description": "消息服务暂时不可用，请稍后重试。"})
}
