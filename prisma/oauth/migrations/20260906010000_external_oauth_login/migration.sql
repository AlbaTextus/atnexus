CREATE TABLE "external_identities" (
    "id" UUID PRIMARY KEY,
    "user_id" UUID NOT NULL REFERENCES "users"("id") ON DELETE CASCADE,
    "provider" VARCHAR(40) NOT NULL,
    "subject" VARCHAR(255) NOT NULL,
    "email" VARCHAR(320) NOT NULL DEFAULT '',
    "display_name" VARCHAR(100) NOT NULL DEFAULT '',
    "created_at" TIMESTAMPTZ(6) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT "external_identities_provider_subject_key" UNIQUE ("provider", "subject"),
    CONSTRAINT "external_identities_user_id_provider_key" UNIQUE ("user_id", "provider")
);

CREATE TABLE "external_login_flows" (
    "state_hash" CHAR(64) PRIMARY KEY,
    "browser_hash" CHAR(64) NOT NULL,
    "provider" VARCHAR(40) NOT NULL,
    "verifier_ciphertext" TEXT NOT NULL,
    "return_to" TEXT NOT NULL,
    "user_id" UUID REFERENCES "users"("id") ON DELETE CASCADE,
    "session_hash" CHAR(64) NOT NULL DEFAULT '',
    "expires_at" TIMESTAMPTZ(6) NOT NULL
);
CREATE INDEX "external_login_flows_expires_at_idx" ON "external_login_flows"("expires_at");
