package httpapi

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/AlbaTextus/connect/internal/store"
)

func (a *API) walletView(w http.ResponseWriter, r *http.Request) {
	user, ok := a.accountActor(w, r)
	if !ok {
		return
	}
	w.Header().Set("X-Data-Source", "wallet")
	st, ok := a.store.(store.WalletViewStore)
	if !ok {
		writeJSON(w, 503, map[string]string{"error": "wallet_unavailable"})
		return
	}
	params := r.URL.Query()
	if len(params["resource"]) != 1 {
		writeJSON(w, 400, map[string]string{"error": "invalid_resource"})
		return
	}
	switch params.Get("resource") {
	case "user":
		if len(params) != 1 {
			writeJSON(w, 400, map[string]string{"error": "invalid_query"})
			return
		}
		wallet, err := st.Wallet(r.Context(), user.ID)
		if err != nil {
			a.walletError(w, err)
			return
		}
		name := "用户 " + strconv.Itoa(user.NumericID)
		if user.DisplayName != nil && *user.DisplayName != "" {
			name = *user.DisplayName
		}
		writeJSON(w, 200, map[string]any{
			"user": map[string]any{"id": user.ID, "username": name, "nickname": name, "avatar_url": user.AvatarURL,
				"trust_level": store.RoleLevel(user.Role), "is_admin": isAdmin(user), "available_balance": wallet.Balance, "pending_balance": wallet.PendingBalance},
			"trust_levels":  map[string]string{"1": "用户", "3": "管理员", "4": "超级管理员"},
			"public_config": map[string]int{"settlement_delay_days_min": 1, "settlement_delay_days_max": 1},
			"notification":  map[string]bool{"show_bell": false},
			"announcement":  map[string]string{"title": "", "link_label": "", "link_url": ""},
		})
	case "daily-stats":
		if len(params) != 1 {
			writeJSON(w, 400, map[string]string{"error": "invalid_query"})
			return
		}
		stats, err := st.WalletDailyStats(r.Context(), user.ID, time.Now())
		if err != nil {
			a.walletError(w, err)
			return
		}
		writeJSON(w, 200, stats)
	case "transactions":
		query, ok := parseWalletActivityQuery(params, time.Now())
		if !ok {
			writeJSON(w, 400, map[string]string{"error": "invalid_query", "error_description": "分页、类型或日期范围无效。"})
			return
		}
		result, err := st.WalletActivity(r.Context(), user.ID, query)
		if err != nil {
			a.walletError(w, err)
			return
		}
		writeJSON(w, 200, result)
	default:
		writeJSON(w, 400, map[string]string{"error": "invalid_resource"})
	}
}

func parseWalletActivityQuery(params url.Values, now time.Time) (store.WalletActivityQuery, bool) {
	q := store.WalletActivityQuery{Page: 1, PageSize: 20, Start: now.AddDate(0, 0, -30), End: now.Add(time.Nanosecond)}
	allowed := map[string]bool{"resource": true, "page": true, "page_size": true, "types": true, "statuses": true, "startTime": true, "endTime": true}
	for key, values := range params {
		if !allowed[key] || len(values) != 1 || len(values[0]) > 512 {
			return q, false
		}
	}
	for key, target := range map[string]*int{"page": &q.Page, "page_size": &q.PageSize} {
		if raw, exists := params[key]; exists {
			if raw[0] == "" {
				return q, false
			}
			for _, c := range raw[0] {
				if c < '0' || c > '9' {
					return q, false
				}
			}
			n, err := strconv.Atoi(raw[0])
			if err != nil {
				return q, false
			}
			*target = n
		}
	}
	if q.Page < 1 || q.Page > 100001 || q.PageSize < 1 || q.PageSize > 100 || int64(q.Page-1)*int64(q.PageSize) > 100000 {
		return q, false
	}
	for _, filter := range []struct {
		key     string
		allowed string
		target  *[]string
	}{
		{"types", ",receive,payment,community,online,test,distribute,red_envelope_send,red_envelope_receive,red_envelope_refund,", &q.Types},
		{"statuses", ",success,pending,failed,expired,disputing,refund,refused,", &q.Statuses},
	} {
		if raw, exists := params[filter.key]; exists {
			values := strings.Split(raw[0], ",")
			if len(values) > 16 {
				return q, false
			}
			for _, value := range values {
				if value == "" || !strings.Contains(filter.allowed, ","+value+",") {
					return q, false
				}
			}
			*filter.target = values
		}
	}
	for key, target := range map[string]*time.Time{"startTime": &q.Start, "endTime": &q.End} {
		if raw, exists := params[key]; exists {
			value, err := time.Parse(time.RFC3339Nano, raw[0])
			if err != nil {
				return q, false
			}
			if key == "endTime" {
				if value.Nanosecond() == 0 {
					value = value.Add(time.Second)
				} else {
					value = value.Add(time.Nanosecond)
				}
			}
			*target = value
		}
	}
	return q, q.End.After(q.Start)
}
