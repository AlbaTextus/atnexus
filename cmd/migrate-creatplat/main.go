package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/AlbaTextus/atnexus/modules/creatplat/storage"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

func main() {
	create := flag.Bool("create-database", false, "create the database specified by CREATPLAT_DATABASE_URL, without migrating data")
	flag.Parse()
	_ = godotenv.Load()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	if err := run(ctx, *create); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, create bool) error {
	targetURL := strings.TrimSpace(os.Getenv("CREATPLAT_DATABASE_URL"))
	if targetURL == "" {
		return fmt.Errorf("CREATPLAT_DATABASE_URL is required")
	}
	if create {
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
	sourceURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if sourceURL == "" {
		return fmt.Errorf("DATABASE_URL is required for importing legacy creator data")
	}
	source, err := pgxpool.New(ctx, sourceURL)
	if err != nil {
		return fmt.Errorf("invalid source database configuration")
	}
	defer source.Close()
	target, err := pgxpool.New(ctx, targetURL)
	if err != nil {
		return fmt.Errorf("invalid creator database configuration")
	}
	defer target.Close()
	result, err := storage.ImportLegacy(ctx, source, target)
	if err != nil {
		return err
	}
	fmt.Printf("Creator import verified: reviews=%d history=%d already_imported=%t\n", result.Reviews, result.History, result.AlreadyImported)
	return nil
}
