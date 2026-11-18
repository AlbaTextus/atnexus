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

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (a *API) SetATHubIllustrationReaders(list, layout func(http.ResponseWriter, *http.Request, int32)) {
	a.athubIllustrationReader, a.athubIllustrationLayoutReader = list, layout
}

func (a *API) listNovelIllustrations(w http.ResponseWriter, r *http.Request) {
	a.readIllustrationContent(w, r, a.athubIllustrationReader)
}

func (a *API) readNovelIllustrationLayout(w http.ResponseWriter, r *http.Request) {
	a.readIllustrationContent(w, r, a.athubIllustrationLayoutReader)
}

func (a *API) readIllustrationContent(w http.ResponseWriter, r *http.Request, read func(http.ResponseWriter, *http.Request, int32)) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 32)
	if err != nil || id <= 0 {
		writeJSON(w, 400, map[string]string{"error": "invalid_novel_id"})
		return
	}
	if read == nil {
		a.novelIllustrationsUnavailable(w)
		return
	}
	read(w, r, int32(id))
}

func (a *API) SetATHubIllustrationSubmitter(submit func(http.ResponseWriter, *http.Request, uuid.UUID, uuid.UUID, int32)) {
	a.athubIllustrationSubmitter = submit
}

func (a *API) SetATHubIllustrationDeleter(remove func(http.ResponseWriter, *http.Request, uuid.UUID, uuid.UUID, int32, int32)) {
	a.athubIllustrationDeleter = remove
}

func (a *API) deleteNovelIllustration(w http.ResponseWriter, r *http.Request) {
	user, session, ok := a.ATHubSessionActor(w, r)
	if !ok {
		return
	}
	novelID, novelErr := strconv.ParseInt(chi.URLParam(r, "id"), 10, 32)
	imageID, imageErr := strconv.ParseInt(chi.URLParam(r, "illustrationId"), 10, 32)
	if novelErr != nil || imageErr != nil || novelID <= 0 || imageID <= 0 || r.URL.RawQuery != "" {
		writeJSON(w, 400, map[string]string{"error": "invalid_illustration", "error_description": "请选择有效的作品插图。"})
		return
	}
	if a.athubIllustrationDeleter == nil {
		writeJSON(w, 503, map[string]string{"error": "illustrations_unavailable", "error_description": "插图删除暂时不可用，请稍后重试。"})
		return
	}
	a.athubIllustrationDeleter(w, r, user.ID, session.ID, int32(novelID), int32(imageID))
}

func (a *API) SetATHubKeyIllustrationUploader(upload func(http.ResponseWriter, *http.Request, func(http.ResponseWriter, *http.Request) (int32, []byte, bool))) {
	a.athubKeyIllustrationUploader = upload
}

func (a *API) uploadNovelIllustration(w http.ResponseWriter, r *http.Request) {
	originalBody := r.Body
	defer func() { _, _ = io.Copy(io.Discard, io.LimitReader(originalBody, maxNovelCoverBytes+(64<<10)+1)) }()
	if r.Header.Get("Authorization") != "" {
		// A personal key is checked, counted and recorded by the ATHub access
		// pipeline before the image is read. Cookies play no part in this path.
		if a.athubKeyIllustrationUploader == nil || a.Images == nil {
			a.novelIllustrationsUnavailable(w)
			return
		}
		a.athubKeyIllustrationUploader(w, r, func(w http.ResponseWriter, admitted *http.Request) (int32, []byte, bool) {
			// The pipeline gave the admitted request its own path, so the novel
			// is read from the route as it arrived.
			id, ok := novelIllustrationID(w, r)
			if !ok {
				return 0, nil, false
			}
			body, ok := a.storeNovelIllustration(w, admitted, id)
			return id, body, ok
		})
		return
	}
	user, session, ok := a.SessionActor(w, r)
	if !ok {
		return
	}
	id, ok := novelIllustrationID(w, r)
	if !ok {
		return
	}
	st, ok := a.Store.(interface {
		CheckATHubIllustrationUpload(context.Context, uuid.UUID) error
	})
	if !ok || a.athubIllustrationSubmitter == nil || a.Images == nil {
		a.novelIllustrationsUnavailable(w)
		return
	}
	if err := st.CheckATHubIllustrationUpload(r.Context(), user.ID); err != nil {
		a.PolicyError(w, err)
		return
	}
	body, ok := a.storeNovelIllustration(w, r, id)
	if !ok {
		return
	}
	request := r.Clone(r.Context())
	request.Body = io.NopCloser(bytes.NewReader(body))
	request.ContentLength = int64(len(body))
	request.Header.Set("Content-Type", "application/json")
	a.athubIllustrationSubmitter(w, request, user.ID, session.ID, id)
}

// novelIllustrationID reads the novel from the route; an upload takes no query.
func novelIllustrationID(w http.ResponseWriter, r *http.Request) (int32, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 32)
	if err != nil || id <= 0 || r.URL.RawQuery != "" {
		writeJSON(w, 400, map[string]string{"error": "invalid_novel_id", "error_description": "请选择有效的作品。"})
		return 0, false
	}
	return int32(id), true
}

// storeNovelIllustration saves the uploaded image of a caller whose permission
// was already checked, and returns the JSON the content API records for it. It
// replies itself and returns false when the image cannot be stored.
func (a *API) storeNovelIllustration(w http.ResponseWriter, r *http.Request, novelID int32) ([]byte, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxNovelCoverBytes+(64<<10))
	if err := r.ParseMultipartForm(maxNovelCoverBytes + (64 << 10)); err != nil {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
		writeJSON(w, 400, map[string]string{"error": "illustration_invalid", "error_description": "请选择不超过 5 MiB 的插图。"})
		return nil, false
	}
	defer r.MultipartForm.RemoveAll()
	if len(r.MultipartForm.File) != 1 || len(r.MultipartForm.File["file"]) != 1 {
		writeJSON(w, 400, map[string]string{"error": "illustration_invalid", "error_description": "每次请求请选择一张插图。"})
		return nil, false
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "illustration_invalid"})
		return nil, false
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxNovelCoverBytes+1))
	if err == nil {
		data, err = normalizeNovelImage(r.Context(), data, 4096)
	}
	if r.Context().Err() != nil {
		// The timeout already answered, or nobody is waiting. Reply anyway, so a
		// recorded key call never ends up looking successful.
		writeJSON(w, 503, map[string]string{"error": "server_timeout"})
		return nil, false
	}
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "illustration_invalid", "error_description": "图片无效或尺寸过大，请使用 JPG、PNG、WebP 或 GIF 图片（最长边不超过 8192 像素，总像素不超过 1600 万）。"})
		return nil, false
	}
	size, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		a.ApplicationError(w, err)
		return nil, false
	}
	location, err := a.uploadNovelImage(r.Context(), data, "illustrations", strconv.FormatInt(int64(novelID), 10))
	if err != nil {
		writeJSON(w, 502, map[string]string{"error": "illustration_upload_failed", "error_description": "插图上传失败，请稍后重试。"})
		return nil, false
	}
	digest := sha256.Sum256(data)
	body, _ := json.Marshal(map[string]any{"url": location, "sha256": hex.EncodeToString(digest[:]), "width": size.Width, "height": size.Height})
	return body, true
}

func (a *API) novelIllustrationsUnavailable(w http.ResponseWriter) {
	writeJSON(w, 503, map[string]string{"error": "illustrations_unavailable", "error_description": "插图上传暂时不可用，请稍后重试。"})
}
