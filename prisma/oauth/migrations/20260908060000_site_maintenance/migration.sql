CREATE TABLE "site_maintenance" (
    "id" INTEGER NOT NULL DEFAULT 1,
    "enabled" BOOLEAN NOT NULL DEFAULT false,
    "updated_by" UUID,
    "updated_at" TIMESTAMPTZ(6) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT "site_maintenance_pkey" PRIMARY KEY ("id"),
    CONSTRAINT "site_maintenance_singleton" CHECK ("id" = 1)
);

INSERT INTO "site_maintenance" ("id", "enabled") VALUES (1, false);
