// Command create-creatplat-db creates the database CREATPLAT_DATABASE_URL names
// when it does not exist yet. It creates no tables: apply the creator
// migrations afterwards.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	if err := run(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	targetURL := strings.TrimSpace(os.Getenv("CREATPLAT_DATABASE_URL"))
	if targetURL == "" {
		return fmt.Errorf("CREATPLAT_DATABASE_URL is required")
	}
	cfg, err := pgx.ParseConfig(targetURL)
	if err != nil {
		return fmt.Errorf("invalid CREATPLAT_DATABASE_URL")
	}
	name := cfg.Database
	if name == "" {
		return fmt.Errorf("creator database name is required")
	}
	cfg.Database = "postgres"
	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		return fmt.Errorf("could not connect to maintenance database with creator credentials")
	}
	defer conn.Close(ctx)
	var exists bool
	if err := conn.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname=$1)`, name).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		if _, err := conn.Exec(ctx, `CREATE DATABASE `+pgx.Identifier{name}.Sanitize()); err != nil {
			return err
		}
	}
	fmt.Println("Creator database is ready; run npm run db:creatplat:deploy next.")
	return nil
}
