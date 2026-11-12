package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type ATHubRankedUser struct {
	UserID      uuid.UUID  `json:"user_id"`
	NumericID   int        `json:"numeric_id"`
	DisplayName *string    `json:"display_name"`
	AvatarURL   string     `json:"avatar_url"`
	Total       int64      `json:"total_calls"`
	Success     int64      `json:"successful_calls"`
	Failed      int64      `json:"failed_calls"`
	Pending     int64      `json:"pending_calls"`
	LastUsedAt  *time.Time `json:"last_used_at"`
}

type ATHubAdminCall struct {
	ATHubCall
	NumericID   *int    `json:"numeric_id"`
	DisplayName *string `json:"display_name"`
	AvatarURL   *string `json:"avatar_url"`
}

type ATHubDashboard struct {
	Usage      ATHubUsage        `json:"usage"`
	Users      []ATHubRankedUser `json:"users"`
	TotalUsers int64             `json:"total_users"`
	Calls      []ATHubAdminCall  `json:"calls"`
	CallsNext  bool              `json:"calls_has_next"`
}

type ATHubDashboardReader interface {
	ATHubDashboard(context.Context, time.Time, time.Time, int, int, int, int) (ATHubDashboard, error)
}

// Match the account avatar selection, including custom and provider avatars.
const dashboardAvatarSQL = `CASE WHEN u.avatar_source='custom' THEN u.custom_avatar_url
	ELSE COALESCE((SELECT avatar_url FROM external_identities
	WHERE user_id=u.id AND avatar_url<>'' AND (u.avatar_source='auto' OR provider=u.avatar_source)
	ORDER BY created_at,id LIMIT 1),'') END`

func (p *Postgres) ATHubDashboard(ctx context.Context, from, until time.Time, bucketSeconds, limit, rankOffset, callsOffset int) (ATHubDashboard, error) {
	result := ATHubDashboard{Users: make([]ATHubRankedUser, 0), Calls: make([]ATHubAdminCall, 0)}
	if !until.After(from) || until.Sub(from) > 31*24*time.Hour ||
		(bucketSeconds != 3600 && bucketSeconds != 86400) || limit < 1 || limit > 100 ||
		rankOffset < 0 || rankOffset > 1000000 || callsOffset < 0 || callsOffset > 1000000 {
		return result, errors.New("invalid dashboard range or pagination")
	}
	// Keep totals, ranking and call log in the same database snapshot.
	tx, err := p.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return result, err
	}
	defer tx.Rollback(ctx)
	result.Usage, err = athubUsageSnapshot(ctx, tx, nil, &from, &until, bucketSeconds)
	if err != nil {
		return result, err
	}
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM users u WHERE EXISTS(
		SELECT 1 FROM athub_api_calls c WHERE c.user_id=u.id AND c.key_id IS NOT NULL AND c.started_at >= $1 AND c.started_at < $2
	)`, from, until).Scan(&result.TotalUsers); err != nil {
		return result, err
	}
	// Only rank users with calls in this range. Aggregate by user rather than
	// current key to retain calls made before key rotation.
	rows, err := tx.Query(ctx, `WITH usage AS (
		SELECT user_id,COUNT(*) AS total,
			COUNT(*) FILTER(WHERE status BETWEEN 200 AND 399) AS success,
			COUNT(*) FILTER(WHERE status>=400) AS failed,
			COUNT(*) FILTER(WHERE status IS NULL) AS pending,MAX(started_at) AS last_used_at
		FROM athub_api_calls WHERE key_id IS NOT NULL AND started_at >= $1 AND started_at < $2 AND user_id IS NOT NULL GROUP BY user_id
	)
	SELECT u.id,u.numeric_id,u.display_name,`+dashboardAvatarSQL+`,c.total,c.success,
		c.failed,c.pending,c.last_used_at
	FROM users u JOIN usage c ON c.user_id=u.id
	ORDER BY c.total DESC,u.numeric_id,u.id LIMIT $3 OFFSET $4`, from, until, limit, rankOffset)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var user ATHubRankedUser
		if err := rows.Scan(&user.UserID, &user.NumericID, &user.DisplayName, &user.AvatarURL,
			&user.Total, &user.Success, &user.Failed, &user.Pending, &user.LastUsedAt); err != nil {
			return result, err
		}
		result.Users = append(result.Users, user)
	}
	if err := rows.Err(); err != nil {
		return result, err
	}
	rows.Close()
	calls, err := tx.Query(ctx, `SELECT c.id,c.user_id,c.key_id,c.request_id,c.method,c.path,c.port,c.authenticated,
		c.status,c.duration_ms,c.response_bytes,c.started_at,c.completed_at,u.numeric_id,u.display_name,`+dashboardAvatarSQL+`
		FROM athub_api_calls c LEFT JOIN users u ON u.id=c.user_id
		WHERE c.key_id IS NOT NULL AND c.started_at >= $1 AND c.started_at < $2
		ORDER BY c.started_at DESC,c.id DESC LIMIT $3 OFFSET $4`, from, until, limit+1, callsOffset)
	if err != nil {
		return result, err
	}
	defer calls.Close()
	for calls.Next() {
		var call ATHubAdminCall
		if err := calls.Scan(&call.ID, &call.UserID, &call.KeyID, &call.RequestID, &call.Method,
			&call.Path, &call.Port, &call.Authenticated, &call.Status, &call.DurationMS,
			&call.ResponseBytes, &call.StartedAt, &call.CompletedAt, &call.NumericID,
			&call.DisplayName, &call.AvatarURL); err != nil {
			return result, err
		}
		result.Calls = append(result.Calls, call)
	}
	if err := calls.Err(); err != nil {
		return result, err
	}
	calls.Close()
	result.CallsNext = len(result.Calls) > limit
	if result.CallsNext {
		result.Calls = result.Calls[:limit]
	}
	return result, tx.Commit(ctx)
}
