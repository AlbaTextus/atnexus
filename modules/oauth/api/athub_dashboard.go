package httpapi

import (
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/AlbaTextus/atnexus/internal/store"
)

func (a *API) athubDashboard(w http.ResponseWriter, r *http.Request) {
	user, ok := a.AccountActor(w, r)
	if !ok {
		return
	}
	if !isAdmin(user) {
		writeJSON(w, 403, map[string]string{"error": "admin_required", "error_description": "全站调用统计仅限管理员使用。"})
		return
	}
	days, rankOffset, callsOffset := 7, 0, 0
	query, err := url.ParseQuery(r.URL.RawQuery)
	valid := err == nil
	for key, values := range query {
		if len(values) != 1 {
			valid = false
			break
		}
		value, err := strconv.Atoi(values[0])
		if err != nil {
			valid = false
			break
		}
		switch key {
		case "days":
			days = value
		case "rank_offset":
			rankOffset = value
		case "calls_offset":
			callsOffset = value
		default:
			valid = false
		}
	}
	if !valid || (days != 1 && days != 7 && days != 30) || rankOffset < 0 || rankOffset > 1000000 || callsOffset < 0 || callsOffset > 1000000 {
		writeJSON(w, 400, map[string]string{"error": "invalid_dashboard_range"})
		return
	}
	reader, ok := a.Store.(store.ATHubDashboardReader)
	if !ok {
		writeJSON(w, 503, map[string]string{"error": "service_unavailable"})
		return
	}
	bucket := time.Hour
	if days != 1 {
		bucket = 24 * time.Hour
	}
	until := time.Now().UTC().Truncate(bucket).Add(bucket)
	from := until.Add(-time.Duration(days) * 24 * time.Hour)
	result, err := reader.ATHubDashboard(r.Context(), from, until, int(bucket.Seconds()), 10, rankOffset, callsOffset)
	if err != nil {
		a.ApplicationError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, result)
}
