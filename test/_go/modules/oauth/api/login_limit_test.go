package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AlbaTextus/atnexus/internal/apibase"
	"github.com/AlbaTextus/atnexus/internal/config"
	"github.com/AlbaTextus/atnexus/internal/store"
)

// memoryLimiter counts attempts like the PostgreSQL store, without expiry.
type memoryLimiter struct {
	store.Store
	mu     sync.Mutex
	counts map[string]int
}

func (m *memoryLimiter) ReserveLogin(_ context.Context, limits []store.LoginLimit, _ time.Duration) (time.Duration, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, limit := range limits {
		if m.counts[limit.Key] >= limit.Limit {
			return 90 * time.Second, nil
		}
	}
	for _, limit := range limits {
		m.counts[limit.Key]++
	}
	return 0, nil
}
func (m *memoryLimiter) ReleaseLogin(_ context.Context, clear, uncount []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, key := range clear {
		delete(m.counts, key)
	}
	for _, key := range uncount {
		if m.counts[key] > 0 {
			m.counts[key]--
		}
	}
	return nil
}

func loginLimitAPI(limit int) (*API, *memoryLimiter) {
	limiter := &memoryLimiter{counts: map[string]int{}}
	cfg := config.Config{LoginFailureLimit: limit, LoginFailureWindow: time.Minute}
	return &API{Base: &apibase.Base{Config: cfg, Store: limiter, Log: slog.Default()}}, limiter
}

func loginFrom(address string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/api/v1/sessions", nil)
	r.RemoteAddr = address
	return r
}

func attempt(a *API, r *http.Request, email string, fail bool) int {
	w := httptest.NewRecorder()
	limits := a.loginLimits(r, email)
	if !a.loginAllowed(w, r, limits) {
		return w.Code
	}
	if fail {
		return http.StatusUnauthorized
	}
	a.loginSucceeded(r, email, limits)
	return http.StatusOK
}

func TestLoginFailuresBlockOnlyTheGuessingClient(t *testing.T) {
	a, _ := loginLimitAPI(3)
	attacker, owner := loginFrom("203.0.113.5:4000"), loginFrom("198.51.100.7:4000")
	for i := 0; i < 3; i++ {
		if code := attempt(a, attacker, "a@example.com", true); code != http.StatusUnauthorized {
			t.Fatalf("attempt %d = %d, want 401", i, code)
		}
	}
	w := httptest.NewRecorder()
	if a.loginAllowed(w, attacker, a.loginLimits(attacker, "a@example.com")) {
		t.Fatal("fourth attempt from the same address was allowed")
	}
	if w.Code != http.StatusTooManyRequests || w.Header().Get("Retry-After") != "90" {
		t.Fatalf("blocked reply = %d, Retry-After %q", w.Code, w.Header().Get("Retry-After"))
	}
	if code := attempt(a, owner, "a@example.com", false); code != http.StatusOK {
		t.Fatalf("owner from another address = %d, want 200", code)
	}
	if code := attempt(a, attacker, "b@example.com", true); code != http.StatusUnauthorized {
		t.Fatalf("same address, other account = %d, want 401", code)
	}
}

func TestLoginFailuresAcrossAddressesBlockTheEmail(t *testing.T) {
	a, _ := loginLimitAPI(2)
	// The per-email limit is three times the per-pair limit.
	for i, address := range []string{"203.0.113.1:1", "203.0.113.2:1", "203.0.113.3:1"} {
		for j := 0; j < 2; j++ {
			if code := attempt(a, loginFrom(address), "a@example.com", true); code != http.StatusUnauthorized {
				t.Fatalf("address %d attempt %d = %d, want 401", i, j, code)
			}
		}
	}
	if code := attempt(a, loginFrom("203.0.113.4:1"), "a@example.com", true); code != http.StatusTooManyRequests {
		t.Fatalf("fresh address after distributed guessing = %d, want 429", code)
	}
}

func TestLoginSuccessClearsOwnFailures(t *testing.T) {
	a, limiter := loginLimitAPI(3)
	client := loginFrom("203.0.113.5:4000")
	attempt(a, client, "a@example.com", true)
	attempt(a, client, "a@example.com", true)
	if code := attempt(a, client, "a@example.com", false); code != http.StatusOK {
		t.Fatalf("correct password = %d, want 200", code)
	}
	if count := limiter.counts[loginPairKey("203.0.113.5", "a@example.com")]; count != 0 {
		t.Fatalf("pair counter after success = %d, want 0", count)
	}
	// The shared counters keep the two failures but not the successful attempt.
	for _, limit := range a.loginLimits(client, "a@example.com") {
		if limit.Key != loginPairKey("203.0.113.5", "a@example.com") && limiter.counts[limit.Key] != 2 {
			t.Fatalf("shared counter after success = %d, want 2", limiter.counts[limit.Key])
		}
	}
}

func TestConcurrentLoginAttemptsCannotExceedTheLimit(t *testing.T) {
	a, _ := loginLimitAPI(3)
	var allowed atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			client := loginFrom("203.0.113.5:4000")
			// Every attempt is still being checked when the others arrive.
			if a.loginAllowed(httptest.NewRecorder(), client, a.loginLimits(client, "a@example.com")) {
				allowed.Add(1)
			}
		}()
	}
	wg.Wait()
	if allowed.Load() != 3 {
		t.Fatalf("attempts let through at once = %d, want 3", allowed.Load())
	}
}

func TestLoginAttemptIsGivenBackWithoutAVerdict(t *testing.T) {
	a, limiter := loginLimitAPI(3)
	client := loginFrom("203.0.113.5:4000")
	limits := a.loginLimits(client, "a@example.com")
	if !a.loginAllowed(httptest.NewRecorder(), client, limits) {
		t.Fatal("first attempt was not allowed")
	}
	a.loginUncounted(client.Context(), limits)
	for key, count := range limiter.counts {
		if count != 0 {
			t.Fatalf("counter %s after an undecided attempt = %d, want 0", key, count)
		}
	}
}

func TestLoginLimitCanBeDisabled(t *testing.T) {
	a, limiter := loginLimitAPI(0)
	client := loginFrom("203.0.113.5:4000")
	for i := 0; i < 50; i++ {
		if code := attempt(a, client, "a@example.com", true); code != http.StatusUnauthorized {
			t.Fatalf("attempt %d = %d, want 401", i, code)
		}
	}
	if len(limiter.counts) != 0 {
		t.Fatal("failures were counted while limiting is disabled")
	}
}
