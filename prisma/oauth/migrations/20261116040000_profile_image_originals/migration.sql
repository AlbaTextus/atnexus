ALTER TABLE "users"
  ADD COLUMN "avatar_original_url" VARCHAR(2048) NOT NULL DEFAULT '',
  ADD COLUMN "background_original_url" VARCHAR(2048) NOT NULL DEFAULT '';
