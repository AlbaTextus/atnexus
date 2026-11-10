package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var ErrATHubKeyLimit = errors.New("one current ATHub key per user")
var ErrATHubAdminDisabled = errors.New("ATHub access disabled by administrator")
var ErrATHubAccessDenied = errors.New("ATHub access changed before dispatch")

type ATHubKey struct {
	AdminDisabled bool       `json:"admin_disabled"`
	ID            uuid.UUID  `json:"id"`
	UserID        uuid.UUID  `json:"user_id"`
	Name          string     `json:"name"`
	TokenHash     string     `json:"-"`
	Prefix        string     `json:"prefix"`
	CreatedAt     time.Time  `json:"created_at"`
	ExpiresAt     *time.Time `json:"expires_at"`
	RevokedAt     *time.Time `json:"revoked_at"`
	LastUsedAt    *time.Time `json:"last_used_at"`
	TotalCalls    int64      `json:"total_calls"`
}

type ATHubIdentity struct {
	KeyID  uuid.UUID
	UserID uuid.UUID
	Active bool
}

type ATHubCall struct {
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

func (p *Postgres) CreateATHubKey(ctx context.Context, key ATHubKey) (ATHubKey, error) {
	result, created, err := p.manageATHubKey(ctx, key, false)
	if err == nil && !created {
		return result, ErrATHubKeyLimit
	}
	return result, err
}

func (p *Postgres) EnsureATHubKey(ctx context.Context, key ATHubKey) (ATHubKey, bool, error) {
	return p.manageATHubKey(ctx, key, false)
}

func (p *Postgres) ResetATHubKey(ctx context.Context, key ATHubKey) (ATHubKey, error) {
	result, _, err := p.manageATHubKey(ctx, key, true)
	return result, err
}

func (p *Postgres) manageATHubKey(ctx context.Context, key ATHubKey, reset bool) (ATHubKey, bool, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return key, false, err
	}
	defer tx.Rollback(ctx)
	var active, adminDisabled bool
	// Serialize provisioning and rotation for this user's single current slot.
	if err := tx.QueryRow(ctx, `SELECT is_active,athub_disabled FROM users WHERE id=$1 FOR UPDATE`, key.UserID).Scan(&active, &adminDisabled); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return key, false, ErrNotFound
		}
		return key, false, err
	}
	if !active {
		return key, false, ErrNotFound
	}
	if reset && adminDisabled {
		return key, false, ErrATHubAdminDisabled
	}
	if !reset {
		var current ATHubKey
		err := tx.QueryRow(ctx, `SELECT k.id,k.user_id,k.name,k.prefix,k.created_at,k.expires_at,k.revoked_at,k.last_used_at,
			(SELECT COUNT(*) FROM athub_api_calls c WHERE c.key_id=k.id)
			FROM athub_api_keys k WHERE k.user_id=$1 AND k.is_current`, key.UserID).Scan(
			&current.ID, &current.UserID, &current.Name, &current.Prefix, &current.CreatedAt, &current.ExpiresAt, &current.RevokedAt, &current.LastUsedAt, &current.TotalCalls)
		if err == nil {
			current.AdminDisabled = adminDisabled
			return current, false, tx.Commit(ctx)
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return key, false, err
		}
	} else {
		if _, err := tx.Exec(ctx, `UPDATE athub_api_keys SET is_current=false,revoked_at=COALESCE(revoked_at,now())
			WHERE user_id=$1 AND is_current`, key.UserID); err != nil {
			return key, false, err
		}
	}
	if adminDisabled {
		return key, false, ErrATHubAdminDisabled
	}
	key.ExpiresAt = nil
	err = tx.QueryRow(ctx, `INSERT INTO athub_api_keys(id,user_id,name,token_hash,prefix,expires_at,is_current)
		VALUES($1,$2,$3,$4,$5,NULL,true) RETURNING created_at`, key.ID, key.UserID, key.Name, key.TokenHash, key.Prefix).Scan(&key.CreatedAt)
	if err != nil {
		return key, false, err
	}
	return key, true, tx.Commit(ctx)
}

