// Package athubaccess authenticates and accounts for content calls in the account database.
package athubaccess

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/AlbaTextus/atnexus/internal/store"
	"github.com/AlbaTextus/atnexus/modules/athub"
	"github.com/AlbaTextus/atnexus/modules/oauth"
	"github.com/AlbaTextus/atnexus/pkg/auth"
	"github.com/google/uuid"
)

type Access struct {
	store          store.ATHubStore
	port           int
	logger         *slog.Logger
	readSecret     string
	content        http.Handler
	wrapped        http.Handler
	sessions       auth.Verifier
	keys           auth.APIKeyVerifier
	creatorHistory store.NovelHistoryStore
	creatorReviews store.ATHubReviewStore
}

// EnableSessions injects the OAuth session authority without sharing credentials.
func (a *Access) EnableSessions(verifier auth.Verifier) { a.sessions = verifier }

func (a *Access) EnableAPIKeys(verifier auth.APIKeyVerifier) { a.keys = verifier }

// Creator data is supplied explicitly; account policy/audit storage stays in
// the existing admission adapter. Failures never fall back to account history.
func (a *Access) EnableCreatorStorage(st interface {
	store.NovelHistoryStore
	store.ATHubReviewStore
}) {
	a.creatorHistory = st
	a.creatorReviews = st
}

func New(st store.ATHubStore, addr string, logger *slog.Logger) (*Access, error) {
	_, portText, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, errors.New("invalid ATHUB_HTTP_ADDR")
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return nil, errors.New("ATHUB_HTTP_ADDR must specify a port between 1 and 65535")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Access{store: st, port: port, logger: logger, keys: oauth.NewAPIKeys(st)}, nil
}

func tokenFrom(r *http.Request) string {
	if len(r.Header.Values("Authorization")) != 1 {
		return ""
	}
	scheme, token, ok := strings.Cut(r.Header.Get("Authorization"), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") || !strings.HasPrefix(token, "ath_") || len(token) != 47 {
		return ""
	}
	for _, c := range token[4:] {
		if !(c >= 'a' && c <= 'z') && !(c >= 'A' && c <= 'Z') && !(c >= '0' && c <= '9') && c != '_' && c != '-' {
			return ""
		}
	}
	return token
}

// Bound labels keep query-string credentials and arbitrary paths out of audit data.
func endpoint(path string) string {
	if store.ATHubKnownPath(path) {
		return path
	}
	return "/api/v2/{unknown}"
}

func method(method string) string {
	switch method {
	case "GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS", "CONNECT", "TRACE":
		return method
	default:
		return "OTHER"
	}
}

func reply(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code})
}

type responseCapture struct {
	http.ResponseWriter
	status int
	bytes  int64
}

func (w *responseCapture) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w *responseCapture) WriteHeader(status int) {
	if status >= 100 && status < 200 {
		w.ResponseWriter.WriteHeader(status)
		return
	}
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}
func (w *responseCapture) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	n, err := w.ResponseWriter.Write(data)
	w.bytes += int64(n)
	return n, err
}

func (a *Access) Wrap(next http.Handler) http.Handler {
	a.content = next
	a.wrapped = a.admit(func(w http.ResponseWriter, r *http.Request, call store.ATHubCall) {
		a.dispatch(w, r, call, next)
	})
	return a.wrapped
}

