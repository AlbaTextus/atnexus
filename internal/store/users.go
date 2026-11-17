package store

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var ErrUserForbidden = errors.New("user management forbidden")

type ManagedUser struct {
	User
	UpdatedAt       time.Time
	LastLoginDevice *LoginDevice
}

type UserFilter struct {
	Search, Role, Status string
	Page, PageSize       int
}

type UserList struct {
	Users                      []ManagedUser
	Total, All, Active, Admins int
}

type UserChange struct {
	DisplayName *string
	Role        string
	IsActive    bool
	UpdatedAt   time.Time
}

type UserManagementStore interface {
	ListUsers(context.Context, UserFilter) (UserList, error)
	CreateManagedUser(context.Context, uuid.UUID, UserSeed) (ManagedUser, error)
	UpdateManagedUser(context.Context, uuid.UUID, uuid.UUID, UserChange) (ManagedUser, error)
	ResetUserPassword(context.Context, uuid.UUID, uuid.UUID, time.Time, string) error
	RevokeUserSessions(context.Context, uuid.UUID, uuid.UUID, time.Time) error
}

func scanManagedUser(row pgx.Row) (ManagedUser, error) {
	var u ManagedUser
	err := row.Scan(append(userScanDestinations(&u.User), &u.UpdatedAt, &u.LastLoginDevice)...)
	return u, err
}

func (p *Postgres) ListUsers(ctx context.Context, filter UserFilter) (UserList, error) {
	result := UserList{Users: make([]ManagedUser, 0)}
	// Search terms are literal, including SQL LIKE wildcard characters.
	search := "%" + strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(filter.Search) + "%"
	where := ` WHERE (email ILIKE $1 OR display_name ILIKE $1 OR id::text ILIKE $1)
	 AND ($2='' OR role=$2) AND ($3='' OR is_active=($3='active'))`
	if err := p.pool.QueryRow(ctx, `SELECT COUNT(*) FROM users`+where, search, filter.Role, filter.Status).Scan(&result.Total); err != nil {
		return result, err
	}
	if err := p.pool.QueryRow(ctx, `SELECT COUNT(*),COUNT(*) FILTER (WHERE is_active),COUNT(*) FILTER (WHERE role IN ('ADMIN','SUPER_ADMIN')) FROM users`).Scan(&result.All, &result.Active, &result.Admins); err != nil {
		return result, err
	}
	rows, err := p.pool.Query(ctx, `SELECT `+userColumns+`,users.updated_at,users.last_login_device FROM users`+where+` ORDER BY created_at DESC,id LIMIT $4 OFFSET $5`, search, filter.Role, filter.Status, filter.PageSize, (filter.Page-1)*filter.PageSize)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		u, err := scanManagedUser(rows)
		if err != nil {
			return result, err
		}
		result.Users = append(result.Users, u)
	}
	return result, rows.Err()
}

// Serialize administrative changes and recheck the actor under a row lock.
// An in-flight request cannot retain privileges after another admin demotes it.
func (p *Postgres) userManagementTx(ctx context.Context, actorID uuid.UUID) (pgx.Tx, User, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return nil, User{}, err
	}
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(742190613)`); err != nil {
		_ = tx.Rollback(ctx)
		return nil, User{}, err
	}
	actor, err := scanUser(tx.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE id=$1 FOR UPDATE`, actorID))
	if err == nil && (!actor.IsActive || (actor.Role != "ADMIN" && actor.Role != "SUPER_ADMIN")) {
		err = ErrUserForbidden
	}
	if err != nil {
		_ = tx.Rollback(ctx)
		return nil, User{}, err
	}
	return tx, actor, nil
}

func canManageUser(actor User, targetRole string) bool {
	return actor.Role == "SUPER_ADMIN" || (actor.Role == "ADMIN" && (targetRole == "USER" || targetRole == "LEVEL_2"))
}

func (p *Postgres) CreateManagedUser(ctx context.Context, actorID uuid.UUID, input UserSeed) (ManagedUser, error) {
	tx, actor, err := p.userManagementTx(ctx, actorID)
	if err != nil {
		return ManagedUser{}, err
	}
	defer tx.Rollback(ctx)
	if !canManageUser(actor, input.Role) {
		return ManagedUser{}, ErrUserForbidden
	}
	u, err := scanManagedUser(tx.QueryRow(ctx, `INSERT INTO users (id,email,password_hash,display_name,role,is_active,created_at,updated_at)
	 VALUES ($1,$2,$3,$4,$5,true,NOW(),NOW()) RETURNING `+userColumns+`,users.updated_at,users.last_login_device`, uuid.New(), input.Email, input.PasswordHash, input.DisplayName, input.Role))
	if isUniqueViolation(err) {
		return ManagedUser{}, ErrAlreadyExists
	}
	if err != nil {
		return ManagedUser{}, err
	}
	return u, tx.Commit(ctx)
}

