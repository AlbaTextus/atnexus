package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/AlbaTextus/atnexus/internal/store"
)

func (a *API) completeProfileSetup(w http.ResponseWriter, r *http.Request) {
	user, ok := a.AccountActor(w, r)
	if !ok {
		return
	}
	var input struct {
		Username string `json:"username"`
	}
	if !readAccountJSON(w, r, &input) {
		return
	}
	name := strings.TrimSpace(input.Username)
	if name == "" || utf8.RuneCountInString(name) > 100 || strings.IndexFunc(name, unicode.IsControl) >= 0 {
		writeJSON(w, 400, map[string]string{"error": "invalid_username", "error_description": "用户名需为 1–100 个字符，不能包含换行等控制字符。"})
		return
	}
	st, ok := a.Store.(store.ProfileSetupStore)
	if !ok {
		writeJSON(w, 503, map[string]string{"error": "service_unavailable"})
		return
	}
	updated, err := st.CompleteProfileSetup(r.Context(), user.ID, name)
	if errors.Is(err, store.ErrConflict) {
		writeJSON(w, 409, map[string]string{"error": "profile_setup_completed", "error_description": "用户名已设置，请继续进入用户中心。"})
		return
	}
	if err != nil {
		a.ApplicationError(w, err)
		return
	}
	writeJSON(w, 200, userView(updated))
}
