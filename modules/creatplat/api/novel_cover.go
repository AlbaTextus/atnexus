package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	stddraw "image/draw"
	"image/jpeg"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/image/draw"
)

const maxNovelCoverBytes = 5 << 20

// Preserve the entire cover's aspect ratio, with bounded decoding and no metadata.
func normalizeNovelCover(data []byte) ([]byte, error) {
	return normalizeNovelImage(data, 1600)
}

func normalizeNovelImage(data []byte, maxSide int) ([]byte, error) {
	if len(data) == 0 || len(data) > maxNovelCoverBytes {
		return nil, fmt.Errorf("invalid image size")
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > 8192 || cfg.Height > 8192 || int64(cfg.Width)*int64(cfg.Height) > 16_000_000 {
		return nil, fmt.Errorf("invalid image dimensions")
	}
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	width, height := cfg.Width, cfg.Height
	if max(width, height) > maxSide {
		width = max(1, width*maxSide/max(cfg.Width, cfg.Height))
		height = max(1, height*maxSide/max(cfg.Width, cfg.Height))
	}
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	stddraw.Draw(dst, dst.Bounds(), &image.Uniform{C: color.White}, image.Point{}, stddraw.Src)
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)
	var output bytes.Buffer
	if err := jpeg.Encode(&output, dst, &jpeg.Options{Quality: 90}); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func (a *API) uploadNovelCover(w http.ResponseWriter, r *http.Request) {
	originalBody := r.Body
	defer func() { _, _ = io.Copy(io.Discard, io.LimitReader(originalBody, maxNovelCoverBytes+(64<<10)+1)) }()
	user, ok := a.AccountActor(w, r)
	if !ok {
		return
	}
	st, ok := a.Store.(interface {
		CheckATHubCoverUpload(context.Context, uuid.UUID) error
	})
	if !ok {
		writeJSON(w, 503, map[string]string{"error": "cover_upload_unavailable", "error_description": "封面上传暂时不可用，请稍后重试。"})
		return
	}
	if err := st.CheckATHubCoverUpload(r.Context(), user.ID); err != nil {
		a.PolicyError(w, err)
		return
	}
	if a.Config.ImgbedURL == "" || a.Config.ImgbedAPIToken == "" {
		writeJSON(w, 503, map[string]string{"error": "cover_upload_unavailable", "error_description": "封面图床尚未配置，请联系管理员。书籍预览已保留。"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxNovelCoverBytes+(64<<10))
	if err := r.ParseMultipartForm(maxNovelCoverBytes + (64 << 10)); err != nil {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
		writeJSON(w, 400, map[string]string{"error": "cover_invalid", "error_description": "请选择不超过 5 MiB 的封面图片。"})
		return
	}
	defer r.MultipartForm.RemoveAll()
	if len(r.MultipartForm.File["file"]) != 1 {
		writeJSON(w, 400, map[string]string{"error": "cover_invalid", "error_description": "请选择一张封面图片。"})
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "cover_invalid"})
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxNovelCoverBytes+1))
	if err == nil {
		data, err = normalizeNovelCover(data)
	}
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "cover_invalid", "error_description": "EPUB 封面不是有效图片，或图片尺寸过大。请更换或移除封面后重试。"})
		return
	}
	location, err := a.uploadNovelImage(r.Context(), data, "covers", user.ID.String())
	if err != nil {
		a.novelCoverUploadError(w)
		return
	}
	writeJSON(w, 200, map[string]string{"url": location})
}

func (a *API) uploadNovelImage(ctx context.Context, data []byte, category, entityID string) (string, error) {
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	part, err := form.CreatePart(textproto.MIMEHeader{
		"Content-Disposition": {`form-data; name="file"; filename="cover.jpg"`},
		"Content-Type":        {"image/jpeg"},
	})
	if err != nil {
		return "", err
	}
	_, _ = part.Write(data)
	_ = form.Close()
	target := a.Config.ImgbedURL + "/api/v2/categories/book/" + category + "/upload?returnFormat=full&entityId=" + url.QueryEscape(entityID)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, target, &body)
	if err != nil {
		return "", err
	}
	request.Header.Set("Authorization", "Bearer "+a.Config.ImgbedAPIToken)
	request.Header.Set("Content-Type", form.FormDataContentType())
	client := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	var uploaded []struct {
		Src string `json:"src"`
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 || json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&uploaded) != nil || len(uploaded) != 1 {
		return "", fmt.Errorf("image host upload failed")
	}
	base, _ := url.Parse(a.Config.ImgbedURL)
	ref, err := url.Parse(uploaded[0].Src)
	if err != nil || uploaded[0].Src == "" {
		return "", fmt.Errorf("invalid image host response")
	}
	resolved := base.ResolveReference(ref)
	if resolved.Scheme != base.Scheme || resolved.Host != base.Host || resolved.User != nil || resolved.Fragment != "" || !strings.HasPrefix(resolved.Path, "/file/") || len(resolved.String()) > 2048 {
		return "", fmt.Errorf("invalid image host URL")
	}
	return resolved.String(), nil
}

func (a *API) novelCoverUploadError(w http.ResponseWriter) {
	writeJSON(w, 502, map[string]string{"error": "cover_upload_failed", "error_description": "封面上传失败，请检查图床服务后重试。书籍预览已保留。"})
}
