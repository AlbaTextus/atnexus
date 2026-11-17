package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/gif"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"
	"time"

	"github.com/AlbaTextus/atnexus/internal/apibase"
	"github.com/google/uuid"
)

const maxProfileImageBody = 2*maxAvatarBytes + (64 << 10)

// Keep the original bytes and dimensions for the editor, separate from the
// bounded crop used by avatars and public cards. Original URLs are private
// account metadata and are not included in the public profile response.
func readProfileOriginal(r *http.Request, allowGIF bool) ([]byte, error) {
	files := r.MultipartForm.File["original"]
	if len(files) == 0 {
		return nil, nil
	}
	if len(files) != 1 {
		return nil, fmt.Errorf("expected one original image")
	}
	file, err := files[0].Open()
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxAvatarBytes+1))
	if err != nil {
		return nil, err
	}
	if err := validateProfileOriginal(r.Context(), data, allowGIF); err != nil {
		return nil, err
	}
	return data, nil
}

func validateProfileOriginal(ctx context.Context, data []byte, allowGIF bool) error {
	if len(data) == 0 || len(data) > maxAvatarBytes {
		return fmt.Errorf("invalid image size")
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > 8192 || cfg.Height > 8192 || int64(cfg.Width)*int64(cfg.Height) > 16_000_000 {
		return fmt.Errorf("invalid image dimensions")
	}
	if format == "gif" {
		if !allowGIF {
			return fmt.Errorf("GIF backgrounds are not supported")
		}
		if err := checkGIFBudget(data); err != nil {
			return err
		}
	} else if format != "png" && format != "jpeg" && format != "webp" {
		return fmt.Errorf("unsupported image format")
	}
	// Only a full decode proves the bytes are a usable image. It costs as much
	// as normalizing one, so it takes a turn as well.
	release, err := apibase.AcquireImageWork(ctx)
	if err != nil {
		return err
	}
	defer release()
	if format == "gif" {
		_, err = gif.DecodeAll(bytes.NewReader(data))
	} else {
		_, _, err = image.Decode(bytes.NewReader(data))
	}
	return err
}

func (a *API) uploadProfileOriginal(ctx context.Context, userID uuid.UUID, category string, data []byte) (string, error) {
	if len(data) == 0 {
		return "", nil
	}
	_, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	return a.uploadToImgbed(ctx, category, userID, "original."+format, http.DetectContentType(data), data)
}

// Shared by every profile image upload: connections to the image service are
// reused, and redirects are never followed with the service token attached.
var imgbedClient = &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

// uploadToImgbed stores one image in a user category and returns its file URL,
// accepted only when it stays on the configured image origin.
func (a *API) uploadToImgbed(ctx context.Context, category string, userID uuid.UUID, filename, contentType string, data []byte) (string, error) {
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	part, err := form.CreatePart(textproto.MIMEHeader{"Content-Disposition": {fmt.Sprintf(`form-data; name="file"; filename="%s"`, filename)}, "Content-Type": {contentType}})
	if err != nil {
		return "", err
	}
	_, _ = part.Write(data)
	_ = form.Close()
	target := a.Config.ImgbedURL + "/api/v2/categories/user/" + category + "/upload?returnFormat=full&entityId=" + userID.String()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+a.Config.ImgbedAPIToken)
	req.Header.Set("Content-Type", form.FormDataContentType())
	response, err := imgbedClient.Do(req)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	var uploaded []struct {
		Src string `json:"src"`
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 || json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&uploaded) != nil || len(uploaded) != 1 {
		return "", fmt.Errorf("image upload failed")
	}
	base, err := url.Parse(a.Config.ImgbedURL)
	if err != nil {
		return "", err
	}
	ref, err := url.Parse(uploaded[0].Src)
	if err != nil {
		return "", err
	}
	resolved := base.ResolveReference(ref)
	if resolved.Scheme != base.Scheme || resolved.Host != base.Host || resolved.User != nil || resolved.Fragment != "" || !strings.HasPrefix(resolved.Path, "/file/") || len(resolved.String()) > 2048 {
		return "", fmt.Errorf("invalid uploaded image URL")
	}
	return resolved.String(), nil
}
