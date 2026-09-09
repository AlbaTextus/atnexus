package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/AlbaTextus/oauth/internal/security"
	"github.com/AlbaTextus/oauth/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (a *API) walletStore(w http.ResponseWriter) (store.WalletStore, bool) {
	st, ok := a.store.(store.WalletStore)
	if !ok {
		writeJSON(w, 503, map[string]string{"error": "wallet_unavailable"})
	}
	return st, ok
}

func (a *API) walletError(w http.ResponseWriter, err error) {
	status, code, message := 500, "wallet_unavailable", "钱包暂时不可用，请稍后重试。"
	switch {
	case errors.Is(err, store.ErrWalletQuery):
		status, code, message = 400, "invalid_query", "分页、类型或日期范围无效。"
	case errors.Is(err, store.ErrNotFound):
		status, code, message = 404, "wallet_not_found", "用户或订单不存在，或你没有访问权限。"
	case errors.Is(err, store.ErrWalletAmount):
		status, code, message = 400, "invalid_amount", "积分必须为整数，且不能超过余额上限。"
	case errors.Is(err, store.ErrWalletBalance):
		status, code, message = 409, "insufficient_balance", "可用余额不足。"
	case errors.Is(err, store.ErrWalletOrderClosed):
		status, code, message = 409, "order_closed", "订单已过期、取消或完成，请刷新订单。"
	case errors.Is(err, store.ErrWalletForbidden):
		status, code, message = 403, "wallet_forbidden", "账户或应用当前不允许进行这项操作。"
	case errors.Is(err, store.ErrConflict):
		status, code, message = 409, "idempotency_conflict", "该订单号或操作编号已用于不同内容，请核对原操作。"
	default:
		a.log.Error("wallet operation", "error", err)
	}
	writeJSON(w, status, map[string]string{"error": code, "error_description": message})
}

func walletOffset(w http.ResponseWriter, r *http.Request) (int, bool) {
	value := r.URL.Query().Get("offset")
	if value == "" {
		return 0, true
	}
	offset, err := strconv.Atoi(value)
	if err != nil || offset < 0 || offset > 100000 {
		writeJSON(w, 400, map[string]string{"error": "invalid_offset"})
		return 0, false
	}
	return offset, true
}

func paymentID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "wallet_not_found"})
	}
	return id, err == nil
}

func (a *API) getWallet(w http.ResponseWriter, r *http.Request) {
	u, ok := a.accountActor(w, r)
	if !ok {
		return
	}
	st, ok := a.walletStore(w)
	if !ok {
		return
	}
	wallet, err := st.Wallet(r.Context(), u.ID)
	if err != nil {
		a.walletError(w, err)
		return
	}
	writeJSON(w, 200, wallet)
}

func (a *API) listWalletEntries(w http.ResponseWriter, r *http.Request) {
	u, ok := a.accountActor(w, r)
	if !ok {
		return
	}
	st, ok := a.walletStore(w)
	if !ok {
		return
	}
	offset, ok := walletOffset(w, r)
	if !ok {
		return
	}
	entries, err := st.WalletEntries(r.Context(), u.ID, offset)
	if err != nil {
		a.walletError(w, err)
		return
	}
	more := len(entries) > 20
	if more {
		entries = entries[:20]
	}
	writeJSON(w, 200, map[string]any{"entries": entries, "has_more": more})
}

func (a *API) listPaymentOrders(w http.ResponseWriter, r *http.Request) {
	u, ok := a.accountActor(w, r)
	if !ok {
		return
	}
	st, ok := a.walletStore(w)
	if !ok {
		return
	}
	offset, ok := walletOffset(w, r)
	if !ok {
		return
	}
	orders, err := st.PaymentOrders(r.Context(), u.ID, offset)
	if err != nil {
		a.walletError(w, err)
		return
	}
	more := len(orders) > 20
	if more {
		orders = orders[:20]
	}
	writeJSON(w, 200, map[string]any{"orders": orders, "has_more": more})
}

func (a *API) getAccountPaymentOrder(w http.ResponseWriter, r *http.Request) {
	u, ok := a.accountActor(w, r)
	if !ok {
		return
	}
	id, ok := paymentID(w, r)
	if !ok {
		return
	}
	st, ok := a.walletStore(w)
	if !ok {
		return
	}
	order, err := st.PaymentOrderForUser(r.Context(), id, u.ID)
	if err != nil {
		a.walletError(w, err)
		return
	}
	writeJSON(w, 200, order)
}

func (a *API) payOrder(w http.ResponseWriter, r *http.Request) {
	u, ok := a.accountActor(w, r)
	if !ok {
		return
	}
	id, ok := paymentID(w, r)
	if !ok {
		return
	}
	st, ok := a.walletStore(w)
	if !ok {
		return
	}
	order, err := st.PayOrder(r.Context(), id, u.ID)
	if err != nil {
		a.walletError(w, err)
		return
	}
	writeJSON(w, 200, order)
}

func (a *API) cancelPaymentOrder(w http.ResponseWriter, r *http.Request) {
	u, ok := a.accountActor(w, r)
	if !ok {
		return
	}
	id, ok := paymentID(w, r)
	if !ok {
		return
	}
	st, ok := a.walletStore(w)
	if !ok {
		return
	}
	if err := st.CancelPaymentOrder(r.Context(), id, u.ID); err != nil {
		a.walletError(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"success": true})
}

