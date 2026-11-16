package store

import (
	"context"
	"errors"
	"time"

	oauthrepo "github.com/AlbaTextus/atnexus/modules/oauth/repository"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var ErrATHubKeyLimit = oauthrepo.ErrATHubKeyLimit
var ErrATHubAdminDisabled = oauthrepo.ErrATHubAdminDisabled
var ErrATHubAccessDenied = errors.New("ATHub access changed before dispatch")

type ATHubKey = oauthrepo.ATHubKey
type ATHubIdentity = oauthrepo.ATHubIdentity

type ATHubCall struct {
	SessionID     *uuid.UUID `json:"-"`
	ID            uuid.UUID  `json:"id"`
	UserID        *uuid.UUID `json:"user_id"`
	KeyID         *uuid.UUID `json:"key_id"`
	RequestID     uuid.UUID  `json:"request_id"`
	Method        string     `json:"method"`
	Path          string     `json:"path"`
	Port          int        `json:"port"`
	Authenticated bool       `json:"authenticated"`
	Status        *int       `json:"status"`
	DurationMS    *int64     `json:"duration_ms"`
	ResponseBytes *int64     `json:"response_bytes"`
	StartedAt     time.Time  `json:"started_at"`
	CompletedAt   *time.Time `json:"completed_at"`
}

type ATHubUsageGroup struct {
	Method string `json:"method"`
	Path   string `json:"path"`
	Port   int    `json:"port"`
	Status *int   `json:"status"`
	Calls  int64  `json:"calls"`
}

type ATHubUsage struct {
	Total         int64             `json:"total_calls"`
	Success       int64             `json:"successful_calls"`
	Failed        int64             `json:"failed_calls"`
	Pending       int64             `json:"pending_calls"`
	Endpoints     []ATHubUsageGroup `json:"endpoints"`
	From          *time.Time        `json:"from,omitempty"`
	Until         *time.Time        `json:"until,omitempty"`
	BucketSeconds int               `json:"bucket_seconds,omitempty"`
	Trend         []ATHubUsagePoint `json:"trend,omitempty"`
	Quota         *ATHubCallQuota   `json:"quota,omitempty"`
}

type ATHubUsagePoint struct {
	StartedAt time.Time `json:"started_at"`
	Port      int       `json:"port"`
	Calls     int64     `json:"calls"`
}

// ATHubUsageReader supports bounded, server-aggregated dashboard queries.
type ATHubUsageReader interface {
	ATHubUsageRange(context.Context, uuid.UUID, time.Time, time.Time, int) (ATHubUsage, error)
}

// ATHubStore lives entirely in the account database. The content module knows no users.
type ATHubStore interface {
	CreateATHubKey(context.Context, ATHubKey) (ATHubKey, error)
	EnsureATHubKey(context.Context, ATHubKey) (ATHubKey, bool, error)
	ResetATHubKey(context.Context, ATHubKey) (ATHubKey, error)
	ListATHubKeys(context.Context, uuid.UUID) ([]ATHubKey, error)
	RevokeATHubKey(context.Context, uuid.UUID, uuid.UUID) error
	ResolveATHubKey(context.Context, string) (ATHubIdentity, error)
	BeginATHubCall(context.Context, ATHubCall) error
	FinishATHubCall(context.Context, uuid.UUID, int, int64, int64) error
	ListATHubCalls(context.Context, uuid.UUID, int, int) ([]ATHubCall, error)
	ATHubUsage(context.Context, uuid.UUID) (ATHubUsage, error)
}

func (p *Postgres) CheckATHubSchema(ctx context.Context) error {
	for _, query := range []string{
		`SELECT athub_disabled FROM users LIMIT 0`,
		`SELECT role,total_call_limit,updated_by,updated_at FROM athub_key_limits LIMIT 0`,
		`SELECT role,review_required,updated_at FROM athub_role_policies LIMIT 0`,
		`SELECT role,operation,method,path,allowed,total_call_limit FROM athub_access_rules LIMIT 0`,
		`SELECT id,user_id,payload,status,reviewer_id FROM athub_reviews LIMIT 0`,
		`SELECT operation FROM athub_api_calls LIMIT 0`,
		`SELECT id,user_id,name,token_hash,prefix,created_at,expires_at,revoked_at,last_used_at,is_current FROM athub_api_keys LIMIT 0`,
		`SELECT id,user_id,key_id,request_id,method,path,port,authenticated,status,duration_ms,response_bytes,started_at,completed_at FROM athub_api_calls LIMIT 0`,
	} {
		rows, err := p.pool.Query(ctx, query)
		if err != nil {
			return errors.New("ATHub authentication schema is not ready; run pnpm db:deploy in the host repository")
		}
		rows.Close()
	}
	return nil
}

// These methods adapt the unchanged shared account pool to OAuth-owned key storage.
func (p *Postgres) CreateATHubKey(ctx context.Context, key ATHubKey) (ATHubKey, error) {
	return oauthrepo.NewAPIKeys(p.pool, ErrNotFound, ErrUserForbidden).CreateATHubKey(ctx, key)
}
func (p *Postgres) EnsureATHubKey(ctx context.Context, key ATHubKey) (ATHubKey, bool, error) {
	return oauthrepo.NewAPIKeys(p.pool, ErrNotFound, ErrUserForbidden).EnsureATHubKey(ctx, key)
}
func (p *Postgres) ResetATHubKey(ctx context.Context, key ATHubKey) (ATHubKey, error) {
	return oauthrepo.NewAPIKeys(p.pool, ErrNotFound, ErrUserForbidden).ResetATHubKey(ctx, key)
}
func (p *Postgres) ListATHubKeys(ctx context.Context, user uuid.UUID) ([]ATHubKey, error) {
	return oauthrepo.NewAPIKeys(p.pool, ErrNotFound, ErrUserForbidden).ListATHubKeys(ctx, user)
}
func (p *Postgres) RevokeATHubKey(ctx context.Context, user, id uuid.UUID) error {
	return oauthrepo.NewAPIKeys(p.pool, ErrNotFound, ErrUserForbidden).RevokeATHubKey(ctx, user, id)
}
func (p *Postgres) ResolveATHubKey(ctx context.Context, hash string) (ATHubIdentity, error) {
	return oauthrepo.NewAPIKeys(p.pool, ErrNotFound, ErrUserForbidden).ResolveATHubKey(ctx, hash)
}

func (p *Postgres) BeginATHubCall(ctx context.Context, call ATHubCall) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	denied, limited := false, false
	var permissionErr error
	if call.Authenticated {
		var userAllowed, keyAllowed bool
		var role string
		if call.UserID == nil || (call.KeyID == nil && call.SessionID == nil) {
			return ErrNotFound
		}
		// Use the same lock order as administrator changes and rotations. Recheck
		// before dispatch so an earlier lookup cannot outlive a committed ban.
		if err := tx.QueryRow(ctx, `SELECT is_active AND NOT athub_disabled,role FROM users WHERE id=$1 FOR UPDATE`, call.UserID).Scan(&userAllowed, &role); err != nil {
			return err
		}
		if call.SessionID != nil {
			err := tx.QueryRow(ctx, `SELECT revoked_at IS NULL AND expires_at>now() FROM user_sessions
				WHERE id=$1 AND user_id=$2 FOR SHARE`, call.SessionID, call.UserID).Scan(&keyAllowed)
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
		} else if err := tx.QueryRow(ctx, `SELECT is_current AND revoked_at IS NULL AND (expires_at IS NULL OR expires_at>now())
			FROM athub_api_keys WHERE id=$1 AND user_id=$2 FOR UPDATE`, call.KeyID, call.UserID).Scan(&keyAllowed); err != nil {
			return err
		}
		denied = !userAllowed || !keyAllowed
		if denied {
			call.Authenticated = false
		} else if call.KeyID != nil {
			var limit *int64
			// The key row above serializes admission. Read the count in a separate
			// statement after acquiring it so concurrent calls see committed usage.
			// Lock settings too: a committed administrator change applies to the
			// next admitted call. Session-based library reads are not key calls.
			if err := tx.QueryRow(ctx, `SELECT total_call_limit FROM athub_key_limits WHERE role=$1 FOR SHARE`, role).Scan(&limit); err != nil {
				return err
			}
			if limit != nil {
				var used int64
				if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM athub_api_calls WHERE key_id=$1 AND authenticated`, call.KeyID).Scan(&used); err != nil {
					return err
				}
				limited = used >= *limit
				if limited {
					call.Authenticated = false
				}
			}
		}
		if call.Authenticated {
			permissionErr = checkATHubRule(ctx, tx, call, role)
			if permissionErr != nil && !errors.Is(permissionErr, ErrATHubPermissionDenied) {
				return permissionErr
			}
			if permissionErr != nil {
				call.Authenticated = false
			}
		}
	}
	_, err = tx.Exec(ctx, `INSERT INTO athub_api_calls(id,user_id,key_id,request_id,method,path,port,authenticated,operation)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, call.ID, call.UserID, call.KeyID, call.RequestID, call.Method, call.Path, call.Port, call.Authenticated, ATHubOperation(call.Method, call.Path))
	if err != nil {
		return err
	}
	if call.Authenticated && call.KeyID != nil {
		if _, err := tx.Exec(ctx, `UPDATE athub_api_keys SET last_used_at=now() WHERE id=$1`, call.KeyID); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	if denied {
		return ErrATHubAccessDenied
	}
	if limited {
		return ErrATHubCallLimit
	}
	if permissionErr != nil {
		return permissionErr
	}
	return nil
}

func (p *Postgres) FinishATHubCall(ctx context.Context, id uuid.UUID, status int, duration, bytes int64) error {
	tag, err := p.pool.Exec(ctx, `UPDATE athub_api_calls SET status=$2,duration_ms=$3,response_bytes=$4,completed_at=now() WHERE id=$1 AND completed_at IS NULL`, id, status, duration, bytes)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

func (p *Postgres) ListATHubCalls(ctx context.Context, userID uuid.UUID, limit, offset int) ([]ATHubCall, error) {
	rows, err := p.pool.Query(ctx, `SELECT id,user_id,key_id,request_id,method,path,port,authenticated,status,duration_ms,response_bytes,started_at,completed_at
		FROM athub_api_calls WHERE user_id=$1 AND key_id IS NOT NULL ORDER BY started_at DESC,id DESC LIMIT $2 OFFSET $3`, userID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	calls := make([]ATHubCall, 0)
	for rows.Next() {
		var c ATHubCall
		if err := rows.Scan(&c.ID, &c.UserID, &c.KeyID, &c.RequestID, &c.Method, &c.Path, &c.Port, &c.Authenticated, &c.Status, &c.DurationMS, &c.ResponseBytes, &c.StartedAt, &c.CompletedAt); err != nil {
			return nil, err
		}
		calls = append(calls, c)
	}
	return calls, rows.Err()
}

func (p *Postgres) ATHubUsage(ctx context.Context, userID uuid.UUID) (ATHubUsage, error) {
	return p.athubUsage(ctx, userID, nil, nil, 0)
}

func (p *Postgres) ATHubUsageRange(ctx context.Context, userID uuid.UUID, from, until time.Time, bucketSeconds int) (ATHubUsage, error) {
	if !until.After(from) || until.Sub(from) > 31*24*time.Hour || (bucketSeconds != 3600 && bucketSeconds != 86400) {
		return ATHubUsage{}, errors.New("invalid usage range")
	}
	return p.athubUsage(ctx, userID, &from, &until, bucketSeconds)
}

func (p *Postgres) athubUsage(ctx context.Context, userID uuid.UUID, from, until *time.Time, bucketSeconds int) (ATHubUsage, error) {
	// One snapshot for totals and groups, including calls still pending completion.
	tx, err := p.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return ATHubUsage{}, err
	}
	defer tx.Rollback(ctx)
	result, err := athubUsageSnapshot(ctx, tx, &userID, from, until, bucketSeconds)
	if err != nil {
		return result, err
	}
	// Quota follows the current key's lifetime usage, independent of the chart range.
	var quota ATHubCallQuota
	var used int64
	if err := tx.QueryRow(ctx, `SELECT l.total_call_limit,
		(SELECT COUNT(*) FROM athub_api_calls c WHERE c.key_id=k.id AND c.authenticated)
		FROM users u JOIN athub_key_limits l ON l.role=u.role
		LEFT JOIN athub_api_keys k ON k.user_id=u.id AND k.is_current
		WHERE u.id=$1`, userID).Scan(&quota.TotalCallLimit, &used); err != nil {
		return result, err
	}
	if quota.TotalCallLimit != nil {
		remaining := max(int64(0), *quota.TotalCallLimit-used)
		quota.RemainingCalls = &remaining
	}
	result.Quota = &quota
	return result, tx.Commit(ctx)
}

// Only personal API key calls contribute to usage. Account-session traffic
// remains in the audit log but must not appear in API key totals or charts.
// A nil userID aggregates API key calls across the whole site.
func athubUsageSnapshot(ctx context.Context, tx pgx.Tx, userID *uuid.UUID, from, until *time.Time, bucketSeconds int) (ATHubUsage, error) {
	result := ATHubUsage{Endpoints: make([]ATHubUsageGroup, 0), From: from, Until: until, BucketSeconds: bucketSeconds}
	if err := tx.QueryRow(ctx, `SELECT COUNT(*),COUNT(*) FILTER(WHERE status BETWEEN 200 AND 399),
		COUNT(*) FILTER(WHERE status>=400),COUNT(*) FILTER(WHERE status IS NULL) FROM athub_api_calls
		WHERE key_id IS NOT NULL AND ($1::uuid IS NULL OR user_id=$1) AND ($2::timestamptz IS NULL OR started_at >= $2) AND ($3::timestamptz IS NULL OR started_at < $3)`, userID, from, until).Scan(&result.Total, &result.Success, &result.Failed, &result.Pending); err != nil {
		return result, err
	}
	rows, err := tx.Query(ctx, `SELECT method,path,port,status,COUNT(*) FROM athub_api_calls
		WHERE key_id IS NOT NULL AND ($1::uuid IS NULL OR user_id=$1) AND ($2::timestamptz IS NULL OR started_at >= $2) AND ($3::timestamptz IS NULL OR started_at < $3)
		GROUP BY method,path,port,status ORDER BY COUNT(*) DESC,method,path,port,status`, userID, from, until)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var g ATHubUsageGroup
		if err := rows.Scan(&g.Method, &g.Path, &g.Port, &g.Status, &g.Calls); err != nil {
			return result, err
		}
		result.Endpoints = append(result.Endpoints, g)
	}
	if err := rows.Err(); err != nil {
		return result, err
	}
	rows.Close()
	if from != nil {
		result.Trend = make([]ATHubUsagePoint, 0)
		points, err := tx.Query(ctx, `SELECT to_timestamp(floor(extract(epoch FROM started_at)/$4::int)*$4::int),port,COUNT(*)
			FROM athub_api_calls WHERE key_id IS NOT NULL AND ($1::uuid IS NULL OR user_id=$1) AND started_at >= $2 AND started_at < $3
			GROUP BY 1,port ORDER BY 1,port`, userID, from, until, bucketSeconds)
		if err != nil {
			return result, err
		}
		defer points.Close()
		for points.Next() {
			var point ATHubUsagePoint
			if err := points.Scan(&point.StartedAt, &point.Port, &point.Calls); err != nil {
				return result, err
			}
			result.Trend = append(result.Trend, point)
		}
		if err := points.Err(); err != nil {
			return result, err
		}
		points.Close()
	}
	return result, nil
}
