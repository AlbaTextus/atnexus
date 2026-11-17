package httpapi

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"

	"github.com/AlbaTextus/atnexus/internal/store"
	"golang.org/x/image/draw"
)

func normalizeBackground(data []byte) ([]byte, error) {
	if len(data) == 0 || len(data) > maxAvatarBytes {
		return nil, fmt.Errorf("invalid image size")
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > 8192 || cfg.Height > 8192 || int64(cfg.Width)*int64(cfg.Height) > 16_000_000 || format == "gif" {
		return nil, fmt.Errorf("invalid background image")
	}
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	w, h := cfg.Width, cfg.Height
	if max(w, h) > 1920 {
		scale := 1920.0 / float64(max(w, h))
		w, h = max(1, int(float64(w)*scale)), max(1, int(float64(h)*scale))
	}
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Src, nil)
	var out bytes.Buffer
	err = png.Encode(&out, dst)
	return out.Bytes(), err
}

func (a *API) removeBackground(w http.ResponseWriter, r *http.Request) {
	user, ok := a.AccountActor(w, r)
	if !ok {
		return
	}
	st, ok := a.Store.(store.BackgroundStore)
	if !ok {
		writeJSON(w, 503, map[string]string{"error": "service_unavailable"})
		return
	}
	updated, err := st.UpdateBackground(r.Context(), user.ID, "", "")
	if err != nil {
		a.ApplicationError(w, err)
		return
	}
	writeJSON(w, 200, userView(updated))
}

func (a *API) uploadBackground(w http.ResponseWriter, r *http.Request) {
	originalBody := r.Body
	defer func() { _, _ = io.Copy(io.Discard, io.LimitReader(originalBody, maxProfileImageBody+1)) }()
	user, ok := a.AccountActor(w, r)
	if !ok {
		return
	}
	if a.Images == nil {
		writeJSON(w, 503, map[string]string{"error": "background_upload_unavailable", "error_description": "背投上传暂未启用，请稍后再试。"})
		return
	}
	st, ok := a.Store.(store.BackgroundStore)
	if !ok {
		writeJSON(w, 503, map[string]string{"error": "service_unavailable"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxProfileImageBody)
	err := r.ParseMultipartForm(maxProfileImageBody)
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	if err != nil || r.MultipartForm == nil || len(r.MultipartForm.File["file"]) != 1 {
		writeJSON(w, 400, map[string]string{"error": "invalid_background", "error_description": "请选择一张不超过 5 MB 的 PNG、JPEG 或 WebP 图片。"})
		return
	}
	original, err := readProfileOriginal(r, false)
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid_background", "error_description": "背投原图无效，请选择不超过 5 MB、1600 万像素的 PNG、JPEG 或 WebP 图片。"})
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid_background"})
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxAvatarBytes+1))
	if err == nil {
		data, err = normalizeBackground(data)
	}
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid_background", "error_description": "请使用不超过 5 MB、1600 万像素的有效 PNG、JPEG 或 WebP 图片。"})
		return
	}
	location, err := a.Images.Save(r.Context(), "user/backgrounds", user.ID.String(), "background.png", "image/png", data)
	if err != nil {
		backgroundUploadError(w)
		return
	}
	originalURL, err := a.uploadProfileOriginal(r.Context(), user.ID, "backgrounds", original)
	if err != nil {
		backgroundUploadError(w)
		return
	}
	updated, err := st.UpdateBackground(r.Context(), user.ID, location, originalURL)
	if err != nil {
		a.ApplicationError(w, err)
		return
	}
	writeJSON(w, 200, userView(updated))
}

func backgroundUploadError(w http.ResponseWriter) {
	writeJSON(w, 502, map[string]string{"error": "background_upload_failed", "error_description": "背投上传失败，请稍后重试。原背投未更改。"})
}
