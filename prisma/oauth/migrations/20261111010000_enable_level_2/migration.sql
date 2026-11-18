BEGIN;
ALTER TABLE "users" DROP CONSTRAINT "users_role_check";
ALTER TABLE "users" ADD CONSTRAINT "users_role_check"
    CHECK ("role" IN ('USER', 'LEVEL_2', 'ADMIN', 'SUPER_ADMIN'));

ALTER TABLE "oauth_clients" DROP CONSTRAINT "oauth_clients_minimum_role_check";
ALTER TABLE "oauth_clients" ADD CONSTRAINT "oauth_clients_minimum_role_check"
    CHECK ("minimum_role" IN ('USER', 'LEVEL_2', 'ADMIN', 'SUPER_ADMIN'));

ALTER TABLE "athub_key_limits" DROP CONSTRAINT "athub_key_limits_role_check";
ALTER TABLE "athub_key_limits" ADD CONSTRAINT "athub_key_limits_role_check"
    CHECK ("role" IN ('USER', 'LEVEL_2', 'ADMIN', 'SUPER_ADMIN'));
INSERT INTO "athub_key_limits" ("role") VALUES ('LEVEL_2');
COMMIT;
