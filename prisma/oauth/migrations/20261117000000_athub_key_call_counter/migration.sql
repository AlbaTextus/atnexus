-- Admitted calls per key, maintained at admission so the quota check does not
-- count the call log on every request.
ALTER TABLE "athub_api_keys" ADD COLUMN "authenticated_calls" BIGINT NOT NULL DEFAULT 0;
UPDATE "athub_api_keys" k SET "authenticated_calls" =
  (SELECT COUNT(*) FROM "athub_api_calls" c WHERE c."key_id" = k."id" AND c."authenticated");
