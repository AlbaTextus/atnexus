package athubaccess

import (
	"context"
	"net/http"
	"net/url"
	"strconv"

	"github.com/google/uuid"
)

// SubmitNovelEdit uses the same admission, permissions, review and audit pipeline
// as API-key mutations. The browser cannot select another content endpoint.
func (a *Access) SubmitNovelEdit(w http.ResponseWriter, r *http.Request, user, session uuid.UUID, novelID int32) {
	if a.wrapped == nil || user == uuid.Nil || session == uuid.Nil {
		reply(w, 503, "editing_unavailable")
		return
	}
	if novelID <= 0 {
		reply(w, 400, "invalid_novel_id")
		return
	}
	request := r.Clone(context.WithValue(r.Context(), submissionIdentityKey{}, submissionIdentity{user, session}))
	request.Method = http.MethodPut
	request.URL = &url.URL{Path: "/api/v2/novelinfo", RawQuery: "id=" + strconv.FormatInt(int64(novelID), 10)}
	request.RequestURI = request.URL.String()
	request.Header.Del("Authorization")
	a.wrapped.ServeHTTP(w, request)
}
