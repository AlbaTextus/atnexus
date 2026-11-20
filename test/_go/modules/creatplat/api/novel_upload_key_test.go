package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/jpeg"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/AlbaTextus/atnexus/internal/store"
	"github.com/AlbaTextus/atnexus/modules/athub"
	athubaccess "github.com/AlbaTextus/atnexus/modules/athub/access"
	"github.com/google/uuid"
)

// uploadStore is the account store behind the upload endpoints: it answers the
// session permission checks and records what the ATHub pipeline admits.
type uploadStore struct {
	*submitAPIStore
	identity                          store.ATHubIdentity
	checkErr, beginErr                error
	coverChecked, illustrationChecked uuid.UUID
	call                              store.ATHubCall
	status                            int
}

func (s *uploadStore) CheckATHubCoverUpload(_ context.Context, user uuid.UUID) error {
	s.coverChecked = user
	return s.checkErr
}
func (s *uploadStore) CheckATHubIllustrationUpload(_ context.Context, user uuid.UUID) error {
	s.illustrationChecked = user
	return s.checkErr
}
func (s *uploadStore) ResolveATHubKey(context.Context, string) (store.ATHubIdentity, error) {
	return s.identity, nil
}
func (s *uploadStore) BeginATHubCall(_ context.Context, call store.ATHubCall) error {
	s.call = call
	return s.beginErr
}
func (s *uploadStore) FinishATHubCall(_ context.Context, _ uuid.UUID, status int, _, _ int64) error {
	s.status = status
	return nil
}

// reviewedUploadStore puts the caller's level under review.
type reviewedUploadStore struct {
	*uploadStore
	store.ATHubPolicyStore
}

func (s *reviewedUploadStore) ATHubPolicy(context.Context, string) (store.ATHubRolePolicy, error) {
	return store.ATHubRolePolicy{ReviewRequired: true, Rules: []store.ATHubAccessRule{{Operation: "upload", Allowed: true}}}, nil
}

// uploadReviews stands in for the creator database's review queue: it keeps
// the change that was last sent to review.
type uploadReviews struct {
	store.NovelHistoryStore
	store.ATHubReviewStore
	queued *store.ATHubReview
}

func (q *uploadReviews) QueueATHubReview(_ context.Context, item store.ATHubReview) (store.ATHubReview, error) {
	q.queued = &item
	return item, nil
}

// imageHost stands in for the built-in image host.
type imageHost struct {
	saved                      int
	uploader, category, entity string
	removed                    []string
}

func (h *imageHost) Save(_ context.Context, uploader, category, entityID, _, _ string, _ []byte) (string, error) {
	h.saved++
	h.uploader, h.category, h.entity = uploader, category, entityID
	return "https://images.test/" + category + "/" + entityID + "/1.jpg", nil
}

func (h *imageHost) Remove(_ context.Context, category, entityID, location string) error {
	h.removed = append(h.removed, category+" "+entityID+" "+location)
	return nil
}

type uploadFixture struct {
	api   *API
	store *uploadStore
	host  *imageHost
	// What was sent to review, if anything was.
	reviews *uploadReviews
	// What the content API received, if the upload reached it.
	content *http.Request
	body    []byte
	// What the content API answers; 201 unless a test says otherwise.
	contentStatus int
	// How often a change was rehearsed, and what a rehearsal answers (200).
	rehearsed       int
	rehearsalStatus int
}

func newUploadFixture(t *testing.T) *uploadFixture {
	t.Helper()
	a, keys := keyAPI(t)
	f := &uploadFixture{api: a, host: &imageHost{}, reviews: &uploadReviews{}, store: &uploadStore{
		submitAPIStore: &submitAPIStore{keyStore: keys, session: store.Session{
			ID: uuid.New(), UserID: keys.user.ID, ExpiresAt: time.Now().Add(time.Hour),
		}},
		identity: store.ATHubIdentity{KeyID: uuid.New(), UserID: uuid.New(), Active: true},
	}}
	a.Store, a.Images = f.store, f.host
	f.wire(t, f.store)
	return f
}

