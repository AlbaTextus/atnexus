// Package apibase contains shared HTTP transport helpers, not module business rules.
package apibase

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"runtime"
	"strings"

	"github.com/AlbaTextus/atnexus/internal/config"
	"github.com/AlbaTextus/atnexus/internal/store"
	"github.com/AlbaTextus/atnexus/modules/oauth"
	"github.com/AlbaTextus/atnexus/pkg/auth"
)

// Decoding and rescaling one image can hold well over 100 MiB of pixels, so
// every module shares one small bound however many CPUs there are; further
// uploads wait holding just their encoded bytes.
var imageWork = make(chan struct{}, min(4, max(2, runtime.GOMAXPROCS(0))))

// AcquireImageWork waits for a turn to decode an image and returns the function
// that ends it. Callers validate cheaply first, so images that will be refused
// never queue, and never wait for a second turn while holding one. A request
// that timed out or was abandoned stops waiting.
func AcquireImageWork(ctx context.Context) (release func(), err error) {
	select {
	case imageWork <- struct{}{}:
		return func() { <-imageWork }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Store remains a transitional adapter for the unchanged account schema.
type Base struct {
	Config config.Config
	Store  store.Store
	Log    *slog.Logger
	Images ImageSaver
}

// ImageSaver is the image module in this process. It stores bytes under an
// upload category and entity, and returns the public file URL.
type ImageSaver interface {
	Save(ctx context.Context, category, entityID, fileName, contentType string, data []byte) (string, error)
}

func (a *Base) CurrentUser(r *http.Request) (store.User, bool) {
	identity, err := oauth.NewSessions(a.Store, a.Config.SessionCookieName).Authenticate(r)
	return identity.User, err == nil
}

func (a *Base) AccountActor(w http.ResponseWriter, r *http.Request) (store.User, bool) {
	user, _, ok := a.SessionActor(w, r)
	return user, ok
}

// SessionActor also returns the session that authenticated the request, already
// checked for revocation, expiry and ownership, so handlers never look it up again.
func (a *Base) SessionActor(w http.ResponseWriter, r *http.Request) (store.User, store.Session, bool) {
	w.Header().Set("Cache-Control", "no-store")
	identity, err := oauth.NewSessions(a.Store, a.Config.SessionCookieName).Authenticate(r)
	if err != nil {
		status, code := http.StatusServiceUnavailable, "authentication_unavailable"
		if errors.Is(err, auth.ErrUnauthenticated) {
			status, code = http.StatusUnauthorized, "login_required"
		}
		WriteJSON(w, status, map[string]string{"error": code})
		return store.User{}, store.Session{}, false
	}
	if r.Method != http.MethodGet && (r.Header.Get("X-AlbaTextus-Client") != "account-center" || r.Header.Get("Sec-Fetch-Site") == "cross-site") {
		WriteJSON(w, 403, map[string]string{"error": "invalid_origin"})
		return identity.User, identity.Session, false
	}
	return identity.User, identity.Session, true
}

func ReadJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") || d.Decode(target) != nil || d.Decode(&struct{}{}) != io.EOF {
		WriteJSON(w, 400, map[string]string{"error": "invalid_request", "error_description": "请提交有效的账户信息。"})
		return false
	}
	return true
}

func (a *Base) ApplicationError(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrApplicationHasPayments) {
		WriteJSON(w, 409, map[string]string{"error": "application_has_payments", "error_description": "应用已有支付记录，请停用应用以保留订单和流水。"})
		return
	}
	if errors.Is(err, store.ErrNotFound) {
		WriteJSON(w, 404, map[string]string{"error": "application_not_found"})
		return
	}
	a.Log.Error("application management", "error", err)
	WriteJSON(w, 500, map[string]string{"error": "server_error"})
}

func (a *Base) ATHubActor(w http.ResponseWriter, r *http.Request) (store.User, store.ATHubStore, bool) {
	user, _, st, ok := a.athubActor(w, r)
	return user, st, ok
}

// ATHubSessionActor is ATHubActor for handlers that act on behalf of the session.
func (a *Base) ATHubSessionActor(w http.ResponseWriter, r *http.Request) (store.User, store.Session, bool) {
	user, session, _, ok := a.athubActor(w, r)
	return user, session, ok
}

func (a *Base) athubActor(w http.ResponseWriter, r *http.Request) (store.User, store.Session, store.ATHubStore, bool) {
	user, session, ok := a.SessionActor(w, r)
	if !ok {
		return user, session, nil, false
	}
	st, ok := a.Store.(store.ATHubStore)
	if !ok {
		WriteJSON(w, 503, map[string]string{"error": "service_unavailable"})
	}
	return user, session, st, ok
}

func (a *Base) PolicyError(w http.ResponseWriter, err error) {
	status, code, message := 503, "permissions_unavailable", "暂时无法处理权限或审核，请稍后重试。"
	switch {
	case errors.Is(err, store.ErrUserForbidden), errors.Is(err, store.ErrATHubPermissionDenied):
		status, code, message = 403, "operation_forbidden", "当前账户没有此操作权限，或提交者的权限已发生变化。"
	case errors.Is(err, store.ErrNotFound):
		status, code, message = 404, "review_not_found", "没有找到这项提交。"
	case errors.Is(err, store.ErrConflict):
		status, code, message = 409, "review_conflict", "记录已被处理或配置已更新，请刷新后重试。"
	case errors.Is(err, store.ErrInvalidATHubKeyLimit):
		status, code, message = 400, "invalid_policy", "请提交完整且有效的权限配置。"
	default:
		a.Log.Error("ATHub permissions or review failed", "error", err)
	}
	WriteJSON(w, status, map[string]string{"error": code, "error_description": message})
}

func IsAdmin(user store.User) bool { return user.Role == "ADMIN" || user.Role == "SUPER_ADMIN" }

func WriteJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func WriteError(w http.ResponseWriter, status int, code string) {
	WriteJSON(w, status, map[string]string{"error": code})
}
