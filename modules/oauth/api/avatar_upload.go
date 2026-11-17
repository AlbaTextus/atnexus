package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"
	"time"

	"github.com/AlbaTextus/atnexus/internal/store"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const maxAvatarBytes = 5 << 20

// Decode and re-encode a bounded image, preserving GIF animation.
func normalizeAvatar(ctx context.Context, data []byte) ([]byte, error) {
	if len(data) == 0 || len(data) > maxAvatarBytes {
		return nil, fmt.Errorf("invalid image size")
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > 8192 || cfg.Height > 8192 || int64(cfg.Width)*int64(cfg.Height) > 16_000_000 {
		return nil, fmt.Errorf("invalid image dimensions")
	}
	if format == "gif" {
		return normalizeGIF(ctx, data, nil)
	}
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	bounds := src.Bounds()
	edge := min(bounds.Dx(), bounds.Dy())
	x, y := bounds.Min.X+(bounds.Dx()-edge)/2, bounds.Min.Y+(bounds.Dy()-edge)/2
	dst := image.NewNRGBA(image.Rect(0, 0, min(edge, 512), min(edge, 512)))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, image.Rect(x, y, x+edge, y+edge), draw.Src, nil)
	var output bytes.Buffer
	if err := png.Encode(&output, dst); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func (a *API) uploadAvatar(w http.ResponseWriter, r *http.Request) {
	// Let the same-origin proxy finish sending bounded multipart bodies even
	// when authentication or validation rejects the request early.
	originalBody := r.Body
	defer func() { _, _ = io.Copy(io.Discard, io.LimitReader(originalBody, maxProfileImageBody+1)) }()
	user, ok := a.AccountActor(w, r)
	if !ok {
		return
	}
	if a.Config.ImgbedURL == "" || a.Config.ImgbedAPIToken == "" {
		writeJSON(w, 503, map[string]string{"error": "avatar_upload_unavailable", "error_description": "头像上传暂未启用，请稍后再试。"})
		return
	}
	st, ok := a.Store.(store.CustomAvatarStore)
	if !ok {
		writeJSON(w, 503, map[string]string{"error": "service_unavailable"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxProfileImageBody)
	if err := r.ParseMultipartForm(maxProfileImageBody); err != nil {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
		writeJSON(w, 400, map[string]string{"error": "invalid_avatar", "error_description": "请选择不超过 5 MB 的图片。"})
		return
	}
	defer r.MultipartForm.RemoveAll()
	if len(r.MultipartForm.File["file"]) != 1 {
		writeJSON(w, 400, map[string]string{"error": "invalid_avatar", "error_description": "请选择一张头像图片。"})
		return
	}
	original, err := readProfileOriginal(r, true)
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid_avatar", "error_description": "头像原图无效，请选择不超过 5 MB、1600 万像素的图片。"})
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid_avatar"})
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxAvatarBytes+1))
	if err == nil {
		data, err = normalizeAvatar(r.Context(), data)
	}
	if r.Context().Err() != nil {
		// The reply was already taken over by the timeout, or nobody is waiting.
		return
	}
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid_avatar", "error_description": "请使用不超过 5 MB 的有效图片；GIF 最多 200 帧，全部画面合计不超过 4000 万像素。"})
		return
	}
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	filename, contentType := "avatar.png", "image/png"
	if bytes.HasPrefix(data, []byte("GIF")) {
		filename, contentType = "avatar.gif", "image/gif"
	}
	part, err := form.CreatePart(textproto.MIMEHeader{"Content-Disposition": {fmt.Sprintf(`form-data; name="file"; filename="%s"`, filename)}, "Content-Type": {contentType}})
	if err != nil {
		a.ApplicationError(w, err)
		return
	}
	_, _ = part.Write(data)
	_ = form.Close()
	target := a.Config.ImgbedURL + "/api/v2/categories/user/avatars/upload?returnFormat=full&entityId=" + user.ID.String()
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, target, &body)
	if err != nil {
		a.ApplicationError(w, err)
		return
	}
	req.Header.Set("Authorization", "Bearer "+a.Config.ImgbedAPIToken)
	req.Header.Set("Content-Type", form.FormDataContentType())
	client := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(req)
	if err != nil {
		a.avatarUploadError(w)
		return
	}
	defer response.Body.Close()
	var uploaded []struct {
		Src string `json:"src"`
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 || json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&uploaded) != nil || len(uploaded) != 1 {
		a.avatarUploadError(w)
		return
	}
	base, _ := url.Parse(a.Config.ImgbedURL)
	ref, err := url.Parse(uploaded[0].Src)
	if err != nil {
		a.avatarUploadError(w)
		return
	}
	resolved := base.ResolveReference(ref)
	if resolved.Scheme != base.Scheme || resolved.Host != base.Host || resolved.User != nil || resolved.Fragment != "" || !strings.HasPrefix(resolved.Path, "/file/") || len(resolved.String()) > 2048 {
		a.avatarUploadError(w)
		return
	}
	originalURL, err := a.uploadProfileOriginal(r.Context(), user.ID, "avatars", original)
	if err != nil {
		a.avatarUploadError(w)
		return
	}
	updated, err := st.UpdateCustomAvatar(r.Context(), user.ID, resolved.String(), originalURL)
	if err != nil {
		a.ApplicationError(w, err)
		return
	}
	writeJSON(w, 200, userView(updated))
}

func (a *API) avatarUploadError(w http.ResponseWriter) {
	writeJSON(w, 502, map[string]string{"error": "avatar_upload_failed", "error_description": "图片上传失败，请稍后重试。原头像未更改。"})
}
