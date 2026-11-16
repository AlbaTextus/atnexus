package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"image"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/AlbaTextus/atnexus/internal/security"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (a *API) SetATHubIllustrationSubmitter(submit func(http.ResponseWriter, *http.Request, uuid.UUID, uuid.UUID, int32)) {
	a.athubIllustrationSubmitter = submit
}

func (a *API) SetATHubIllustrationDeleter(remove func(http.ResponseWriter, *http.Request, uuid.UUID, uuid.UUID, int32, int32)) {
	a.athubIllustrationDeleter = remove
}

func (a *API) deleteNovelIllustration(w http.ResponseWriter, r *http.Request) {
	user, _, ok := a.athubActor(w, r)
	if !ok {
		return
	}
	novelID, novelErr := strconv.ParseInt(chi.URLParam(r, "id"), 10, 32)
	imageID, imageErr := strconv.ParseInt(chi.URLParam(r, "illustrationId"), 10, 32)
	if novelErr != nil || imageErr != nil || novelID <= 0 || imageID <= 0 || r.URL.RawQuery != "" {
		writeJSON(w, 400, map[string]string{"error": "invalid_illustration", "error_description": "请选择有效的作品插图。"})
		return
	}
	cookie, err := r.Cookie(a.cfg.SessionCookieName)
	if err != nil {
		writeJSON(w, 401, map[string]string{"error": "login_required"})
		return
	}
	session, err := a.store.SessionByHash(r.Context(), security.HashToken(cookie.Value))
	if err != nil || session.UserID != user.ID || session.RevokedAt != nil || !session.ExpiresAt.After(time.Now()) {
		writeJSON(w, 401, map[string]string{"error": "login_required"})
		return
	}
	if a.athubIllustrationDeleter == nil {
		writeJSON(w, 503, map[string]string{"error": "illustrations_unavailable", "error_description": "插图删除暂时不可用，请稍后重试。"})
		return
	}
	a.athubIllustrationDeleter(w, r, user.ID, session.ID, int32(novelID), int32(imageID))
}

func (a *API) uploadNovelIllustration(w http.ResponseWriter, r *http.Request) {
	originalBody := r.Body
	defer func() { _, _ = io.Copy(io.Discard, io.LimitReader(originalBody, maxNovelCoverBytes+(64<<10)+1)) }()
	user, ok := a.accountActor(w, r)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 32)
	if err != nil || id <= 0 || r.URL.RawQuery != "" {
		writeJSON(w, 400, map[string]string{"error": "invalid_novel_id", "error_description": "请选择有效的作品。"})
		return
	}
	cookie, err := r.Cookie(a.cfg.SessionCookieName)
	if err != nil {
		writeJSON(w, 401, map[string]string{"error": "login_required"})
		return
	}
	session, err := a.store.SessionByHash(r.Context(), security.HashToken(cookie.Value))
	if err != nil || session.UserID != user.ID || session.RevokedAt != nil || !session.ExpiresAt.After(time.Now()) {
		writeJSON(w, 401, map[string]string{"error": "login_required"})
		return
	}
	st, ok := a.store.(interface {
		CheckATHubIllustrationUpload(context.Context, uuid.UUID) error
	})
	if !ok || a.athubIllustrationSubmitter == nil {
		writeJSON(w, 503, map[string]string{"error": "illustrations_unavailable", "error_description": "插图上传暂时不可用，请稍后重试。"})
		return
	}
	if err := st.CheckATHubIllustrationUpload(r.Context(), user.ID); err != nil {
		a.athubPolicyError(w, err)
		return
	}
	if a.cfg.ImgbedURL == "" || a.cfg.ImgbedAPIToken == "" {
		writeJSON(w, 503, map[string]string{"error": "illustrations_unavailable", "error_description": "插图图床尚未配置，请联系管理员。"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxNovelCoverBytes+(64<<10))
	if err := r.ParseMultipartForm(maxNovelCoverBytes + (64 << 10)); err != nil {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
		writeJSON(w, 400, map[string]string{"error": "illustration_invalid", "error_description": "请选择不超过 5 MiB 的插图。"})
		return
	}
	defer r.MultipartForm.RemoveAll()
	if len(r.MultipartForm.File) != 1 || len(r.MultipartForm.File["file"]) != 1 {
		writeJSON(w, 400, map[string]string{"error": "illustration_invalid", "error_description": "每次请求请选择一张插图。"})
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "illustration_invalid"})
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxNovelCoverBytes+1))
	if err == nil {
		data, err = normalizeNovelImage(data, 4096)
	}
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "illustration_invalid", "error_description": "图片无效或尺寸过大，请使用 JPG、PNG、WebP 或 GIF 图片（最长边不超过 8192 像素，总像素不超过 1600 万）。"})
		return
	}
	size, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		a.applicationError(w, err)
		return
	}
	location, err := a.uploadNovelImage(r.Context(), data, "illustrations", strconv.FormatInt(id, 10))
	if err != nil {
		writeJSON(w, 502, map[string]string{"error": "illustration_upload_failed", "error_description": "插图上传失败，请稍后重试。"})
		return
	}
	digest := sha256.Sum256(data)
	body, _ := json.Marshal(map[string]any{"url": location, "sha256": hex.EncodeToString(digest[:]), "width": size.Width, "height": size.Height})
	request := r.Clone(r.Context())
	request.Body = io.NopCloser(bytes.NewReader(body))
	request.ContentLength = int64(len(body))
	request.Header.Set("Content-Type", "application/json")
	a.athubIllustrationSubmitter(w, request, user.ID, session.ID, int32(id))
}
