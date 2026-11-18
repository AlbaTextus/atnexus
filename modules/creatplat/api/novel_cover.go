package httpapi

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	stddraw "image/draw"
	"image/jpeg"
	"io"
	"net/http"

	"github.com/AlbaTextus/atnexus/internal/apibase"
	"github.com/google/uuid"
	"golang.org/x/image/draw"
)

const maxNovelCoverBytes = 5 << 20

// Preserve the entire cover's aspect ratio, with bounded decoding and no metadata.
func normalizeNovelCover(ctx context.Context, data []byte) ([]byte, error) {
	return normalizeNovelImage(ctx, data, 1600)
}

func normalizeNovelImage(ctx context.Context, data []byte, maxSide int) ([]byte, error) {
	if len(data) == 0 || len(data) > maxNovelCoverBytes {
		return nil, fmt.Errorf("invalid image size")
	}
	// Reading the header is cheap, so images that will be refused never queue.
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > 8192 || cfg.Height > 8192 || int64(cfg.Width)*int64(cfg.Height) > 16_000_000 {
		return nil, fmt.Errorf("invalid image dimensions")
	}
	release, err := apibase.AcquireImageWork(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
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

func (a *API) SetATHubKeyCoverUploader(upload func(http.ResponseWriter, *http.Request, func(http.ResponseWriter, *http.Request, uuid.UUID))) {
	a.athubKeyCoverUploader = upload
}

func (a *API) uploadNovelCover(w http.ResponseWriter, r *http.Request) {
	originalBody := r.Body
	defer func() { _, _ = io.Copy(io.Discard, io.LimitReader(originalBody, maxNovelCoverBytes+(64<<10)+1)) }()
	if r.Header.Get("Authorization") != "" {
		// A personal key is checked, counted and recorded by the ATHub access
		// pipeline before the image is read. Cookies play no part in this path.
		if a.athubKeyCoverUploader == nil || a.Images == nil {
			a.novelCoverUnavailable(w)
			return
		}
		a.athubKeyCoverUploader(w, r, a.saveNovelCover)
		return
	}
	user, ok := a.AccountActor(w, r)
	if !ok {
		return
	}
	st, ok := a.Store.(interface {
		CheckATHubCoverUpload(context.Context, uuid.UUID) error
	})
	if !ok || a.Images == nil {
		a.novelCoverUnavailable(w)
		return
	}
	if err := st.CheckATHubCoverUpload(r.Context(), user.ID); err != nil {
		a.PolicyError(w, err)
		return
	}
	a.saveNovelCover(w, r, user.ID)
}

// saveNovelCover stores the cover of a caller whose permission was already
// checked, under that user's cover folder.
func (a *API) saveNovelCover(w http.ResponseWriter, r *http.Request, user uuid.UUID) {
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
		data, err = normalizeNovelCover(r.Context(), data)
	}
	if r.Context().Err() != nil {
		// The timeout already answered, or nobody is waiting. Reply anyway, so a
		// recorded key call never ends up looking successful.
		writeJSON(w, 503, map[string]string{"error": "server_timeout"})
		return
	}
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "cover_invalid", "error_description": "EPUB 封面不是有效图片，或图片尺寸过大。请更换或移除封面后重试。"})
		return
	}
	location, err := a.uploadNovelImage(r.Context(), user, data, "covers", user.String())
	if err != nil {
		a.novelCoverUploadError(w)
		return
	}
	writeJSON(w, 200, map[string]string{"url": location})
}

func (a *API) uploadNovelImage(ctx context.Context, uploader uuid.UUID, data []byte, category, entityID string) (string, error) {
	return a.Images.Save(ctx, uploader.String(), "book/"+category, entityID, "cover.jpg", "image/jpeg", data)
}

func (a *API) novelCoverUnavailable(w http.ResponseWriter) {
	writeJSON(w, 503, map[string]string{"error": "cover_upload_unavailable", "error_description": "封面上传暂时不可用，请稍后重试。"})
}

func (a *API) novelCoverUploadError(w http.ResponseWriter) {
	writeJSON(w, 502, map[string]string{"error": "cover_upload_failed", "error_description": "封面上传失败，请检查图床服务后重试。书籍预览已保留。"})
}
