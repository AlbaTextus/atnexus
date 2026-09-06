-- Add a small, explicit role marker so bootstrap/operator accounts can be
-- distinguished from regular users. Authorization endpoints remain unchanged;
-- application-specific admin routes can enforce this value.
ALTER TABLE "users"
ADD COLUMN "role" VARCHAR(20) NOT NULL DEFAULT 'USER';

ALTER TABLE "users"
ADD CONSTRAINT "users_role_check" CHECK ("role" IN ('USER', 'ADMIN', 'SUPER_ADMIN'));
