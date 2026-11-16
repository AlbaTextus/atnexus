package image

import (
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/AlbaTextus/atnexus/pkg/auth"
	smithyhttp "github.com/aws/smithy-go/transport/http"
)

func (m *Module) Files() http.Handler { return http.HandlerFunc(m.serveFile) }
func (m *Module) serveFile(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if !method(w, r, http.MethodGet, http.MethodHead) {
		return
	}
	id := ""
	for _, prefix := range []string{"/file/", "/api/v2/file/"} {
		if strings.HasPrefix(r.URL.Path, prefix) {
			id = strings.TrimPrefix(r.URL.Path, prefix)
			break
		}
	}
	if !validKey(id) {
		respondError(w, 400, "invalid_file_path")
		return
	}
	f, err := m.getFile(r.Context(), id)
	if err != nil {
		m.failure(w, err)
		return
	}
	preview := r.URL.Query().Get("from") == "admin"
	if preview {
		authorized := false
		if header := r.Header.Get("Authorization"); header != "" {
			_, err = m.deps.Keys.AuthorizeImageKey(r.Context(), strings.TrimPrefix(header, "Bearer "), "manage")
			authorized = err == nil
		} else {
			identity, e := m.deps.Sessions.VerifySession(r)
			err = e
			authorized = e == nil && (identity.Role == "ADMIN" || identity.Role == "SUPER_ADMIN")
		}
		if !authorized {
			switch {
			case errors.Is(err, auth.ErrUnauthenticated):
				respondError(w, 401, "admin_authentication_required")
			case err == nil || errors.Is(err, auth.ErrForbidden):
				respondError(w, 403, "permission_denied")
			default:
				respondError(w, 503, "authentication_unavailable")
			}
			return
		}
	} else if text(f.Metadata, "ListType") == "Block" {
		respondError(w, 403, "image_blocked")
		return
	}
	store, key, err := m.storeFor(r.Context(), f.Metadata)
	if err != nil {
		m.failure(w, err)
		return
	}
	if key == "" {
		key = id
	}
	info, err := store.Open(r.Context(), key, r.Header.Get("Range"), r.Method == http.MethodHead)
	if err != nil {
		var remote *smithyhttp.ResponseError
		switch {
		case errors.Is(err, os.ErrNotExist):
			respondError(w, 404, "image_bytes_not_found")
		case errors.As(err, &remote) && remote.HTTPStatusCode() == 404:
			respondError(w, 404, "image_bytes_not_found")
		case remote != nil && remote.HTTPStatusCode() == 416:
			w.Header().Set("Accept-Ranges", "bytes")
			respondError(w, 416, "invalid_byte_range")
		default:
			m.failure(w, err)
		}
		return
	}
	if info.Body != nil {
		defer info.Body.Close()
	}
	contentType := text(f.Metadata, "FileType")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	filename := text(f.Metadata, "FileName")
	if filename == "" {
		filename = id
	}
	w.Header().Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": filename}))
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Cache-Control", "public, max-age=2592000")
	if preview {
		w.Header().Set("Cache-Control", "private, no-store")
		w.Header().Del("Access-Control-Allow-Origin")
	}
	// Uploaded HTML/SVG must not gain same-origin access to session-bearing APIs.
	w.Header().Set("Content-Security-Policy", "sandbox")
	if local, ok := info.Body.(*os.File); ok {
		http.ServeContent(w, r, filename, info.Modified, local)
		return
	}
	if info.ETag != "" {
		w.Header().Set("ETag", info.ETag)
	}
	w.Header().Set("Content-Length", strconv.FormatInt(info.Size, 10))
	if info.Range != "" {
		w.Header().Set("Content-Range", info.Range)
		w.WriteHeader(http.StatusPartialContent)
	}
	if r.Method != http.MethodHead && info.Body != nil {
		_, _ = io.Copy(w, info.Body)
	}
}

func fileURL(origin, id string) string {
	u, _ := url.Parse(strings.TrimRight(origin, "/") + "/file/")
	u.Path += id
	return u.String()
}
