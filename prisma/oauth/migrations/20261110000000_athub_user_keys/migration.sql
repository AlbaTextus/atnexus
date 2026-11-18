-- Authentication and accounting belong to the account database, never the content database.
BEGIN;
CREATE TABLE "athub_api_keys" (
    "id" UUID NOT NULL PRIMARY KEY,
    "user_id" UUID NOT NULL REFERENCES "users"("id") ON DELETE CASCADE,
    "name" VARCHAR(100) NOT NULL,
    "token_hash" CHAR(64) NOT NULL UNIQUE,
    "prefix" VARCHAR(20) NOT NULL,
    "created_at" TIMESTAMPTZ(6) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "expires_at" TIMESTAMPTZ(6),
    "revoked_at" TIMESTAMPTZ(6),
    "last_used_at" TIMESTAMPTZ(6)
);
CREATE INDEX "athub_api_keys_user_id_created_at_idx" ON "athub_api_keys"("user_id", "created_at");

CREATE TABLE "athub_api_calls" (
    "id" UUID NOT NULL PRIMARY KEY,
    "user_id" UUID REFERENCES "users"("id") ON DELETE SET NULL,
    "key_id" UUID REFERENCES "athub_api_keys"("id") ON DELETE SET NULL,
    "request_id" UUID NOT NULL UNIQUE,
    "method" VARCHAR(16) NOT NULL,
    "path" VARCHAR(255) NOT NULL,
    "port" INTEGER NOT NULL CHECK ("port" BETWEEN 1 AND 65535),
    "authenticated" BOOLEAN NOT NULL,
    "status" INTEGER CHECK ("status" BETWEEN 100 AND 599),
    "duration_ms" BIGINT CHECK ("duration_ms" >= 0),
    "response_bytes" BIGINT CHECK ("response_bytes" >= 0),
    "started_at" TIMESTAMPTZ(6) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "completed_at" TIMESTAMPTZ(6)
);
CREATE INDEX "athub_api_calls_user_id_started_at_idx" ON "athub_api_calls"("user_id", "started_at");
CREATE INDEX "athub_api_calls_key_id_started_at_idx" ON "athub_api_calls"("key_id", "started_at");
CREATE INDEX "athub_api_calls_started_at_idx" ON "athub_api_calls"("started_at");
COMMIT;
