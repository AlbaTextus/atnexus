// Package image is the Go image backend. PostgreSQL owns metadata; file bytes
// stay in configured local/S3 stores. It never serves the previous frontend.
package image

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/AlbaTextus/atnexus/pkg/auth"
	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type KeyVerifier interface {
	AuthorizeImageKey(context.Context, string, string) (string, error)
}
type Dependencies struct {
	Sessions auth.Verifier
	Keys     KeyVerifier
	Logger   *slog.Logger
	// Optional shared transport for S3, e.g. for tracing or controlled egress.
	StorageHTTPClient *http.Client
}
type Module struct {
	cfg        Config
	pool       *pgxpool.Pool
	local      *localStore
	scratch    *os.Root
	deps       Dependencies
	chunkSlots chan struct{}
	s3Clients  sync.Map
	settings   settingCache
	lifetime   context.Context
	stop       context.CancelFunc
	done       chan struct{}
}

func New(ctx context.Context, cfg Config, deps Dependencies) (*Module, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if deps.Sessions == nil || deps.Keys == nil {
		return nil, errors.New("OAuth session and image key authorities are required")
	}
	if deps.Logger == nil {
		deps.Logger = slog.Default()
	}
	if cfg.MaxSessionsPerOwner == 0 {
		cfg.MaxSessionsPerOwner = defaultSessionsPerOwner
	}
	p, err := openPool(ctx, cfg.DatabaseURL, cfg.MaxConns)
	if err != nil {
		return nil, err
	}
	fail := func(err error) (*Module, error) { p.Close(); return nil, err }
	// A configured data root must already exist: never silently create an empty
	// directory when a migration path or mounted volume was mistyped.
	root, err := os.OpenRoot(cfg.LocalRoot)
	if err != nil {
		return fail(errors.New("IMAGE_LOCAL_ROOT is unavailable"))
	}
	if err = os.MkdirAll(cfg.TempRoot, 0700); err != nil {
		root.Close()
		return fail(err)
	}
	scratch, err := os.OpenRoot(cfg.TempRoot)
	if err != nil {
		root.Close()
		return fail(err)
	}
	// A merge holds its session lock while committing a durable file receipt.
	// Bound such operations so they cannot occupy the entire metadata pool.
	m := &Module{cfg: cfg, pool: p, local: &localStore{root}, scratch: scratch, deps: deps, chunkSlots: make(chan struct{}, max(1, int(p.Config().MaxConns/3))), done: make(chan struct{})}
	background, stop := context.WithCancel(context.Background())
	m.lifetime, m.stop = background, stop
	go m.janitor(background)
	return m, nil
}

// detached bounds work that must finish after its request is gone, such as
// removing bytes whose row is already committed as deleting. It outlives the
// request but not the module, so shutdown is never held up by it.
func (m *Module) detached(ctx context.Context, limit time.Duration) (context.Context, context.CancelFunc) {
	out, cancel := context.WithTimeout(context.WithoutCancel(ctx), limit)
	release := context.AfterFunc(m.lifetime, cancel)
	return out, func() { release(); cancel() }
}
func (m *Module) Close() {
	m.stop()
	<-m.done
	m.scratch.Close()
	m.local.root.Close()
	m.pool.Close()
}
func (m *Module) Ping(ctx context.Context) error {
	if err := m.pool.Ping(ctx); err != nil {
		return err
	}
	_, err := m.local.root.Stat(".")
	return err
}
func (m *Module) Router() http.Handler {
	r := chi.NewRouter()
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if m.Ping(r.Context()) != nil {
			respondError(w, 503, "image_storage_unavailable")
			return
		}
		respond(w, 200, map[string]string{"status": "ok"})
	})
	r.Handle("/api/v2/upload", m.authorize("upload", http.HandlerFunc(m.upload)))
	for _, category := range []string{"user", "user/avatars", "user/backgrounds", "book", "book/covers", "book/illustrations", "forum"} {
		r.Handle("/api/v2/categories/"+category+"/upload", m.authorize("upload", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { m.categoryUpload(w, r, category) })))
		r.Handle("/api/v2/categories/"+category+"/list", m.authorize("list", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { m.list(w, r, category) })))
	}
	r.Handle("/api/v2/manage/list", m.authorize("list", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { m.list(w, r, "") })))
	r.Handle("/api/v2/directoryTree", m.authorize("list", http.HandlerFunc(m.directoryTree)))
	r.Handle("/api/v2/channels", m.authorize("upload", http.HandlerFunc(m.channelList)))
	r.Handle("/api/v2/manage/quota", m.authorize("manage", http.HandlerFunc(m.quota)))
	r.Handle("/api/v2/manage/sysConfig/upload", m.authorize("manage", http.HandlerFunc(m.uploadSettings)))
	r.Handle("/api/v2/manage/delete/batch", m.authorize("delete", http.HandlerFunc(m.batchDelete)))
	for _, op := range []string{"delete", "metadata", "rename", "move", "block", "unblock", "tags"} {
		permission := "manage"
		if op == "delete" {
			permission = "delete"
		}
		r.Handle("/api/v2/manage/"+op+"/*", m.authorize(permission, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { m.fileAction(w, r, op, chi.URLParam(r, "*")) })))
	}
	r.Handle("/api/v2/manage/tags/autocomplete", m.authorize("list", http.HandlerFunc(m.tagsAutocomplete)))
	r.Handle("/api/v2/manage/tags/batch", m.authorize("manage", http.HandlerFunc(m.batchTags)))
	r.Handle("/api/v2/manage/batch/list", m.authorize("manage", http.HandlerFunc(m.exportFiles)))
	r.Handle("/api/v2/manage/batch/settings", m.authorize("manage", http.HandlerFunc(m.settingsBackup)))
	r.Handle("/api/v2/manage/batch/restore/chunk", m.authorize("manage", http.HandlerFunc(m.restoreFiles)))
	r.Handle("/api/v2/manage/cusConfig/blockipList", m.authorize("manage", http.HandlerFunc(m.blockedIPs)))
	return r
}

