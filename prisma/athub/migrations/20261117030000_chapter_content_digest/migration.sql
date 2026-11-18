-- md5 and length of every chapter body, maintained by PostgreSQL on every write
-- so history snapshots compare chapters without reading their bodies. Adding
-- stored generated columns rewrites "Chapter" once and backfills existing rows.
ALTER TABLE "Chapter"
    ADD COLUMN "contentHash" TEXT NOT NULL GENERATED ALWAYS AS (md5("content")) STORED,
    ADD COLUMN "contentLength" INTEGER NOT NULL GENERATED ALWAYS AS (length("content")) STORED;
