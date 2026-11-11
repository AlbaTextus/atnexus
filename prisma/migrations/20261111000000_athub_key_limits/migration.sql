BEGIN;
CREATE TABLE "athub_key_limits" (
    "role" VARCHAR(20) NOT NULL PRIMARY KEY CHECK ("role" IN ('USER', 'ADMIN', 'SUPER_ADMIN')),
    "total_call_limit" BIGINT CHECK ("total_call_limit" BETWEEN 0 AND 9007199254740991),
    "updated_by" UUID,
    "updated_at" TIMESTAMPTZ(6) NOT NULL DEFAULT CURRENT_TIMESTAMP
);
-- NULL preserves unlimited access for every existing account level.
INSERT INTO "athub_key_limits" ("role") VALUES ('USER'), ('ADMIN'), ('SUPER_ADMIN');
COMMIT;
