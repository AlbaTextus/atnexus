package api

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"albatextus/athub/internal/storage"
)

const MaxBodyBytes int64 = 8 << 20

type API struct {
	store     storage.Store
	tokenHash [32]byte
	logger    *slog.Logger
}

func New(store storage.Store, token string, logger *slog.Logger) (http.Handler, error) {
	if len(token) < 32 || strings.TrimSpace(token) != token || strings.ContainsAny(token, " \t\r\n") {
		return nil, errors.New("API_TOKEN must contain at least 32 bytes and no whitespace")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &API{store: store, tokenHash: sha256.Sum256([]byte(token)), logger: logger}, nil
}

func (a *API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	scheme, token, ok := strings.Cut(r.Header.Get("Authorization"), " ")
	tokenHash := sha256.Sum256([]byte(token))
	if len(r.Header.Values("Authorization")) != 1 || !ok || !strings.EqualFold(scheme, "Bearer") || subtle.ConstantTimeCompare(tokenHash[:], a.tokenHash[:]) != 1 {
		w.Header().Set("WWW-Authenticate", "Bearer")
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	a.serveContent(w, r)
}

// NewWithMiddleware delegates authentication and accounting to the host.
// New remains available for standalone API regression tests.
func NewWithMiddleware(store storage.Store, wrap func(http.Handler) http.Handler, logger *slog.Logger) (http.Handler, error) {
	if wrap == nil {
		return nil, errors.New("host authentication middleware is required")
	}
	if logger == nil {
		logger = slog.Default()
	}
	a := &API{store: store, logger: logger}
	return wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		a.serveContent(w, r)
	})), nil
}

func (a *API) serveContent(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	r = r.WithContext(ctx)
	var err error
	switch r.URL.Path {
	case "/api/v2/platforms":
		err = a.platforms(w, r)
	case "/api/v2/authors":
		err = a.authors(w, r)
	case "/api/v2/novelinfo":
		err = a.novelInfo(w, r)
	case "/api/v2/content":
		err = a.content(w, r)
	case "/api/v2/toc":
		err = a.toc(w, r)
	case "/api/v2/volumes":
		err = a.volumes(w, r)
	case "/api/v2/upload":
		err = a.upload(w, r)
	default:
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		a.respondError(w, r, err)
	}
}

type requestError struct {
	status  int
	message string
}

func (e *requestError) Error() string { return e.message }

func badRequest(message string) error { return &requestError{http.StatusBadRequest, message} }

func (a *API) novelInfo(w http.ResponseWriter, r *http.Request) error {
	query, err := parseQuery(r.URL.RawQuery, "id", "platformId", "authorId", "limit", "offset")
	if err != nil {
		return err
	}
	switch r.Method {
	case http.MethodGet:
		if query.Has("id") {
			if len(query) != 1 {
				return badRequest("id cannot be combined with list filters")
			}
			id, err := positiveID(query, "id", true)
			if err != nil {
				return err
			}
			n, err := a.store.GetNovel(r.Context(), id)
			if err != nil {
				return err
			}
			return writeJSON(w, http.StatusOK, n)
		}
		platformID, err := positiveID(query, "platformId", false)
		if err != nil {
			return err
		}
		authorID, err := positiveID(query, "authorId", false)
		if err != nil {
			return err
		}
		limit, offset, err := pagination(query)
		if err != nil {
			return err
		}
		novels, err := a.store.ListNovels(r.Context(), storage.NovelFilter{PlatformID: platformID, AuthorID: authorID, Limit: limit, Offset: offset})
		if err != nil {
			return err
		}
		if novels == nil {
			novels = []storage.Novel{}
		}
		return writeJSON(w, http.StatusOK, novels)
	case http.MethodPost:
		if len(query) != 0 {
			return badRequest("POST /api/v2/novelinfo does not accept query parameters")
		}
		var input storage.NovelInput
		if err := decodeJSON(w, r, &input); err != nil {
			return err
		}
		if err := validateNovel(&input); err != nil {
			return err
		}
		n, err := a.store.CreateNovel(r.Context(), input)
		if err != nil {
			return err
		}
		w.Header().Set("Location", "/api/v2/novelinfo?id="+strconv.FormatInt(int64(n.ID), 10))
		return writeJSON(w, http.StatusCreated, n)
	case http.MethodPut:
		id, err := onlyID(query)
		if err != nil {
			return err
		}
		var input storage.NovelInput
		if err := decodeJSON(w, r, &input); err != nil {
			return err
		}
		if err := validateNovel(&input); err != nil {
			return err
		}
		n, err := a.store.UpdateNovel(r.Context(), id, input)
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusOK, n)
	case http.MethodDelete:
		id, err := onlyID(query)
		if err != nil {
			return err
		}
		if err := a.store.DeleteNovel(r.Context(), id); err != nil {
			return err
		}
		w.WriteHeader(http.StatusNoContent)
		return nil
	default:
		w.Header().Set("Allow", "GET, POST, PUT, DELETE")
		return &requestError{http.StatusMethodNotAllowed, "method not allowed"}
	}
}

