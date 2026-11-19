package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/AlbaTextus/atnexus/internal/config"
	"github.com/AlbaTextus/atnexus/internal/security"
	"github.com/AlbaTextus/atnexus/internal/store"
	"github.com/AlbaTextus/atnexus/modules/mail"
	"github.com/google/uuid"
)

// VerificationStore returns the code store, or answers 503 when codes cannot
// be issued or checked. Registration in the account module completes a code
// through it as well.
func VerificationStore(w http.ResponseWriter, st store.Store, smtp config.SMTP) (store.EmailStore, bool) {
	codes, ok := st.(store.EmailStore)
	if !ok || !smtp.Enabled() {
		writeJSON(w, 503, map[string]string{"error": "email_unavailable", "error_description": "邮箱服务暂未配置或不可用，请稍后重试。"})
		return nil, false
	}
	return codes, true
}

func (a *API) sendRegistrationCode(w http.ResponseWriter, r *http.Request) {
	if !a.account.RequireRegistration(w, r) {
		return
	}
	if !a.account.VerifyHuman(w, r) {
		return
	}
	var input struct {
		Email string `json:"email"`
	}
	if !readAccountJSON(w, r, &input) {
		return
	}
	a.sendEmailCode(w, r, store.EmailChallenge{Purpose: "register", Email: mail.NormalizeAddress(input.Email), Actor: "register"})
}

func (a *API) sendAccountEmailCode(w http.ResponseWriter, r *http.Request) {
	user, ok := a.AccountActor(w, r)
	if !ok {
		return
	}
	var input struct {
		Email string `json:"email"`
	}
	if !readAccountJSON(w, r, &input) {
		return
	}
	cookie, _ := r.Cookie(a.Config.SessionCookieName)
	a.sendEmailCode(w, r, store.EmailChallenge{Purpose: "account", Email: mail.NormalizeAddress(input.Email), Actor: user.ID.String() + ":" + security.HashToken(cookie.Value), PreviousEmail: user.Email})
}

func (a *API) sendEmailCode(w http.ResponseWriter, r *http.Request, challenge store.EmailChallenge) {
	w.Header().Set("Cache-Control", "no-store")
	st, ok := VerificationStore(w, a.Store, a.Config.SMTP)
	if !ok {
		return
	}
	if !mail.ValidAddress(challenge.Email) {
		writeJSON(w, 400, map[string]string{"error": "invalid_email", "error_description": "请填写有效的收件邮箱。"})
		return
	}
	code, err := security.NewEmailCode()
	if err != nil {
		WriteError(w, a.Log, err)
		return
	}
	challenge.ID = uuid.New()
	challenge.CodeHash = security.EmailCodeHash(a.Config.ClientSecretEncryptionKey, challenge.ID.String(), code)
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
		WriteError(w, a.Log, err)
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
		WriteError(w, a.Log, err)
		return
	}
	writeJSON(w, 200, map[string]any{"challenge_id": challenge.ID, "expires_in": 300, "retry_after": 60})
}

func (a *API) confirmAccountEmail(w http.ResponseWriter, r *http.Request) {
	user, ok := a.AccountActor(w, r)
	if !ok {
		return
	}
	st, ok := VerificationStore(w, a.Store, a.Config.SMTP)
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
		WriteError(w, a.Log, store.ErrEmailCode)
		return
	}
	cookie, _ := r.Cookie(a.Config.SessionCookieName)
	sessionHash := security.HashToken(cookie.Value)
	updated, err := st.CompleteEmailVerification(r.Context(), store.EmailCompletion{
		EmailChallenge: store.EmailChallenge{ID: id, Purpose: "account", Email: mail.NormalizeAddress(input.Email), Actor: user.ID.String() + ":" + sessionHash, CodeHash: security.EmailCodeHash(a.Config.ClientSecretEncryptionKey, id.String(), input.Code)},
		UserID:         user.ID, SessionHash: sessionHash, PasswordHash: user.PasswordHash,
	})
	if err != nil {
		WriteError(w, a.Log, err)
		return
	}
	writeJSON(w, 200, a.account.UserView(updated))
}

// WriteError answers a failed code operation without exposing its cause.
func WriteError(w http.ResponseWriter, log *slog.Logger, err error) {
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
		log.Error("email operation failed", "error", err)
	}
	writeJSON(w, status, map[string]string{"error": code, "error_description": message})
}
