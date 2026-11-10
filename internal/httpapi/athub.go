package httpapi

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/AlbaTextus/oauth/internal/security"
	"github.com/AlbaTextus/oauth/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (a *API) athubActor(w http.ResponseWriter, r *http.Request) (store.User, store.ATHubStore, bool) {
	user, ok := a.accountActor(w, r)
	if !ok {
		return user, nil, false
	}
	st, ok := a.store.(store.ATHubStore)
	if !ok {
		writeJSON(w, 503, map[string]string{"error": "service_unavailable"})
	}
	return user, st, ok
}

func (a *API) listATHubKeys(w http.ResponseWriter, r *http.Request) {
	user, st, ok := a.athubActor(w, r)
	if !ok {
		return
	}
	keys, err := st.ListATHubKeys(r.Context(), user.ID)
	if err != nil {
		a.applicationError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"keys": keys})
}

func (a *API) createATHubKey(w http.ResponseWriter, r *http.Request) {
	a.provisionATHubKey(w, r, false)
}

func (a *API) resetATHubKey(w http.ResponseWriter, r *http.Request) {
	a.provisionATHubKey(w, r, true)
}

func (a *API) provisionATHubKey(w http.ResponseWriter, r *http.Request, reset bool) {
	user, st, ok := a.athubActor(w, r)
	if !ok {
		return
	}
	var input struct {
		Name *string `json:"name"`
	}
	if !readAccountJSON(w, r, &input) {
		return
	}
	name := "ATHub"
	if input.Name != nil {
		name = strings.TrimSpace(*input.Name)
	}
	if name == "" || utf8.RuneCountInString(name) > 100 || strings.ContainsRune(name, 0) {
		writeJSON(w, 400, map[string]string{"error": "invalid_key_name"})
		return
	}
	random, err := security.RandomToken(32)
	if err != nil {
		a.applicationError(w, err)
		return
	}
	token := "ath_" + random
	key := store.ATHubKey{ID: uuid.New(), UserID: user.ID, Name: name, TokenHash: security.HashToken(token), Prefix: token[:12]}
	created := true
	if reset {
		key, err = st.ResetATHubKey(r.Context(), key)
	} else {
		key, created, err = st.EnsureATHubKey(r.Context(), key)
	}
	if err != nil {
		if errors.Is(err, store.ErrATHubAdminDisabled) {
			writeJSON(w, 403, map[string]string{"error": "athub_admin_disabled", "error_description": "API 密钥已被管理员禁用，无法自行重置或启用。"})
			return
		}
		a.applicationError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	if !created {
		writeJSON(w, 200, map[string]any{"key": key})
		return
	}
	writeJSON(w, 201, map[string]any{"key": key, "api_key": token})
}

func (a *API) revokeATHubKey(w http.ResponseWriter, r *http.Request) {
	user, st, ok := a.athubActor(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid_key_id"})
		return
	}
	err = st.RevokeATHubKey(r.Context(), user.ID, id)
	if errors.Is(err, store.ErrNotFound) {
		writeJSON(w, 404, map[string]string{"error": "not_found"})
		return
	}
	if err != nil {
		a.applicationError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) athubUsage(w http.ResponseWriter, r *http.Request) {
	user, st, ok := a.athubActor(w, r)
	if !ok {
		return
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid_usage_range"})
		return
	}
	for key, values := range query {
		if key != "days" || len(values) != 1 {
			writeJSON(w, 400, map[string]string{"error": "invalid_usage_range"})
			return
		}
	}
	var usage store.ATHubUsage
	if values, exists := query["days"]; exists {
		days, parseErr := strconv.Atoi(values[0])
		if parseErr != nil || (days != 1 && days != 7 && days != 30) {
			writeJSON(w, 400, map[string]string{"error": "invalid_usage_range"})
			return
		}
		reader, supported := st.(store.ATHubUsageReader)
		if !supported {
			writeJSON(w, 503, map[string]string{"error": "service_unavailable"})
			return
		}
		bucket := time.Hour
		if days != 1 {
			bucket = 24 * time.Hour
		}
		until := time.Now().UTC().Truncate(bucket).Add(bucket)
		from := until.Add(-time.Duration(days) * 24 * time.Hour)
		usage, err = reader.ATHubUsageRange(r.Context(), user.ID, from, until, int(bucket.Seconds()))
	} else {
		usage, err = st.ATHubUsage(r.Context(), user.ID)
	}
	if err != nil {
		a.applicationError(w, err)
		return
	}
	writeJSON(w, 200, usage)
}

func (a *API) athubCalls(w http.ResponseWriter, r *http.Request) {
	user, st, ok := a.athubActor(w, r)
	if !ok {
		return
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid_pagination"})
		return
	}
	limit, offset := 50, 0
	for key, values := range query {
		if len(values) != 1 {
			err = errors.New("duplicate parameter")
			break
		}
		var value int
		value, err = strconv.Atoi(values[0])
		if err != nil {
			break
		}
		switch key {
		case "limit":
			limit = value
		case "offset":
			offset = value
		default:
			err = errors.New("unknown parameter")
		}
		if err != nil {
			break
		}
	}
	if err != nil || limit < 1 || limit > 100 || offset < 0 || offset > 1000000 {
		writeJSON(w, 400, map[string]string{"error": "invalid_pagination"})
		return
	}
	calls, err := st.ListATHubCalls(r.Context(), user.ID, limit, offset)
	if err != nil {
		a.applicationError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"calls": calls, "limit": limit, "offset": offset})
}