// wire connects the endpoints to a real ATHub access pipeline over st.
func (f *uploadFixture) wire(t *testing.T, st store.ATHubStore) {
	t.Helper()
	access := athubaccess.New(st, f.api.Log)
	access.EnableCreatorStorage(f.reviews)
	access.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// A reviewed change is rehearsed first; only a real call is recorded.
		if athub.Rehearsing(r.Context()) {
			f.rehearsed++
			status := 200
			if f.rehearsalStatus != 0 {
				status = f.rehearsalStatus
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"error":"novel_not_found"}`))
			return
		}
		f.content = r
		f.body, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		if f.contentStatus != 0 {
			w.WriteHeader(f.contentStatus)
			_, _ = w.Write([]byte(`{"error":"refused"}`))
			return
		}
		w.WriteHeader(201)
		_, _ = w.Write([]byte(`{"id":5}`))
	}))
	f.api.SetATHubIllustrationSubmitter(access.SubmitIllustration)
	f.api.SetATHubKeyCoverUploader(access.UploadCoverWithKey)
	f.api.SetATHubKeyIllustrationUploader(access.UploadIllustrationWithKey)
}

func (f *uploadFixture) serve(r *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	f.api.Router().ServeHTTP(w, r)
	return w
}

const (
	coverTarget        = "/api/v4/account/athub/cover"
	illustrationTarget = "/api/v4/account/athub/novels/17/illustrations"
)

func picture(t *testing.T) []byte {
	t.Helper()
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, image.NewRGBA(image.Rect(0, 0, 4, 3)), nil); err != nil {
		t.Fatal(err)
	}
	return encoded.Bytes()
}

// uploadRequest posts each file under the form field "file".
func uploadRequest(t *testing.T, target string, files ...[]byte) *http.Request {
	t.Helper()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	for _, file := range files {
		part, err := form.CreateFormFile("file", "picture.jpg")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write(file); err != nil {
			t.Fatal(err)
		}
	}
	if err := form.Close(); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", target, &body)
	r.Header.Set("Content-Type", form.FormDataContentType())
	return r
}

func withKey(r *http.Request) *http.Request {
	r.Header.Set("Authorization", "Bearer ath_"+strings.Repeat("a", 43))
	return r
}

func withSession(r *http.Request) *http.Request {
	r.AddCookie(&http.Cookie{Name: "session", Value: "logged-in"})
	r.Header.Set("X-AlbaTextus-Client", "account-center")
	return r
}

type storedIllustration struct {
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

// recorded checks that the content API was handed the stored illustration of novel 17.
func (f *uploadFixture) recorded(t *testing.T) {
	t.Helper()
	if f.content == nil {
		t.Fatal("illustration never reached the content API")
	}
	var stored storedIllustration
	if err := json.Unmarshal(f.body, &stored); err != nil {
		t.Fatal(err)
	}
	if f.content.Method != "POST" || f.content.URL.Path != "/api/v2/illustrations" || f.content.URL.RawQuery != "novelId=17" ||
		stored.URL != "https://images.test/book/illustrations/17/1.jpg" || len(stored.SHA256) != 64 || stored.Width != 4 || stored.Height != 3 {
		t.Fatalf("content API received %s %s body=%s", f.content.Method, f.content.URL, f.body)
	}
}

func TestCoverUploadAcceptsAPersonalKey(t *testing.T) {
	f := newUploadFixture(t)
	w := f.serve(withKey(uploadRequest(t, coverTarget, picture(t))))
	var reply struct {
		URL string `json:"url"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &reply) != nil {
		t.Fatalf("status=%d body=%s", w.Code, w.Body)
	}
	owner := f.store.identity.UserID.String()
	if f.host.saved != 1 || f.host.category != "book/covers" || f.host.entity != owner || reply.URL != "https://images.test/book/covers/"+owner+"/1.jpg" {
		t.Fatalf("cover was not stored for the key's user: %+v url=%s", f.host, reply.URL)
	}
	if f.store.call.Path != store.ATHubCoverPath || f.store.call.KeyID == nil || *f.store.call.KeyID != f.store.identity.KeyID || f.store.status != 200 {
		t.Fatalf("key call was not recorded: %+v status=%d", f.store.call, f.store.status)
	}
	if f.store.coverChecked != uuid.Nil || f.content != nil {
		t.Fatal("key upload took the session path or reached the content API")
	}
}

