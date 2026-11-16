package athubaccess

import (
	"context"
	"net/http"
	"net/url"
	"strconv"

	"github.com/google/uuid"
)

// Called by the account API after validating the browser session and image.
func (a *Access) SubmitIllustration(w http.ResponseWriter, r *http.Request, user, session uuid.UUID, novelID int32) {
	if a.wrapped == nil || user == uuid.Nil || session == uuid.Nil {
		reply(w, 503, "illustrations_unavailable")
		return
	}
	if novelID <= 0 {
		reply(w, 400, "invalid_novel_id")
		return
	}
	request := r.Clone(context.WithValue(r.Context(), submissionIdentityKey{}, submissionIdentity{user, session}))
	request.Method = http.MethodPost
	request.URL = &url.URL{Path: "/api/v2/illustrations", RawQuery: "novelId=" + strconv.FormatInt(int64(novelID), 10)}
	request.RequestURI = request.URL.String()
	request.Header.Del("Authorization")
	a.wrapped.ServeHTTP(w, request)
}

// Keep the parent novel in the request so a mismatched image cannot be removed.
func (a *Access) DeleteIllustration(w http.ResponseWriter, r *http.Request, user, session uuid.UUID, novelID, imageID int32) {
	if a.wrapped == nil || user == uuid.Nil || session == uuid.Nil {
		reply(w, 503, "illustrations_unavailable")
		return
	}
	if novelID <= 0 || imageID <= 0 {
		reply(w, 400, "invalid_illustration")
		return
	}
	request := r.Clone(context.WithValue(r.Context(), submissionIdentityKey{}, submissionIdentity{user, session}))
	request.Method = http.MethodDelete
	query := url.Values{"novelId": {strconv.FormatInt(int64(novelID), 10)}, "id": {strconv.FormatInt(int64(imageID), 10)}}
	request.URL = &url.URL{Path: "/api/v2/illustrations", RawQuery: query.Encode()}
	request.RequestURI = request.URL.String()
	request.Header.Del("Authorization")
	a.wrapped.ServeHTTP(w, request)
}

func (a *Access) SubmitIllustrationLayout(w http.ResponseWriter, r *http.Request, user, session uuid.UUID, novelID int32) {
	if a.wrapped == nil || user == uuid.Nil || session == uuid.Nil {
		reply(w, 503, "illustrations_unavailable")
		return
	}
	if novelID <= 0 {
		reply(w, 400, "invalid_novel_id")
		return
	}
	request := r.Clone(context.WithValue(r.Context(), submissionIdentityKey{}, submissionIdentity{user, session}))
	request.Method = http.MethodPut
	request.URL = &url.URL{Path: "/api/v2/illustration-layout", RawQuery: "novelId=" + strconv.FormatInt(int64(novelID), 10)}
	request.RequestURI = request.URL.String()
	request.Header.Del("Authorization")
	a.wrapped.ServeHTTP(w, request)
}
