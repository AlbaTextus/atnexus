package image

import (
	"errors"
	"net/url"
	"strings"
	"time"
)

type Config struct {
	DatabaseURL    string
	LocalRoot      string
	TempRoot       string
	PublicURL      string
	MaxUploadBytes int64
	MaxChunkBytes  int64
	MaxChunks      int
	UploadTTL      time.Duration
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.DatabaseURL) == "" {
		return errors.New("IMAGE_DATABASE_URL is required")
	}
	if c.LocalRoot == "" || c.TempRoot == "" {
		return errors.New("IMAGE_LOCAL_ROOT and IMAGE_TEMP_ROOT are required")
	}
	u, err := url.Parse(c.PublicURL)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") || (u.Scheme != "https" && u.Scheme != "http") {
		return errors.New("IMAGE_PUBLIC_URL must be an HTTP(S) origin")
	}
	if c.MaxUploadBytes <= 0 || c.MaxChunkBytes <= 0 || c.MaxChunks <= 0 || c.MaxChunks > 10000 || c.UploadTTL <= 0 {
		return errors.New("image upload limits must be positive and max chunks at most 10000")
	}
	return nil
}