func (p *Postgres) ListATHubKeys(ctx context.Context, userID uuid.UUID) ([]ATHubKey, error) {
	rows, err := p.pool.Query(ctx, `SELECT k.id,k.user_id,k.name,k.prefix,k.created_at,k.expires_at,k.revoked_at,k.last_used_at,u.athub_disabled,
		(SELECT COUNT(*) FROM athub_api_calls c WHERE c.key_id=k.id)
		FROM athub_api_keys k JOIN users u ON u.id=k.user_id WHERE k.user_id=$1 AND k.is_current ORDER BY k.created_at DESC,k.id DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	keys := make([]ATHubKey, 0)
	for rows.Next() {
		var key ATHubKey
		if err := rows.Scan(&key.ID, &key.UserID, &key.Name, &key.Prefix, &key.CreatedAt, &key.ExpiresAt, &key.RevokedAt, &key.LastUsedAt, &key.AdminDisabled, &key.TotalCalls); err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	return keys, rows.Err()
}

func (p *Postgres) RevokeATHubKey(ctx context.Context, userID, keyID uuid.UUID) error {
	tag, err := p.pool.Exec(ctx, `UPDATE athub_api_keys SET revoked_at=COALESCE(revoked_at,now()) WHERE id=$1 AND user_id=$2`, keyID, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (p *Postgres) ResolveATHubKey(ctx context.Context, hash string) (ATHubIdentity, error) {
	var identity ATHubIdentity
	err := p.pool.QueryRow(ctx, `SELECT k.id,k.user_id,
		(u.is_active AND NOT u.athub_disabled AND k.is_current AND k.revoked_at IS NULL AND (k.expires_at IS NULL OR k.expires_at>now()))
		FROM athub_api_keys k JOIN users u ON u.id=k.user_id WHERE k.token_hash=$1`, hash).Scan(&identity.KeyID, &identity.UserID, &identity.Active)
	if errors.Is(err, pgx.ErrNoRows) {
		return identity, ErrNotFound
	}
	return identity, err
}

func (p *Postgres) BeginATHubCall(ctx context.Context, call ATHubCall) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	denied := false
	if call.Authenticated {
		var userAllowed, keyAllowed bool
		if call.UserID == nil || call.KeyID == nil {
			return ErrNotFound
		}
		// Use the same lock order as administrator changes and rotations. Recheck
		// before dispatch so an earlier lookup cannot outlive a committed ban.
		if err := tx.QueryRow(ctx, `SELECT is_active AND NOT athub_disabled FROM users WHERE id=$1 FOR SHARE`, call.UserID).Scan(&userAllowed); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT is_current AND revoked_at IS NULL AND (expires_at IS NULL OR expires_at>now())
			FROM athub_api_keys WHERE id=$1 AND user_id=$2 FOR UPDATE`, call.KeyID, call.UserID).Scan(&keyAllowed); err != nil {
			return err
		}
		denied = !userAllowed || !keyAllowed
		if denied {
			call.Authenticated = false
		}
	}
	_, err = tx.Exec(ctx, `INSERT INTO athub_api_calls(id,user_id,key_id,request_id,method,path,port,authenticated)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, call.ID, call.UserID, call.KeyID, call.RequestID, call.Method, call.Path, call.Port, call.Authenticated)
	if err != nil {
		return err
	}
	if call.Authenticated {
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
		FROM athub_api_calls WHERE user_id=$1 ORDER BY started_at DESC,id DESC LIMIT $2 OFFSET $3`, userID, limit, offset)
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
	result := ATHubUsage{Endpoints: make([]ATHubUsageGroup, 0), From: from, Until: until, BucketSeconds: bucketSeconds}
	if err := tx.QueryRow(ctx, `SELECT COUNT(*),COUNT(*) FILTER(WHERE status BETWEEN 200 AND 399),
		COUNT(*) FILTER(WHERE status>=400),COUNT(*) FILTER(WHERE status IS NULL) FROM athub_api_calls
		WHERE user_id=$1 AND ($2::timestamptz IS NULL OR started_at >= $2) AND ($3::timestamptz IS NULL OR started_at < $3)`, userID, from, until).Scan(&result.Total, &result.Success, &result.Failed, &result.Pending); err != nil {
		return result, err
	}
	rows, err := tx.Query(ctx, `SELECT method,path,port,status,COUNT(*) FROM athub_api_calls
		WHERE user_id=$1 AND ($2::timestamptz IS NULL OR started_at >= $2) AND ($3::timestamptz IS NULL OR started_at < $3)
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
			FROM athub_api_calls WHERE user_id=$1 AND started_at >= $2 AND started_at < $3
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
	return result, tx.Commit(ctx)
}