func TestIllustrationUploadAcceptsAPersonalKey(t *testing.T) {
	f := newUploadFixture(t)
	w := f.serve(withKey(uploadRequest(t, illustrationTarget, picture(t))))
	if w.Code != 201 || strings.TrimSpace(w.Body.String()) != `{"id":5}` {
		t.Fatalf("status=%d body=%s", w.Code, w.Body)
	}
	f.recorded(t)
	if f.host.saved != 1 || f.host.category != "book/illustrations" || f.host.entity != "17" {
		t.Fatalf("illustration was not stored under its novel: %+v", f.host)
	}
	if f.store.call.Path != "/api/v2/illustrations" || f.store.call.KeyID == nil || *f.store.call.KeyID != f.store.identity.KeyID ||
		f.store.call.SessionID != nil || f.store.status != 201 {
		t.Fatalf("key call was not recorded: %+v status=%d", f.store.call, f.store.status)
	}
	if f.store.illustrationChecked != uuid.Nil {
		t.Fatal("key upload took the session path")
	}
	if f.host.uploader != f.store.identity.UserID.String() || len(f.host.removed) != 0 {
		t.Fatalf("uploader=%q removed=%v", f.host.uploader, f.host.removed)
	}
}

// Only the content API knows whether the novel exists. When it refuses the
// illustration, the image stored for it must not stay behind.
func TestRefusedIllustrationIsRemovedFromTheImageHost(t *testing.T) {
	const stored = "book/illustrations 17 https://images.test/book/illustrations/17/1.jpg"
	for _, scenario := range []struct {
		name    string
		status  int
		removed bool
	}{
		{"unknown novel", 404, true},
		{"rejected content", 400, true},
		// The outcome is unknown: the illustration may have been recorded.
		{"content failure", 503, false},
	} {
		for name, authenticate := range map[string]func(*http.Request) *http.Request{"key": withKey, "session": withSession} {
			t.Run(scenario.name+" with "+name, func(t *testing.T) {
				f := newUploadFixture(t)
				f.contentStatus = scenario.status
				w := f.serve(authenticate(uploadRequest(t, illustrationTarget, picture(t))))
				if w.Code != scenario.status || f.host.saved != 1 {
					t.Fatalf("status=%d saved=%d body=%s", w.Code, f.host.saved, w.Body)
				}
				if scenario.removed != (len(f.host.removed) == 1 && f.host.removed[0] == stored) {
					t.Fatalf("removed=%v", f.host.removed)
				}
			})
		}
	}
}

func TestKeyIllustrationUploadIsReviewed(t *testing.T) {
	f := newUploadFixture(t)
	f.wire(t, &reviewedUploadStore{uploadStore: f.store})
	w := f.serve(withKey(uploadRequest(t, illustrationTarget, picture(t))))
	if w.Code != 202 || f.content != nil || f.host.saved != 1 {
		t.Fatalf("status=%d body=%s stored=%d", w.Code, w.Body, f.host.saved)
	}
	var stored storedIllustration
	queued := f.reviews.queued
	if queued == nil || json.Unmarshal(queued.Payload, &stored) != nil {
		t.Fatalf("illustration was not queued: %+v", queued)
	}
	if queued.ID != f.store.call.ID || queued.UserID == nil || *queued.UserID != f.store.identity.UserID ||
		queued.Path != "/api/v2/illustrations" || queued.Query != "novelId=17" ||
		stored.URL != "https://images.test/book/illustrations/17/1.jpg" {
		t.Fatalf("review lost the novel or image: %+v", queued)
	}
	if f.rehearsed != 1 || len(f.host.removed) != 0 {
		t.Fatalf("rehearsed=%d removed=%v", f.rehearsed, f.host.removed)
	}
}

