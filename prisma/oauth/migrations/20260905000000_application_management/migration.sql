ALTER TABLE "oauth_clients"
  ADD COLUMN "owner_id" UUID,
  ADD COLUMN "homepage_url" VARCHAR(2048) NOT NULL DEFAULT '',
  ADD COLUMN "description" VARCHAR(1000) NOT NULL DEFAULT '',
  ADD COLUMN "icon_url" VARCHAR(2048) NOT NULL DEFAULT '',
  ADD COLUMN "minimum_role" VARCHAR(20) NOT NULL DEFAULT 'USER';

ALTER TABLE "oauth_clients" ADD CONSTRAINT "oauth_clients_owner_id_fkey"
  FOREIGN KEY ("owner_id") REFERENCES "users"("id") ON DELETE RESTRICT ON UPDATE CASCADE;
ALTER TABLE "oauth_clients" ADD CONSTRAINT "oauth_clients_minimum_role_check"
  CHECK ("minimum_role" IN ('USER', 'ADMIN', 'SUPER_ADMIN'));
CREATE INDEX "oauth_clients_owner_id_idx" ON "oauth_clients"("owner_id");