type actorKey struct{}
type actor struct {
	ID    string
	Admin bool
}

func (m *Module) authorize(permission string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var who actor
		header := r.Header.Get("Authorization")
		if header != "" {
			if len(r.Header.Values("Authorization")) != 1 {
				respondError(w, 401, "invalid_key")
				return
			}
			raw := strings.TrimPrefix(header, "Bearer ")
			id, err := m.deps.Keys.AuthorizeImageKey(r.Context(), raw, permission)
			if err != nil {
				switch {
				case errors.Is(err, auth.ErrUnauthenticated):
					respondError(w, 401, "invalid_key")
				case errors.Is(err, auth.ErrForbidden):
					respondError(w, 403, "permission_denied")
				default:
					respondError(w, 503, "authentication_unavailable")
				}
				return
			}
			who = actor{ID: "key:" + id, Admin: permission == "manage"}
		} else {
			id, err := m.deps.Sessions.VerifySession(r)
			if err != nil {
				if errors.Is(err, auth.ErrUnauthenticated) {
					respondError(w, 401, "login_required")
				} else {
					respondError(w, 503, "authentication_unavailable")
				}
				return
			}
			who = actor{ID: "user:" + id.UserID.String(), Admin: id.Role == "ADMIN" || id.Role == "SUPER_ADMIN"}
			if permission != "upload" && !who.Admin {
				respondError(w, 403, "permission_denied")
				return
			}
			if r.Method != http.MethodGet && r.Method != http.MethodHead && (r.Header.Get("X-AlbaTextus-Client") != "account-center" || r.Header.Get("Sec-Fetch-Site") == "cross-site") {
				respondError(w, 403, "invalid_origin")
				return
			}
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), actorKey{}, who)))
	})
}

func respond(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func respondError(w http.ResponseWriter, status int, code string) {
	respond(w, status, map[string]string{"error": code})
}
func (m *Module) failure(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		respondError(w, 404, "image_not_found")
	case errors.Is(err, ErrConflict):
		respondError(w, 409, "image_conflict")
	case errors.Is(err, ErrInvalid):
		respondError(w, 400, "invalid_image_request")
	default:
		m.deps.Logger.Error("image operation failed", "error", safeError(err))
		respondError(w, 503, "image_storage_unavailable")
	}
}
func safeError(err error) string {
	if err == nil {
		return ""
	}
	var remote *smithyhttp.ResponseError
	if errors.As(err, &remote) {
		return fmt.Sprintf("remote HTTP %d", remote.HTTPStatusCode())
	}
	var operation *smithy.OperationError
	if errors.As(err, &operation) {
		return fmt.Sprintf("%s %s: %T", operation.ServiceID, operation.OperationName, operation.Unwrap())
	}
	return fmt.Sprintf("%T", err)
}
func text(m map[string]any, key string) string {
	if value, ok := m[key].(string); ok {
		return value
	}
	return ""
}
func decodeJSON(raw []byte, v any) error {
	d := json.NewDecoder(strings.NewReader(string(raw)))
	d.UseNumber()
	return d.Decode(v)
}
func method(w http.ResponseWriter, r *http.Request, allowed ...string) bool {
	for _, v := range allowed {
		if r.Method == v {
			return true
		}
	}
	w.Header().Set("Allow", strings.Join(allowed, ", "))
	respondError(w, 405, "method_not_allowed")
	return false
}