// A change the content API would refuse today is answered at once: it is not
// queued for a reviewer, and the image stored for it does not stay behind.
func TestReviewedUploadIsRefusedBeforeItIsQueued(t *testing.T) {
	f := newUploadFixture(t)
	f.wire(t, &reviewedUploadStore{uploadStore: f.store})
	f.rehearsalStatus = 404
	w := f.serve(withKey(uploadRequest(t, illustrationTarget, picture(t))))
	if w.Code != 404 || !strings.Contains(w.Body.String(), "novel_not_found") {
		t.Fatalf("status=%d body=%s", w.Code, w.Body)
	}
	if f.reviews.queued != nil || f.content != nil {
		t.Fatal("a refused change was queued or applied")
	}
	if len(f.host.removed) != 1 {
		t.Fatalf("removed=%v", f.host.removed)
	}
}

// A refused key must cost nothing: no file is read into the image host, and a
// browser session sent along with it is not used instead.
func TestRefusedKeyStoresNothing(t *testing.T) {
	for _, target := range []string{coverTarget, illustrationTarget} {
		for _, scenario := range []struct {
			name     string
			auth     string
			inactive bool
			beginErr error
			want     int
		}{
			{"forbidden for the level", "", false, store.ErrATHubPermissionDenied, 403},
			{"total call limit reached", "", false, store.ErrATHubCallLimit, 429},
			{"revoked expired or disabled", "", true, nil, 401},
			{"not a personal key", "Bearer " + strings.Repeat("t", 64), false, nil, 401},
			{"another scheme", "Basic dXNlcjpwYXNz", false, nil, 401},
		} {
			t.Run(target+" "+scenario.name, func(t *testing.T) {
				f := newUploadFixture(t)
				f.store.beginErr, f.store.identity.Active = scenario.beginErr, !scenario.inactive
				request := withSession(withKey(uploadRequest(t, target, picture(t))))
				if scenario.auth != "" {
					request.Header.Set("Authorization", scenario.auth)
				}
				w := f.serve(request)
				if w.Code != scenario.want || f.host.saved != 0 || f.content != nil {
					t.Fatalf("status=%d body=%s stored=%d", w.Code, w.Body, f.host.saved)
				}
				if f.store.call.ID == uuid.Nil || f.store.status != scenario.want {
					t.Fatalf("refusal was not recorded: %+v status=%d", f.store.call, f.store.status)
				}
				if f.store.coverChecked != uuid.Nil || f.store.illustrationChecked != uuid.Nil {
					t.Fatal("refused key fell back to the browser session")
				}
			})
		}
	}
}

func TestKeyUploadChecksItsNovelAndImage(t *testing.T) {
	for _, scenario := range []struct {
		name, target string
		files        [][]byte
		code         string
	}{
		{"novel zero", "/api/v4/account/athub/novels/0/illustrations", [][]byte{picture(t)}, "invalid_novel_id"},
		{"novel not a number", "/api/v4/account/athub/novels/first/illustrations", [][]byte{picture(t)}, "invalid_novel_id"},
		{"novel out of range", "/api/v4/account/athub/novels/2147483648/illustrations", [][]byte{picture(t)}, "invalid_novel_id"},
		{"unexpected query", illustrationTarget + "?novelId=18", [][]byte{picture(t)}, "invalid_novel_id"},
		{"illustration is not an image", illustrationTarget, [][]byte{[]byte("not an image")}, "illustration_invalid"},
		{"two illustrations", illustrationTarget, [][]byte{picture(t), picture(t)}, "illustration_invalid"},
		{"no illustration", illustrationTarget, nil, "illustration_invalid"},
		{"cover is not an image", coverTarget, [][]byte{[]byte("not an image")}, "cover_invalid"},
		{"two covers", coverTarget, [][]byte{picture(t), picture(t)}, "cover_invalid"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			f := newUploadFixture(t)
			w := f.serve(withKey(uploadRequest(t, scenario.target, scenario.files...)))
			var reply struct {
				Error string `json:"error"`
			}
			if w.Code != 400 || json.Unmarshal(w.Body.Bytes(), &reply) != nil || reply.Error != scenario.code {
				t.Fatalf("status=%d body=%s", w.Code, w.Body)
			}
			if f.host.saved != 0 || f.content != nil {
				t.Fatal("rejected upload was stored")
			}
			// The key was accepted, so the call counts and shows as failed.
			if !f.store.call.Authenticated || f.store.status != 400 {
				t.Fatalf("call=%+v status=%d", f.store.call, f.store.status)
			}
		})
	}
}