// Application secrets authenticate server-to-server requests only. Browser
// sessions cannot create merchant orders, and application tokens cannot pay.
func (a *API) paymentClient(w http.ResponseWriter, r *http.Request) (store.Client, bool) {
	w.Header().Set("Cache-Control", "no-store")
	id, secret, ok := r.BasicAuth()
	if !ok || len(id) > 100 || len(secret) > 72 {
		writeOAuthError(w, oauthErr("invalid_client", "application credentials required", 401))
		return store.Client{}, false
	}
	client, err := a.store.ClientByID(r.Context(), id)
	if err != nil || !client.IsActive || client.IsPublic() || !security.CheckPassword(*client.SecretHash, secret) {
		writeOAuthError(w, oauthErr("invalid_client", "application credentials are invalid", 401))
		return store.Client{}, false
	}
	return client, true
}

func (a *API) createPaymentOrder(w http.ResponseWriter, r *http.Request) {
	client, ok := a.paymentClient(w, r)
	if !ok {
		return
	}
	var in struct {
		ExternalID string `json:"external_order_id"`
		PayerID    string `json:"payer_id"`
		Title      string `json:"title"`
		Amount     string `json:"amount"`
	}
	if !readAccountJSON(w, r, &in) {
		return
	}
	amount, err := store.ParseAmount(in.Amount)
	if err != nil || amount <= 0 {
		a.walletError(w, store.ErrWalletAmount)
		return
	}
	payer, err := uuid.Parse(in.PayerID)
	if err != nil || payer == uuid.Nil || in.ExternalID == "" || utf8.RuneCountInString(in.ExternalID) > 100 || strings.TrimSpace(in.Title) == "" || utf8.RuneCountInString(in.Title) > 100 {
		writeJSON(w, 400, map[string]string{"error": "invalid_order", "error_description": "请提供付款用户 ID、订单号及 1–100 字的订单名称。"})
		return
	}
	st, ok := a.walletStore(w)
	if !ok {
		return
	}
	order, created, err := st.CreatePaymentOrder(r.Context(), store.NewPaymentOrder{ClientID: client.ID, SecretHash: *client.SecretHash, ExternalID: in.ExternalID, PayerID: payer, Title: strings.TrimSpace(in.Title), Amount: amount})
	if err != nil {
		a.walletError(w, err)
		return
	}
	status := 200
	if created {
		status = 201
	}
	writeJSON(w, status, map[string]any{"order": order, "checkout_url": strings.TrimRight(a.cfg.AuthUIURL, "/") + "/pay/" + order.ID.String()})
}

func (a *API) getClientPaymentOrder(w http.ResponseWriter, r *http.Request) {
	client, ok := a.paymentClient(w, r)
	if !ok {
		return
	}
	id, ok := paymentID(w, r)
	if !ok {
		return
	}
	st, ok := a.walletStore(w)
	if !ok {
		return
	}
	order, err := st.PaymentOrderForClient(r.Context(), id, client.ID)
	if err != nil {
		a.walletError(w, err)
		return
	}
	writeJSON(w, 200, order)
}

func (a *API) adjustWallet(w http.ResponseWriter, r *http.Request) {
	u, ok := a.accountActor(w, r)
	if !ok {
		return
	}
	if u.Role != "SUPER_ADMIN" {
		a.walletError(w, store.ErrWalletForbidden)
		return
	}
	var in struct {
		UserID int    `json:"user_numeric_id"`
		Amount string `json:"amount"`
		Reason string `json:"reason"`
		Key    string `json:"idempotency_key"`
	}
	if !readAccountJSON(w, r, &in) {
		return
	}
	amount, err := store.ParseAmount(in.Amount)
	if err != nil || amount == 0 {
		a.walletError(w, store.ErrWalletAmount)
		return
	}
	if in.UserID <= 0 || strings.TrimSpace(in.Reason) == "" || utf8.RuneCountInString(in.Reason) > 200 || in.Key == "" || len(in.Key) > 100 {
		writeJSON(w, 400, map[string]string{"error": "invalid_adjustment", "error_description": "请填写用户编号、调整原因与操作编号。"})
		return
	}
	st, ok := a.walletStore(w)
	if !ok {
		return
	}
	entry, err := st.AdjustWallet(r.Context(), store.WalletAdjustment{ActorID: u.ID, UserNumericID: in.UserID, Amount: amount, Reason: strings.TrimSpace(in.Reason), Key: in.Key})
	if err != nil {
		a.walletError(w, err)
		return
	}
	writeJSON(w, 200, entry)
}

func (a *API) getAdminWallet(w http.ResponseWriter, r *http.Request) {
	u, ok := a.accountActor(w, r)
	if !ok {
		return
	}
	if u.Role != "SUPER_ADMIN" {
		a.walletError(w, store.ErrWalletForbidden)
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(query) > 100 {
		a.walletError(w, store.ErrWalletQuery)
		return
	}
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil || id <= 0 {
		a.walletError(w, store.ErrNotFound)
		return
	}
	offset, ok := walletOffset(w, r)
	if !ok {
		return
	}
	st, ok := a.walletStore(w)
	if !ok {
		return
	}
	user, err := st.WalletUser(r.Context(), id)
	if err != nil {
		a.walletError(w, err)
		return
	}
	wallet, err := st.Wallet(r.Context(), user.ID)
	if err != nil {
		a.walletError(w, err)
		return
	}
	var entries []store.WalletEntry
	if query == "" {
		entries, err = st.WalletEntries(r.Context(), user.ID, offset)
	} else if search, ok := a.store.(store.WalletEntrySearchStore); ok {
		entries, err = search.SearchWalletEntries(r.Context(), user.ID, query, offset)
	} else {
		writeJSON(w, 503, map[string]string{"error": "wallet_unavailable"})
		return
	}
	if err != nil {
		a.walletError(w, err)
		return
	}
	more := len(entries) > 20
	if more {
		entries = entries[:20]
	}
	writeJSON(w, 200, map[string]any{"numeric_id": user.NumericID, "display_name": user.DisplayName, "is_active": user.IsActive, "wallet": wallet, "entries": entries, "has_more": more})
}
