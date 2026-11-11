package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/AlbaTextus/atnexus/internal/store"
	"github.com/google/uuid"
)

type keyLimitStore struct {
	*keyStore
	limits []store.ATHubKeyLimit
	saved  bool
	actor  uuid.UUID
	err    error
}

func (s *keyLimitStore) ATHubKeyLimits(context.Context) ([]store.ATHubKeyLimit, error) {
	return s.limits, s.err
}

func (s *keyLimitStore) SetATHubKeyLimit(_ context.Context, role string, limit *int64, actor uuid.UUID) (store.ATHubKeyLimit, error) {
	s.saved, s.actor = true, actor
	return store.ATHubKeyLimit{Role: role, TotalCallLimit: limit}, s.err
}

func TestATHubKeyLimitAdministration(t *testing.T) {
	for _, role := range []string{"USER", "LEVEL_2", "ADMIN", "SUPER_ADMIN"} {
		t.Run(role, func(t *testing.T) {
			a, keys := keyAPI(t)
			keys.user.Role = role
			s := &keyLimitStore{keyStore: keys, limits: []store.ATHubKeyLimit{{Role: "USER"}}}
			a.store = s
			for _, method := range []string{"GET", "PUT"} {
				w := httptest.NewRecorder()
				a.athubKeyLimits(w, keyRequest(method, `{"role":"USER","total_call_limit":100}`))
				if role == "USER" || role == "LEVEL_2" {
					if w.Code != 403 || s.saved {
						t.Fatal("non-administrator accessed configuration")
					}
				} else if w.Code != 200 || (method == "PUT" && (!s.saved || s.actor != keys.user.ID)) {
					t.Fatalf("method=%s status=%d body=%s", method, w.Code, w.Body)
				}
			}
		})
	}
}

func TestATHubKeyLimitValidation(t *testing.T) {
	for _, body := range []string{
		`{}`, `{"role":"USER"}`, `{"role":"LT2","total_call_limit":10}`,
		`{"role":"USER","total_call_limit":-1}`, `{"role":"USER","total_call_limit":1.5}`,
		`{"role":"USER","total_call_limit":"100"}`, `{"role":"USER","total_call_limit":true}`,
		`{"role":"USER","total_call_limit":9007199254740992}`,
		`{"role":"USER","total_call_limit":1,"extra":true}`,
		`{"role":"USER","total_call_limit":1} {}`,
	} {
		a, keys := keyAPI(t)
		keys.user.Role = "ADMIN"
		s := &keyLimitStore{keyStore: keys}
		a.store = s
		w := httptest.NewRecorder()
		a.athubKeyLimits(w, keyRequest("PUT", body))
		if w.Code != 400 || s.saved {
			t.Fatalf("invalid body accepted: %s status=%d", body, w.Code)
		}
	}
	for _, value := range []string{"null", "0", "1", "9007199254740991"} {
		a, keys := keyAPI(t)
		keys.user.Role = "ADMIN"
		a.store = &keyLimitStore{keyStore: keys}
		w := httptest.NewRecorder()
		a.athubKeyLimits(w, keyRequest("PUT", `{"role":"LEVEL_2","total_call_limit":`+value+`}`))
		var result store.ATHubKeyLimit
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil {
			t.Fatalf("valid value rejected: %s body=%s", value, w.Body)
		}
		if (value == "null") != (result.TotalCallLimit == nil) {
			t.Fatal("unlimited and zero must remain distinct")
		}
	}
}

func TestATHubKeyLimitAccessAndFailure(t *testing.T) {
	for _, scenario := range []string{"no session", "missing origin", "cross site", "offline"} {
		t.Run(scenario, func(t *testing.T) {
			a, keys := keyAPI(t)
			keys.user.Role = "ADMIN"
			s := &keyLimitStore{keyStore: keys}
			a.store = s
			r := keyRequest("PUT", `{"role":"USER","total_call_limit":null}`)
			want := 403
			switch scenario {
			case "no session":
				r.Header.Del("Cookie")
				want = 401
			case "missing origin":
				r.Header.Del("X-AlbaTextus-Client")
			case "cross site":
				r.Header.Set("Sec-Fetch-Site", "cross-site")
			case "offline":
				s.err = errors.New("database offline")
				want = 500
			}
			w := httptest.NewRecorder()
			a.athubKeyLimits(w, r)
			if w.Code != want {
				t.Fatalf("status=%d want=%d", w.Code, want)
			}
			if scenario != "offline" && s.saved {
				t.Fatal("unauthorized update reached store")
			}
		})
	}
}
