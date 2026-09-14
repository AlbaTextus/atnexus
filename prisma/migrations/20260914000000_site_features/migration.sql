CREATE TABLE "site_features" (
    "id" INTEGER NOT NULL DEFAULT 1,
    "external_login_enabled" BOOLEAN NOT NULL DEFAULT true,
    "registration_enabled" BOOLEAN NOT NULL DEFAULT true,
    "turnstile_enabled" BOOLEAN NOT NULL DEFAULT true,
    "updated_by" UUID,
    "updated_at" TIMESTAMPTZ(6) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT "site_features_pkey" PRIMARY KEY ("id"),
    CONSTRAINT "site_features_singleton" CHECK ("id" = 1)
);

INSERT INTO "site_features" (
    "id",
    "external_login_enabled",
    "registration_enabled",
    "turnstile_enabled"
) VALUES (1, true, true, true);
