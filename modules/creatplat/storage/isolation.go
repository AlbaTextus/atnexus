package storage

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgxpool"
)

type databaseIdentity struct{ Database, Server string }

func identify(ctx context.Context, pool *pgxpool.Pool) (databaseIdentity, error) {
	var id databaseIdentity
	err := pool.QueryRow(ctx, `SELECT current_database(),COALESCE(inet_server_addr()::text,'local')||':'||COALESCE(inet_server_port()::text,'socket')`).Scan(&id.Database, &id.Server)
	return id, err
}

// RequireSeparateDatabases rejects a second schema or credential pointing at
// the same physical database. Creator data cannot silently use an account pool.
func (p *Postgres) RequireSeparateDatabases(ctx context.Context, urls ...string) error {
	current, err := identify(ctx, p.pool)
	if err != nil {
		return err
	}
	for _, raw := range urls {
		other, err := pgxpool.New(ctx, raw)
		if err != nil {
			return errors.New("could not check database isolation")
		}
		id, err := identify(ctx, other)
		other.Close()
		if err != nil {
			return errors.New("could not check database isolation")
		}
		if current == id {
			return errors.New("CREATPLAT_DATABASE_URL must identify a database separate from OAuth and ATHub")
		}
	}
	return nil
}
