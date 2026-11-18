package athubaccess

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/AlbaTextus/atnexus/internal/store"
	"github.com/google/uuid"
)

// Personal-key uploads through the creator platform. Unlike the session
// forwards, the caller is not authenticated yet: the call is admitted here
// first, so the key is checked, counted and recorded before a file is read or
// stored.

// UploadCoverWithKey admits a cover upload under its own permission rule, then
// lets save store the image for the key's user. A cover changes no content, so
// it is never queued for review.
func (a *Access) UploadCoverWithKey(w http.ResponseWriter, r *http.Request, save func(http.ResponseWriter, *http.Request, uuid.UUID)) {
	request := r.Clone(r.Context())
	request.Method = http.MethodPost
	request.URL = &url.URL{Path: store.ATHubCoverPath}
	request.RequestURI = request.URL.String()
	a.admit(func(w http.ResponseWriter, r *http.Request, call store.ATHubCall) {
		save(w, r, *call.UserID)
	}).ServeHTTP(w, request)
}

// UploadIllustrationWithKey admits an illustration upload. Once admitted, save
// stores the image and returns its novel with the JSON the content API expects,
// or false when it has already replied. Review and history then apply as they
// do to every illustration.
func (a *Access) UploadIllustrationWithKey(w http.ResponseWriter, r *http.Request, save func(http.ResponseWriter, *http.Request) (int32, []byte, bool)) {
	if a.content == nil {
		reply(w, 503, "illustrations_unavailable")
		return
	}
	request := r.Clone(r.Context())
	request.Method = http.MethodPost
	request.URL = &url.URL{Path: "/api/v2/illustrations"}
	request.RequestURI = request.URL.String()
	a.admit(func(w http.ResponseWriter, r *http.Request, call store.ATHubCall) {
		novelID, body, ok := save(w, r)
		if !ok {
			return
		}
		r.URL.RawQuery = "novelId=" + strconv.FormatInt(int64(novelID), 10)
		r.RequestURI = r.URL.String()
		r.Body = io.NopCloser(bytes.NewReader(body))
		r.ContentLength = int64(len(body))
		r.Header.Set("Content-Type", "application/json")
		a.dispatch(w, r, call, a.content)
	}).ServeHTTP(w, request)
}