func managedTarget(ctx context.Context, tx pgx.Tx, actor User, id uuid.UUID, revision time.Time) (ManagedUser, error) {
	u, err := scanManagedUser(tx.QueryRow(ctx, `SELECT `+userColumns+`,users.updated_at,users.last_login_device FROM users WHERE id=$1 FOR UPDATE`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return u, ErrNotFound
	}
	if err != nil {
		return u, err
	}
	if !canManageUser(actor, u.Role) {
		return u, ErrUserForbidden
	}
	if !u.UpdatedAt.Equal(revision) {
		return u, ErrConflict
	}
	return u, nil
}

func revokeManagedUserAccess(ctx context.Context, tx pgx.Tx, id uuid.UUID) error {
	for _, query := range []string{
		`UPDATE user_sessions SET revoked_at=NOW() WHERE user_id=$1 AND revoked_at IS NULL`,
		`UPDATE oauth_tokens SET revoked_at=NOW() WHERE user_id=$1 AND revoked_at IS NULL`,
		`UPDATE authorization_codes SET used_at=NOW() WHERE user_id=$1 AND used_at IS NULL`,
		`DELETE FROM external_login_flows WHERE user_id=$1`,
		`DELETE FROM oauth_consent_requests WHERE user_id=$1`,
		`DELETE FROM webauthn_ceremonies WHERE user_id=$1`,
	} {
		if _, err := tx.Exec(ctx, query, id); err != nil {
			return err
		}
	}
	return nil
}

func (p *Postgres) UpdateManagedUser(ctx context.Context, actorID, id uuid.UUID, input UserChange) (ManagedUser, error) {
	tx, actor, err := p.userManagementTx(ctx, actorID)
	if err != nil {
		return ManagedUser{}, err
	}
	defer tx.Rollback(ctx)
	previous, err := managedTarget(ctx, tx, actor, id, input.UpdatedAt)
	if err != nil {
		return ManagedUser{}, err
	}
	if !canManageUser(actor, input.Role) || (actorID == id && (input.Role != previous.Role || !input.IsActive)) {
		return ManagedUser{}, ErrUserForbidden
	}
	u, err := scanManagedUser(tx.QueryRow(ctx, `UPDATE users SET display_name=$2,role=$3,is_active=$4,
		athub_disabled=CASE WHEN NOT $4 THEN true ELSE athub_disabled END,updated_at=clock_timestamp() WHERE id=$1 RETURNING `+userColumns+`,users.updated_at,users.last_login_device`, id, input.DisplayName, input.Role, input.IsActive))
	if err != nil {
		return ManagedUser{}, err
	}
	if previous.Role != input.Role || previous.IsActive != input.IsActive {
		if err := revokeManagedUserAccess(ctx, tx, id); err != nil {
			return ManagedUser{}, err
		}
	}
	if !input.IsActive {
		if _, err := tx.Exec(ctx, `UPDATE athub_api_keys SET revoked_at=COALESCE(revoked_at,now()) WHERE user_id=$1`, id); err != nil {
			return ManagedUser{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return ManagedUser{}, err
	}
	return u, nil
}

func (p *Postgres) ResetUserPassword(ctx context.Context, actorID, id uuid.UUID, revision time.Time, hash string) error {
	return p.updateManagedUserAccess(ctx, actorID, id, revision, &hash)
}

func (p *Postgres) RevokeUserSessions(ctx context.Context, actorID, id uuid.UUID, revision time.Time) error {
	return p.updateManagedUserAccess(ctx, actorID, id, revision, nil)
}

func (p *Postgres) updateManagedUserAccess(ctx context.Context, actorID, id uuid.UUID, revision time.Time, hash *string) error {
	tx, actor, err := p.userManagementTx(ctx, actorID)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if actorID == id {
		return ErrUserForbidden
	}
	if _, err := managedTarget(ctx, tx, actor, id, revision); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE users SET password_hash=COALESCE($2,password_hash),updated_at=clock_timestamp() WHERE id=$1`, id, hash); err != nil {
		return err
	}
	if err := revokeManagedUserAccess(ctx, tx, id); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return nil
}
