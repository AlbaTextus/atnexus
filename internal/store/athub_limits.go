package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var ErrATHubCallLimit = errors.New("ATHub key total call limit reached")
var ErrInvalidATHubKeyLimit = errors.New("invalid ATHub key limit")

// Keep configured limits exactly representable in the administrator's browser.
const MaxATHubCallLimit int64 = 9007199254740991

type ATHubKeyLimit struct {
	Role           string    `json:"role"`
	TotalCallLimit *int64    `json:"total_call_limit"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type ATHubCallQuota struct {
	TotalCallLimit *int64 `json:"total_call_limit"`
	RemainingCalls *int64 `json:"remaining_calls"`
}

func ValidATHubKeyLimit(role string, limit *int64) bool {
	return RoleLevel(role) > 0 && (limit == nil || (*limit >= 0 && *limit <= MaxATHubCallLimit))
}

type ATHubKeyLimitStore interface {
	ATHubKeyLimits(context.Context) ([]ATHubKeyLimit, error)
	SetATHubKeyLimit(context.Context, string, *int64, uuid.UUID) (ATHubKeyLimit, error)
}

func (p *Postgres) ATHubKeyLimits(ctx context.Context) ([]ATHubKeyLimit, error) {
	rows, err := p.pool.Query(ctx, `SELECT role,total_call_limit,updated_at FROM athub_key_limits
		ORDER BY CASE role WHEN 'USER' THEN 1 WHEN 'LEVEL_2' THEN 2 WHEN 'ADMIN' THEN 3 WHEN 'SUPER_ADMIN' THEN 4 END`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	limits := make([]ATHubKeyLimit, 0, 4)
	for rows.Next() {
		var limit ATHubKeyLimit
		if err := rows.Scan(&limit.Role, &limit.TotalCallLimit, &limit.UpdatedAt); err != nil {
			return nil, err
		}
		limits = append(limits, limit)
	}
	return limits, rows.Err()
}

func (p *Postgres) SetATHubKeyLimit(ctx context.Context, role string, limit *int64, actor uuid.UUID) (ATHubKeyLimit, error) {
	var result ATHubKeyLimit
	if !ValidATHubKeyLimit(role, limit) {
		return result, ErrInvalidATHubKeyLimit
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return result, err
	}
	defer tx.Rollback(ctx)
	// Wait out calls admitted under the old limit; later ones queue behind this
	// until commit, so the lock and the write share one round trip.
	change := &pgx.Batch{}
	change.Queue(`SELECT pg_advisory_xact_lock(`+athubRoleLock(`$1`)+`)`, role)
	change.Queue(`UPDATE athub_key_limits SET total_call_limit=$2,updated_by=$3,updated_at=clock_timestamp()
		WHERE role=$1 RETURNING role,total_call_limit,updated_at`, role, limit, actor)
	results := tx.SendBatch(ctx, change)
	if _, err = results.Exec(); err == nil {
		err = results.QueryRow().Scan(&result.Role, &result.TotalCallLimit, &result.UpdatedAt)
	}
	if closeErr := results.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return result, err
	}
	return result, tx.Commit(ctx)
}
