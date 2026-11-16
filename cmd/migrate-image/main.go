package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/AlbaTextus/atnexus/modules/image/migration"
	"github.com/AlbaTextus/atnexus/modules/oauth"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

func main() {
	source := flag.String("source", "", "read-only source SQLite path (or IMAGE_LEGACY_SQLITE)")
	flag.Parse()
	_ = godotenv.Load()
	if *source == "" {
		*source = os.Getenv("IMAGE_LEGACY_SQLITE")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	if err := run(ctx, *source); err != nil {
		fmt.Fprintln(os.Stderr, "Image migration failed:", err)
		os.Exit(1)
	}
}
func run(ctx context.Context, source string) error {
	if source == "" || os.Getenv("IMAGE_DATABASE_URL") == "" || os.Getenv("DATABASE_URL") == "" {
		return fmt.Errorf("source, IMAGE_DATABASE_URL and DATABASE_URL are required")
	}
	target, err := pgxpool.New(ctx, os.Getenv("IMAGE_DATABASE_URL"))
	if err != nil {
		return fmt.Errorf("invalid image database configuration")
	}
	defer target.Close()
	keys, err := oauth.NewImageKeys(ctx, os.Getenv("DATABASE_URL"), nil)
	if err != nil {
		return err
	}
	defer keys.Close()
	result, err := migration.Import(ctx, migration.Options{SQLitePath: source, LocalRoot: os.Getenv("IMAGE_LOCAL_ROOT"), EncryptionKey: os.Getenv("CLIENT_SECRET_ENCRYPTION_KEY"), Target: target, Keys: keys})
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(result)
}