// admit authenticates a call and records it before serve answers it, so every
// way into the pipeline shares one admission, audit and completion path.
func (a *Access) admit(serve func(http.ResponseWriter, *http.Request, store.ATHubCall)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		requestID := uuid.New()
		w.Header().Set("X-Request-ID", requestID.String())
		call := store.ATHubCall{ID: uuid.New(), RequestID: requestID, Method: method(r.Method), Path: endpoint(r.URL.Path), Port: a.port}
		if identity, ok := r.Context().Value(submissionIdentityKey{}).(submissionIdentity); ok && store.ATHubSubmissionEndpoint(r.Method, r.URL.Path) {
			call.UserID, call.SessionID = &identity.user, &identity.session
			call.Authenticated = true
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		token := tokenFrom(r)
		// Explicit bearer credentials retain their existing authentication behavior.
		// Browser calls use the OAuth cookie and never receive a second token.
		if a.sessions != nil && !call.Authenticated && r.Header.Get("Authorization") == "" {
			identity, err := a.sessions.VerifySession(r.WithContext(ctx))
			if err != nil && !errors.Is(err, auth.ErrUnauthenticated) {
				cancel()
				reply(w, 503, "authentication_unavailable")
				return
			}
			if err == nil {
				if r.Method != http.MethodGet && (r.Header.Get("X-AlbaTextus-Client") != "account-center" || r.Header.Get("Sec-Fetch-Site") == "cross-site") {
					cancel()
					reply(w, 403, "invalid_origin")
					return
				}
				call.UserID, call.SessionID, call.Authenticated = &identity.UserID, &identity.SessionID, true
				r = r.WithContext(auth.WithIdentity(r.Context(), identity))
			}
		}
		if len(r.Header.Values("Authorization")) == 1 && r.Method == http.MethodGet && a.readSecret != "" {
			scheme, credential, ok := strings.Cut(r.Header.Get("Authorization"), " ")
			if ok && strings.EqualFold(scheme, "Bearer") {
				if identity, valid := parseReadSession(a.readSecret, credential); valid {
					call.UserID = &identity.UserID
					call.SessionID = &identity.SessionID
					call.Authenticated = true
				}
			}
		}
		if token != "" && !call.Authenticated {
			identity, err := a.keys.ResolveAPIKey(ctx, token)
			if err != nil && !errors.Is(err, store.ErrNotFound) {
				cancel()
				a.logger.Error("ATHub key lookup failed", "request_id", requestID)
				reply(w, 503, "authentication_unavailable")
				return
			}
			if err == nil {
				call.KeyID = &identity.KeyID
				call.UserID = &identity.UserID
				call.Authenticated = identity.Active
			}
		}
		// A pending row is durable before dispatch. An accounting outage cannot
		// produce an unrecorded write; crashes retain a row with status=null.
		err := a.store.BeginATHubCall(ctx, call)
		cancel()
		limited := errors.Is(err, store.ErrATHubCallLimit)
		forbidden := errors.Is(err, store.ErrATHubPermissionDenied)
		if forbidden {
			err = nil
		}
		if limited {
			err = nil
		}
		if errors.Is(err, store.ErrATHubAccessDenied) {
			call.Authenticated = false
			err = nil
		}
		if err != nil {
			a.logger.Error("ATHub call recording unavailable", "request_id", requestID)
			reply(w, 503, "accounting_unavailable")
			return
		}
		capture := &responseCapture{ResponseWriter: w}
		defer func() {
			if recover() != nil {
				a.logger.Error("ATHub handler panicked", "request_id", requestID)
				if capture.status == 0 {
					reply(capture, 500, "internal_server_error")
				}
			}
			if capture.status == 0 {
				capture.WriteHeader(http.StatusOK)
			}
			// Client cancellation must not cancel audit completion.
			finish, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 5*time.Second)
			defer cancel()
			if err := a.store.FinishATHubCall(finish, call.ID, capture.status, time.Since(start).Milliseconds(), capture.bytes); err != nil {
				a.logger.Error("ATHub call completion failed; pending audit retained", "request_id", requestID)
			}
		}()
		if forbidden {
			reply(capture, http.StatusForbidden, "operation_forbidden")
			return
		}
		if limited {
			reply(capture, http.StatusTooManyRequests, "api_key_total_call_limit_reached")
			return
		}
		if !call.Authenticated {
			capture.Header().Set("WWW-Authenticate", "Bearer")
			reply(capture, 401, "invalid_api_key")
			return
		}
		serve(capture, r, call)
	})
}

// dispatch answers an admitted call: history directly, content writes behind
// the contributor lookup and the review queue.
func (a *Access) dispatch(w http.ResponseWriter, r *http.Request, call store.ATHubCall, next http.Handler) {
	if r.URL.Path == "/api/v3/history" || r.URL.Path == "/api/v3/contributors" {
		a.history(w, r)
		return
	}
	if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodDelete || r.Method == http.MethodPatch {
		actor, err := a.contributor(r.Context(), *call.UserID)
		if err != nil {
			reply(w, 503, "contributor_unavailable")
			return
		}
		r = r.WithContext(athub.WithContributor(r.Context(), actor))
		if a.queueMutation(w, r, call, actor) {
			return
		}
	}
	next.ServeHTTP(w, r)
}
