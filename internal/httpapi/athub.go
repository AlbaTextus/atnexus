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
	user, st, ok := a.athubActor(w, r)
	if !ok {
		return
	}
	var input struct {
		Name      string     `json:"name"`
		ExpiresAt *time.Time `json:"expires_at"`
	}
	if !readAccountJSON(w, r, &input) {
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || utf8.RuneCountInString(input.Name) > 100 || strings.ContainsRune(input.Name, 0) {
		writeJSON(w, 400, map[string]string{"error": "invalid_key_name"})
		return
	}
	if input.ExpiresAt != nil && !input.ExpiresAt.After(time.Now()) {
		writeJSON(w, 400, map[string]string{"error": "invalid_expiry"})
		return
	}
	random, err := security.RandomToken(32)
	if err != nil {
		a.applicationError(w, err)
		return
	}
	token := "ath_" + random
	key, err := st.CreateATHubKey(r.Context(), store.ATHubKey{
		ID: uuid.New(), UserID: user.ID, Name: input.Name, TokenHash: security.HashToken(token), Prefix: token[:12], ExpiresAt: input.ExpiresAt,
	})
	if errors.Is(err, store.ErrATHubKeyLimit) {
		writeJSON(w, 409, map[string]string{"error": "api_key_limit", "error_description": "最多保留 20 个未过期且未撤销的密钥。"})
		return
	}
	if err != nil {
		a.applicationError(w, err)
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
	usage, err := st.ATHubUsage(r.Context(), user.ID)
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
