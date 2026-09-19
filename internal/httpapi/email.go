package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/AlbaTextus/oauth/internal/security"
	"github.com/AlbaTextus/oauth/internal/store"
	"github.com/google/uuid"
)

func validEmail(email string) bool {
	address, err := mail.ParseAddress(email)
	return err == nil && address.Address == email && len(email) <= 320 && !strings.ContainsAny(email, "\r\n") && !strings.HasSuffix(email, ".invalid")
}

func (a *API) emailStore(w http.ResponseWriter) (store.EmailStore, bool) {
	st, ok := a.store.(store.EmailStore)
	if !ok || !a.cfg.SMTP.Enabled() || a.mailer == nil {
		writeJSON(w, 503, map[string]string{"error": "email_unavailable", "error_description": "邮箱服务暂未配置或不可用，请稍后重试。"})
		return nil, false
	}
	return st, true
}

func (a *API) sendRegistrationCode(w http.ResponseWriter, r *http.Request) {
	if !a.requireRegistration(w, r) {
		return
	}
	if !a.verifyHuman(w, r) {
		return
	}
	var input struct {
		Email string `json:"email"`
	}
	if !readAccountJSON(w, r, &input) {
		return
	}
	a.sendEmailCode(w, r, store.EmailChallenge{Purpose: "register", Email: normalizeEmail(input.Email), Actor: "register"})
}

func (a *API) sendAccountEmailCode(w http.ResponseWriter, r *http.Request) {
	user, ok := a.accountActor(w, r)
	if !ok {
		return
	}
	var input struct {
		Email string `json:"email"`
	}
	if !readAccountJSON(w, r, &input) {
		return
	}
	cookie, _ := r.Cookie(a.cfg.SessionCookieName)
	a.sendEmailCode(w, r, store.EmailChallenge{Purpose: "account", Email: normalizeEmail(input.Email), Actor: user.ID.String() + ":" + security.HashToken(cookie.Value), PreviousEmail: user.Email})
}

func (a *API) sendEmailCode(w http.ResponseWriter, r *http.Request, challenge store.EmailChallenge) {
	w.Header().Set("Cache-Control", "no-store")
	st, ok := a.emailStore(w)
	if !ok {
		return
	}
	if !validEmail(challenge.Email) {
		writeJSON(w, 400, map[string]string{"error": "invalid_email", "error_description": "请填写有效的收件邮箱。"})
		return
	}
	code, err := security.NewEmailCode()
	if err != nil {
		a.emailError(w, err)
		return
	}
	challenge.ID = uuid.New()
	challenge.CodeHash = security.EmailCodeHash(a.cfg.ClientSecretEncryptionKey, challenge.ID.String(), code)
	limits := []store.EmailRateLimit{
		{Key: security.HashToken("email:minute:" + challenge.Email), Limit: 1, Window: time.Minute},
		{Key: security.HashToken("email:hour:" + challenge.Email), Limit: 10, Window: time.Hour},
		{Key: security.HashToken("email:global:hour"), Limit: 300, Window: time.Hour},
	}
	if challenge.Purpose == "account" {
		// Bind rate limits to the user, not the session, so relogin cannot reset them.
		actor := strings.SplitN(challenge.Actor, ":", 2)[0]
		limits = append(limits, store.EmailRateLimit{Key: security.HashToken("email:actor:minute:" + actor), Limit: 1, Window: time.Minute}, store.EmailRateLimit{Key: security.HashToken("email:actor:hour:" + actor), Limit: 10, Window: time.Hour})
	}
	if err := st.ReserveEmailCode(r.Context(), challenge, limits); err != nil {
		a.emailError(w, err)
		return
	}
	err = a.mailer.SendCode(r.Context(), challenge.Email, code, challenge.Purpose)
	if err == nil {
		err = st.ReadyEmailCode(r.Context(), challenge.ID)
	}
	if err != nil {
		// The HTTP context may already be cancelled. Failed delivery must never
		// leave a usable challenge, even when the caller disconnects.
		ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 2*time.Second)
		defer cancel()
		_ = st.DeleteEmailCode(ctx, challenge.ID)
		a.emailError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"challenge_id": challenge.ID, "expires_in": 300, "retry_after": 60})
}

func (a *API) confirmAccountEmail(w http.ResponseWriter, r *http.Request) {
	user, ok := a.accountActor(w, r)
	if !ok {
		return
	}
	st, ok := a.emailStore(w)
	if !ok {
		return
	}
	var input struct {
		Email           string `json:"email"`
		Code            string `json:"code"`
		ChallengeID     string `json:"challenge_id"`
		CurrentPassword string `json:"current_password"`
	}
	if !readAccountJSON(w, r, &input) {
		return
	}
	if user.PasswordHash != "" && !security.CheckPassword(user.PasswordHash, input.CurrentPassword) {
		writeJSON(w, 400, map[string]string{"error": "incorrect_password", "error_description": "当前密码不正确。"})
		return
	}
	id, err := uuid.Parse(input.ChallengeID)
	if err != nil {
		a.emailError(w, store.ErrEmailCode)
		return
	}
	cookie, _ := r.Cookie(a.cfg.SessionCookieName)
	sessionHash := security.HashToken(cookie.Value)
	updated, err := st.CompleteEmailVerification(r.Context(), store.EmailCompletion{
		EmailChallenge: store.EmailChallenge{ID: id, Purpose: "account", Email: normalizeEmail(input.Email), Actor: user.ID.String() + ":" + sessionHash, CodeHash: security.EmailCodeHash(a.cfg.ClientSecretEncryptionKey, id.String(), input.Code)},
		UserID:         user.ID, SessionHash: sessionHash, PasswordHash: user.PasswordHash,
	})
	if err != nil {
		a.emailError(w, err)
		return
	}
	writeJSON(w, 200, userView(updated))
}

func (a *API) emailError(w http.ResponseWriter, err error) {
	status, code, message := 503, "email_unavailable", "邮件发送或验证暂时失败，请稍后重试。"
	switch {
	case errors.Is(err, store.ErrEmailCode):
		status, code, message = 400, "invalid_email_code", "验证码错误、已过期或尝试次数过多，请重新获取。"
	case errors.Is(err, store.ErrEmailRateLimit):
		status, code, message = 429, "email_rate_limited", "发送过于频繁，请至少等待 60 秒；达到每小时上限时需稍后再试。"
		w.Header().Set("Retry-After", "60")
	case errors.Is(err, store.ErrAlreadyExists):
		status, code, message = 409, "email_exists", "这个邮箱已被使用，请登录已有账户或更换邮箱。"
	case errors.Is(err, store.ErrConflict):
		status, code, message = 409, "account_changed", "账户信息已变更，请刷新页面或重新登录后重试。"
	case errors.Is(err, store.ErrRegistrationDisabled):
		status, code, message = http.StatusForbidden, "registration_disabled", "全站注册已关闭，已有账户仍可正常登录。"
	default:
		a.log.Error("email operation failed")
	}
	writeJSON(w, status, map[string]string{"error": code, "error_description": message})
}