func (a *API) content(w http.ResponseWriter, r *http.Request) error {
	query, err := parseQuery(r.URL.RawQuery, "novelId", "order", "limit", "offset")
	if err != nil {
		return err
	}
	switch r.Method {
	case http.MethodGet:
		novelID, err := positiveID(query, "novelId", true)
		if err != nil {
			return err
		}
		if query.Has("order") {
			if len(query) != 2 {
				return badRequest("order cannot be combined with pagination")
			}
			order, err := positiveID(query, "order", true)
			if err != nil {
				return err
			}
			chapter, err := a.store.GetChapter(r.Context(), novelID, order)
			if err != nil {
				return err
			}
			return writeJSON(w, http.StatusOK, chapter)
		}
		limit, offset, err := pagination(query)
		if err != nil {
			return err
		}
		chapters, err := a.store.ListChapters(r.Context(), novelID, limit, offset)
		if err != nil {
			return err
		}
		if chapters == nil {
			chapters = []storage.Chapter{}
		}
		return writeJSON(w, http.StatusOK, chapters)
	case http.MethodPut:
		if len(query) != 0 {
			return badRequest("PUT /api/v2/content uses novelId and order in the JSON body")
		}
		var input storage.ChapterInput
		if err := decodeJSON(w, r, &input); err != nil {
			return err
		}
		if err := validateChapter(&input); err != nil {
			return err
		}
		chapter, err := a.store.SaveChapter(r.Context(), input)
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusOK, chapter)
	case http.MethodDelete:
		if len(query) != 2 {
			return badRequest("novelId and order are required; no other query parameters are allowed")
		}
		novelID, err := positiveID(query, "novelId", true)
		if err != nil {
			return err
		}
		order, err := positiveID(query, "order", true)
		if err != nil {
			return err
		}
		if err := a.store.DeleteChapter(r.Context(), novelID, order); err != nil {
			return err
		}
		w.WriteHeader(http.StatusNoContent)
		return nil
	default:
		w.Header().Set("Allow", "GET, PUT, DELETE")
		return &requestError{http.StatusMethodNotAllowed, "method not allowed"}
	}
}

func validateNovel(n *storage.NovelInput) error {
	if strings.ContainsRune(n.Title, 0) || (n.Summary != nil && strings.ContainsRune(*n.Summary, 0)) || (n.CoverURL != nil && strings.ContainsRune(*n.CoverURL, 0)) {
		return badRequest("text fields cannot contain NUL characters")
	}
	if n.PlatformID <= 0 || (n.AuthorID != nil && *n.AuthorID <= 0) {
		return badRequest("platformId and authorId must be positive 32-bit integers")
	}
	n.Title = strings.TrimSpace(n.Title)
	if n.Title == "" {
		return badRequest("title is required")
	}
	if n.Status == "" {
		n.Status = "ONGOING"
	}
	if n.Status != "ONGOING" && n.Status != "COMPLETED" {
		return badRequest("status must be ONGOING or COMPLETED")
	}
	return nil
}

func validateChapter(input *storage.ChapterInput) error {
	if input.NovelID <= 0 || input.Order <= 0 {
		return badRequest("novelId and order must be positive 32-bit integers")
	}
	if input.VolumeID != nil && *input.VolumeID <= 0 {
		return badRequest("volumeId must be a positive 32-bit integer or null")
	}
	return validateChapterText(&input.Title, input.Content)
}

