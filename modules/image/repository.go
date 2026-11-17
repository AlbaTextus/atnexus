package image

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("image not found")
var ErrConflict = errors.New("image operation conflicts with another operation")
var ErrInvalid = errors.New("invalid image request")

type File struct {
	ID       string         `json:"name"`
	Value    string         `json:"-"`
	Metadata map[string]any `json:"metadata"`
	Revision int64          `json:"-"`
}

func decodeObject(raw []byte) (map[string]any, error) {
	var m map[string]any
	d := json.NewDecoder(strings.NewReader(string(raw)))
	d.UseNumber()
	err := d.Decode(&m)
	if m == nil && err == nil {
		err = ErrInvalid
	}
	return m, err
}

func scanFile(row interface{ Scan(...any) error }) (File, error) {
	var f File
	var raw []byte
	err := row.Scan(&f.ID, &f.Value, &raw, &f.Revision)
	if errors.Is(err, pgx.ErrNoRows) {
		return f, ErrNotFound
	}
	if err == nil {
		f.Metadata, err = decodeObject(raw)
	}
	return f, err
}

const fileColumns = `id,value,metadata,revision`

func openPool(ctx context.Context, raw string, maxConns int32) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(raw)
	if err != nil {
		return nil, errors.New("invalid IMAGE_DATABASE_URL")
	}
	// An explicit setting wins, then pool_max_conns in the DSN, then 10.
	if maxConns > 0 {
		cfg.MaxConns = maxConns
	} else if !strings.Contains(raw, "pool_max_conns") {
		cfg.MaxConns = 10
	}
	p, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, errors.New("image PostgreSQL pool unavailable")
	}
	if err = p.Ping(ctx); err != nil {
		p.Close()
		return nil, errors.New("image database unavailable")
	}
	for _, q := range []string{`SELECT id,metadata,state,revision FROM image_files LIMIT 0`, `SELECT key,value FROM image_settings LIMIT 0`, `SELECT id,owner,data,expires_at FROM image_uploads LIMIT 0`, `SELECT upload_id,number,size,digest FROM image_upload_parts LIMIT 0`, `SELECT key,source,digest,counts FROM image_imports LIMIT 0`} {
		rows, e := p.Query(ctx, q)
		if e != nil {
			p.Close()
			return nil, fmt.Errorf("image schema not ready; run npm run db:image:deploy: %w", e)
		}
		rows.Close()
	}
	return p, nil
}

func (m *Module) getFile(ctx context.Context, id string) (File, error) {
	return scanFile(m.pool.QueryRow(ctx, `SELECT `+fileColumns+` FROM image_files WHERE id=$1 AND state='ready'`, id))
}
func (m *Module) setting(ctx context.Context, key string) (string, error) {
	var s string
	err := m.pool.QueryRow(ctx, `SELECT value FROM image_settings WHERE key=$1`, key).Scan(&s)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return s, err
}
func (m *Module) setSetting(ctx context.Context, key, value string) error {
	_, err := m.pool.Exec(ctx, `INSERT INTO image_settings(key,value) VALUES($1,$2) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}
func (m *Module) patchMetadata(ctx context.Context, id string, fn func(map[string]any) error) (File, error) {
	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return File{}, err
	}
	defer tx.Rollback(context.WithoutCancel(ctx))
	f, err := scanFile(tx.QueryRow(ctx, `SELECT `+fileColumns+` FROM image_files WHERE id=$1 AND state='ready' FOR UPDATE`, id))
	if err != nil {
		return f, err
	}
	if err = fn(f.Metadata); err != nil {
		return f, err
	}
	raw, err := json.Marshal(f.Metadata)
	if err != nil {
		return f, err
	}
	_, err = tx.Exec(ctx, `UPDATE image_files SET metadata=$2,revision=revision+1,updated_at=now() WHERE id=$1`, id, raw)
	if err == nil {
		err = tx.Commit(ctx)
	}
	return f, err
}

// CheckSeparateDatabases compares actual server/database identities, including
// connections whose URL hostnames differ or whose schema settings differ.
func (m *Module) CheckSeparateDatabases(ctx context.Context, urls ...string) error {
	var current string
	q := `SELECT current_database()||'@'||COALESCE(inet_server_addr()::text,'local')||':'||COALESCE(inet_server_port()::text,'socket')`
	if err := m.pool.QueryRow(ctx, q).Scan(&current); err != nil {
		return err
	}
	for _, raw := range urls {
		other, err := pgxpool.New(ctx, raw)
		if err != nil {
			return errors.New("database isolation check failed")
		}
		var id string
		err = other.QueryRow(ctx, q).Scan(&id)
		other.Close()
		if err != nil {
			return errors.New("database isolation check failed")
		}
		if id == current {
			return errors.New("IMAGE_DATABASE_URL must use its own database")
		}
	}
	return nil
}
