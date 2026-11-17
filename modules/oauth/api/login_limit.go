package httpapi

import (
	"context"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/AlbaTextus/atnexus/internal/security"
	"github.com/AlbaTextus/atnexus/internal/store"
)

// loginLimits returns the counters for one password login attempt. Each holds
// failed attempts plus those still being checked.
// The address-and-email pair is the tight limit, so one client guessing cannot
// lock the owner out from elsewhere. The wider per-email and per-address
// limits bound guessing spread across many addresses or many accounts.
func (a *API) loginLimits(r *http.Request, email string) []store.LoginLimit {
	limit := a.Config.LoginFailureLimit
	limits := []store.LoginLimit{{Key: security.HashToken("login:email:" + email), Limit: limit * 3}}
	if ip := loginIP(r, a.Config.TrustedProxyCIDRs); ip != "" {
		limits = append(limits,
			store.LoginLimit{Key: loginPairKey(ip, email), Limit: limit},
			store.LoginLimit{Key: security.HashToken("login:ip:" + ip), Limit: limit * 5})
	}
	return limits
}

func loginPairKey(ip, email string) string {
	return security.HashToken("login:pair:" + ip + "|" + email)
}

// loginLimiter is nil when limiting is switched off or the store cannot count.
func (a *API) loginLimiter() store.LoginLimiter {
	if a.Config.LoginFailureLimit <= 0 {
		return nil
	}
	limiter, _ := a.Store.(store.LoginLimiter)
	return limiter
}

// loginAllowed counts this attempt against every limit before the password is
// checked, so attempts sent together cannot all slip under a limit. It writes
// the 429 response itself when the caller must wait.
func (a *API) loginAllowed(w http.ResponseWriter, r *http.Request, limits []store.LoginLimit) bool {
	limiter := a.loginLimiter()
	if limiter == nil {
		return true
	}
	wait, err := limiter.ReserveLogin(r.Context(), limits, a.Config.LoginFailureWindow)
	if err != nil {
		a.Log.Error("check login rate limit", "error", err)
		writeJSON(w, 500, map[string]string{"error": "server_error"})
		return false
	}
	if wait <= 0 {
		return true
	}
	w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(wait.Seconds()))))
	writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "login_rate_limited", "error_description": "登录失败次数过多，请稍后再试。"})
	return false
}

// loginSucceeded forgets this client's own failures for the account and gives
// the attempt back to the shared per-email and per-address counters, which
// otherwise keep running until they expire.
func (a *API) loginSucceeded(r *http.Request, email string, limits []store.LoginLimit) {
	pair := ""
	if ip := loginIP(r, a.Config.TrustedProxyCIDRs); ip != "" {
		pair = loginPairKey(ip, email)
	}
	var clear, uncount []string
	for _, limit := range limits {
		if limit.Key == pair {
			clear = append(clear, limit.Key)
		} else {
			uncount = append(uncount, limit.Key)
		}
	}
	a.loginReleased(r.Context(), clear, uncount)
}

// loginUncounted gives the attempt back when it ended without a verdict on the
// password, such as a storage outage.
func (a *API) loginUncounted(ctx context.Context, limits []store.LoginLimit) {
	keys := make([]string, len(limits))
	for i, limit := range limits {
		keys[i] = limit.Key
	}
	a.loginReleased(ctx, nil, keys)
}

func (a *API) loginReleased(ctx context.Context, clear, uncount []string) {
	limiter := a.loginLimiter()
	if limiter == nil {
		return
	}
	// The verdict is already decided; a client that hangs up must not leave a
	// successful login counted as a failed guess.
	settle, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := limiter.ReleaseLogin(settle, clear, uncount); err != nil {
		a.Log.Error("release login attempt", "error", err)
	}
}