func validateChapterText(title *string, content string) error {
	if err := validateTitle(title); err != nil {
		return err
	}
	if strings.TrimSpace(content) == "" {
		return badRequest("content is required")
	}
	if strings.ContainsRune(content, 0) {
		return badRequest("text fields cannot contain NUL characters")
	}
	return nil
}

func validateTitle(title *string) error {
	*title = strings.TrimSpace(*title)
	if *title == "" {
		return badRequest("title is required")
	}
	if strings.ContainsRune(*title, 0) {
		return badRequest("text fields cannot contain NUL characters")
	}
	return nil
}

func parseQuery(raw string, allowed ...string) (url.Values, error) {
	query, err := url.ParseQuery(raw)
	if err != nil {
		return nil, badRequest("invalid query string")
	}
	for key, values := range query {
		valid := false
		for _, name := range allowed {
			if key == name {
				valid = true
				break
			}
		}
		if !valid || len(values) != 1 {
			return nil, badRequest("unknown or repeated query parameter: " + key)
		}
	}
	return query, nil
}

func positiveID(query url.Values, key string, required bool) (int32, error) {
	if !query.Has(key) && !required {
		return 0, nil
	}
	value, err := strconv.ParseInt(query.Get(key), 10, 32)
	if err != nil || value <= 0 {
		return 0, badRequest(key + " must be a positive 32-bit integer")
	}
	return int32(value), nil
}

func onlyID(query url.Values) (int32, error) {
	if len(query) != 1 || !query.Has("id") {
		return 0, badRequest("only the id query parameter is required")
	}
	return positiveID(query, "id", true)
}

func pagination(query url.Values) (int32, int32, error) {
	limit := int64(20)
	if query.Has("limit") {
		value, err := strconv.ParseInt(query.Get("limit"), 10, 32)
		if err != nil || value < 1 || value > 100 {
			return 0, 0, badRequest("limit must be between 1 and 100")
		}
		limit = value
	}
	var offset int64
	if query.Has("offset") {
		value, err := strconv.ParseInt(query.Get("offset"), 10, 32)
		if err != nil || value < 0 {
			return 0, 0, badRequest("offset must be a nonnegative 32-bit integer")
		}
		offset = value
	}
	return int32(limit), int32(offset), nil
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) error {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return &requestError{http.StatusUnsupportedMediaType, "Content-Type must be application/json"}
	}
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)
	data, err := io.ReadAll(r.Body)
	if err != nil {
		var limitError *http.MaxBytesError
		if errors.As(err, &limitError) {
			return &requestError{http.StatusRequestEntityTooLarge, "JSON body exceeds 8 MiB"}
		}
		return badRequest("could not read JSON body")
	}
	if !utf8.Valid(data) {
		return badRequest("JSON body must be valid UTF-8")
	}
	// Require an object: encoding/json otherwise accepts null as an empty struct.
	if body := strings.TrimSpace(string(data)); len(body) == 0 || body[0] != '{' {
		return badRequest("JSON body must be an object")
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return badRequest("invalid JSON body or unknown field")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return badRequest("JSON body must contain exactly one object")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) error {
	// Encode before writing headers so an encoding failure can still return 500.
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if _, err := w.Write(append(data, '\n')); err != nil {
		slog.Warn("response write failed", "error", err)
	}
	return nil
}

func writeError(w http.ResponseWriter, status int, message string) {
	_ = writeJSON(w, status, map[string]string{"error": message})
}

func (a *API) respondError(w http.ResponseWriter, r *http.Request, err error) {
	var requestErr *requestError
	switch {
	case errors.As(err, &requestErr):
		writeError(w, requestErr.status, requestErr.message)
	case errors.Is(err, storage.ErrNotFound):
		writeError(w, http.StatusNotFound, "record not found")
	case errors.Is(err, storage.ErrInvalidReference):
		writeError(w, http.StatusBadRequest, "platform, novel or volume does not exist, author does not belong to platform, or volume does not belong to novel")
	case errors.Is(err, storage.ErrConflict):
		writeError(w, http.StatusConflict, "record already exists")
	case errors.Is(err, storage.ErrVolumeNotEmpty):
		writeError(w, http.StatusConflict, "volume still contains chapters; move or delete them first")
	case errors.Is(err, context.DeadlineExceeded):
		writeError(w, http.StatusGatewayTimeout, "database request timed out")
	default:
		a.logger.Error("database request failed", "method", r.Method, "path", r.URL.Path, "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}