func TestAbandonedKeyUploadIsNotRecordedAsSuccess(t *testing.T) {
	for _, target := range []string{coverTarget, illustrationTarget} {
		f := newUploadFixture(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		w := f.serve(withKey(uploadRequest(t, target, picture(t))).WithContext(ctx))
		if w.Code != 503 || f.store.status != 503 || f.host.saved != 0 || f.content != nil {
			t.Fatalf("%s: status=%d recorded=%d stored=%d", target, w.Code, f.store.status, f.host.saved)
		}
	}
}

func TestKeyUploadsNeedTheImageHostAndThePipeline(t *testing.T) {
	for _, scenario := range []struct {
		name, target, code string
		disable            func(*API)
	}{
		{"cover without image host", coverTarget, "cover_upload_unavailable", func(a *API) { a.Images = nil }},
		{"cover without pipeline", coverTarget, "cover_upload_unavailable", func(a *API) { a.SetATHubKeyCoverUploader(nil) }},
		{"illustration without image host", illustrationTarget, "illustrations_unavailable", func(a *API) { a.Images = nil }},
		{"illustration without pipeline", illustrationTarget, "illustrations_unavailable", func(a *API) { a.SetATHubKeyIllustrationUploader(nil) }},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			f := newUploadFixture(t)
			scenario.disable(f.api)
			w := f.serve(withKey(uploadRequest(t, scenario.target, picture(t))))
			if w.Code != 503 || !strings.Contains(w.Body.String(), scenario.code) {
				t.Fatalf("status=%d body=%s", w.Code, w.Body)
			}
			if f.store.call.ID != uuid.Nil || f.host.saved != 0 {
				t.Fatal("unavailable upload was admitted or stored")
			}
		})
	}
}

// The website keeps calling both endpoints with its login session.
func TestUploadsKeepTheAccountSession(t *testing.T) {
	for _, target := range []string{coverTarget, illustrationTarget} {
		for _, scenario := range []struct {
			name     string
			checkErr error
			want     int
		}{
			{"valid session", nil, 0},
			{"no cookie", nil, 401},
			{"missing client header", nil, 403},
			{"cross site", nil, 403},
			{"revoked session", nil, 401},
			{"forbidden for the level", store.ErrATHubPermissionDenied, 403},
		} {
			t.Run(target+" "+scenario.name, func(t *testing.T) {
				f := newUploadFixture(t)
				f.store.checkErr = scenario.checkErr
				request := withSession(uploadRequest(t, target, picture(t)))
				switch scenario.name {
				case "no cookie":
					request.Header.Del("Cookie")
				case "missing client header":
					request.Header.Del("X-AlbaTextus-Client")
				case "cross site":
					request.Header.Set("Sec-Fetch-Site", "cross-site")
				case "revoked session":
					now := time.Now()
					f.store.session.RevokedAt = &now
				}
				w := f.serve(request)
				account := f.store.user.ID
				if scenario.want != 0 {
					if w.Code != scenario.want || f.host.saved != 0 || f.content != nil {
						t.Fatalf("status=%d body=%s stored=%d", w.Code, w.Body, f.host.saved)
					}
					return
				}
				if target == coverTarget {
					// A cover stored from the website is checked against the cover
					// rule and, as before, leaves no call record.
					if w.Code != 200 || f.host.category != "book/covers" || f.host.entity != account.String() ||
						f.store.coverChecked != account || f.store.call.ID != uuid.Nil {
						t.Fatalf("status=%d body=%s host=%+v call=%+v", w.Code, w.Body, f.host, f.store.call)
					}
					return
				}
				f.recorded(t)
				if w.Code != 201 || f.host.entity != "17" || f.store.illustrationChecked != account ||
					f.store.call.KeyID != nil || f.store.call.SessionID == nil || *f.store.call.SessionID != f.store.session.ID ||
					f.store.call.UserID == nil || *f.store.call.UserID != account {
					t.Fatalf("status=%d body=%s call=%+v", w.Code, w.Body, f.store.call)
				}
			})
		}
	}
}
