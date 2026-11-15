package athubaccess

import (
	"context"
	"net/http"
	"net/url"

	"github.com/google/uuid"
)

type submissionIdentityKey struct{}
type submissionIdentity struct{ user, session uuid.UUID }

// SubmitUpload is called only after the account API validates the browser session
// and CSRF headers. The normal admission, audit and review pipeline still applies.
func (a *Access) SubmitUpload(w http.ResponseWriter, r *http.Request, user, session uuid.UUID) {
	if a.wrapped == nil || user == uuid.Nil || session == uuid.Nil {
		reply(w, 503, "submission_unavailable")
		return
	}
	request := r.Clone(context.WithValue(r.Context(), submissionIdentityKey{}, submissionIdentity{user, session}))
	request.Method = http.MethodPost
	request.URL = &url.URL{Path: "/api/v2/upload"}
	request.RequestURI = "/api/v2/upload"
	request.Header.Del("Authorization")
	a.wrapped.ServeHTTP(w, request)
}
