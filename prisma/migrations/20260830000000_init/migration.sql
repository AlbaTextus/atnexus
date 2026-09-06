CREATE TABLE "users" (
    "id" UUID NOT NULL,
    "email" VARCHAR(320) NOT NULL,
    "password_hash" VARCHAR(255) NOT NULL,
    "display_name" VARCHAR(100),
    "is_active" BOOLEAN NOT NULL DEFAULT true,
    "created_at" TIMESTAMPTZ(6) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updated_at" TIMESTAMPTZ(6) NOT NULL,
    CONSTRAINT "users_pkey" PRIMARY KEY ("id")
);

CREATE TABLE "user_sessions" (
    "id" UUID NOT NULL,
    "token_hash" CHAR(64) NOT NULL,
    "user_id" UUID NOT NULL,
    "expires_at" TIMESTAMPTZ(6) NOT NULL,
    "revoked_at" TIMESTAMPTZ(6),
    "created_at" TIMESTAMPTZ(6) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT "user_sessions_pkey" PRIMARY KEY ("id")
);

CREATE TABLE "passkey_credentials" (
    "id" UUID NOT NULL,
    "user_id" UUID NOT NULL,
    "credential_id" TEXT NOT NULL,
    "public_key" BYTEA NOT NULL,
    "attestation_type" VARCHAR(50) NOT NULL,
    "transports" TEXT[] DEFAULT ARRAY[]::TEXT[],
    "sign_count" BIGINT NOT NULL DEFAULT 0,
    "user_present" BOOLEAN NOT NULL DEFAULT true,
    "user_verified" BOOLEAN NOT NULL DEFAULT false,
    "backup_eligible" BOOLEAN NOT NULL DEFAULT false,
    "backup_state" BOOLEAN NOT NULL DEFAULT false,
    "name" VARCHAR(100) NOT NULL DEFAULT '通行密钥',
    "created_at" TIMESTAMPTZ(6) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "last_used_at" TIMESTAMPTZ(6),
    CONSTRAINT "passkey_credentials_pkey" PRIMARY KEY ("id")
);

CREATE TABLE "webauthn_ceremonies" (
    "id" UUID NOT NULL,
    "token_hash" CHAR(64) NOT NULL,
    "user_id" UUID,
    "kind" VARCHAR(20) NOT NULL,
    "session_data" JSONB NOT NULL,
    "expires_at" TIMESTAMPTZ(6) NOT NULL,
    "used_at" TIMESTAMPTZ(6),
    "created_at" TIMESTAMPTZ(6) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT "webauthn_ceremonies_pkey" PRIMARY KEY ("id")
);

CREATE TABLE "oauth_clients" (
    "id" VARCHAR(100) NOT NULL,
    "name" VARCHAR(100) NOT NULL,
    "secret_hash" VARCHAR(255),
    "redirect_uris" TEXT[] DEFAULT ARRAY[]::TEXT[],
    "allowed_grant_types" TEXT[] DEFAULT ARRAY[]::TEXT[],
    "allowed_scopes" TEXT[] DEFAULT ARRAY[]::TEXT[],
    "access_token_ttl" INTEGER NOT NULL DEFAULT 900,
    "refresh_token_ttl" INTEGER NOT NULL DEFAULT 2592000,
    "is_active" BOOLEAN NOT NULL DEFAULT true,
    "created_at" TIMESTAMPTZ(6) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updated_at" TIMESTAMPTZ(6) NOT NULL,
    CONSTRAINT "oauth_clients_pkey" PRIMARY KEY ("id")
);

CREATE TABLE "authorization_codes" (
    "id" UUID NOT NULL,
    "code_hash" CHAR(64) NOT NULL,
    "client_id" VARCHAR(100) NOT NULL,
    "user_id" UUID NOT NULL,
    "redirect_uri" TEXT NOT NULL,
    "scopes" TEXT[] DEFAULT ARRAY[]::TEXT[],
    "code_challenge" VARCHAR(128),
    "code_challenge_method" VARCHAR(10),
    "expires_at" TIMESTAMPTZ(6) NOT NULL,
    "used_at" TIMESTAMPTZ(6),
    "created_at" TIMESTAMPTZ(6) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT "authorization_codes_pkey" PRIMARY KEY ("id")
);

CREATE TABLE "oauth_tokens" (
    "id" UUID NOT NULL,
    "access_token_hash" CHAR(64) NOT NULL,
    "refresh_token_hash" CHAR(64),
    "client_id" VARCHAR(100) NOT NULL,
    "user_id" UUID,
    "scopes" TEXT[] DEFAULT ARRAY[]::TEXT[],
    "access_expires_at" TIMESTAMPTZ(6) NOT NULL,
    "refresh_expires_at" TIMESTAMPTZ(6),
    "revoked_at" TIMESTAMPTZ(6),
    "created_at" TIMESTAMPTZ(6) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT "oauth_tokens_pkey" PRIMARY KEY ("id")
);

CREATE UNIQUE INDEX "users_email_key" ON "users"("email");
CREATE UNIQUE INDEX "user_sessions_token_hash_key" ON "user_sessions"("token_hash");
CREATE UNIQUE INDEX "passkey_credentials_credential_id_key" ON "passkey_credentials"("credential_id");
CREATE INDEX "passkey_credentials_user_id_idx" ON "passkey_credentials"("user_id");
CREATE UNIQUE INDEX "webauthn_ceremonies_token_hash_key" ON "webauthn_ceremonies"("token_hash");
CREATE INDEX "webauthn_ceremonies_user_id_idx" ON "webauthn_ceremonies"("user_id");
CREATE INDEX "webauthn_ceremonies_expires_at_idx" ON "webauthn_ceremonies"("expires_at");
CREATE INDEX "user_sessions_user_id_idx" ON "user_sessions"("user_id");
CREATE INDEX "user_sessions_expires_at_idx" ON "user_sessions"("expires_at");
CREATE UNIQUE INDEX "authorization_codes_code_hash_key" ON "authorization_codes"("code_hash");
CREATE INDEX "authorization_codes_client_id_idx" ON "authorization_codes"("client_id");
CREATE INDEX "authorization_codes_user_id_idx" ON "authorization_codes"("user_id");
CREATE INDEX "authorization_codes_expires_at_idx" ON "authorization_codes"("expires_at");
CREATE UNIQUE INDEX "oauth_tokens_access_token_hash_key" ON "oauth_tokens"("access_token_hash");
CREATE UNIQUE INDEX "oauth_tokens_refresh_token_hash_key" ON "oauth_tokens"("refresh_token_hash");
CREATE INDEX "oauth_tokens_client_id_idx" ON "oauth_tokens"("client_id");
CREATE INDEX "oauth_tokens_user_id_idx" ON "oauth_tokens"("user_id");
CREATE INDEX "oauth_tokens_access_expires_at_idx" ON "oauth_tokens"("access_expires_at");
CREATE INDEX "oauth_tokens_refresh_expires_at_idx" ON "oauth_tokens"("refresh_expires_at");

ALTER TABLE "user_sessions" ADD CONSTRAINT "user_sessions_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "users"("id") ON DELETE CASCADE ON UPDATE CASCADE;
ALTER TABLE "passkey_credentials" ADD CONSTRAINT "passkey_credentials_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "users"("id") ON DELETE CASCADE ON UPDATE CASCADE;
ALTER TABLE "webauthn_ceremonies" ADD CONSTRAINT "webauthn_ceremonies_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "users"("id") ON DELETE CASCADE ON UPDATE CASCADE;
ALTER TABLE "authorization_codes" ADD CONSTRAINT "authorization_codes_client_id_fkey" FOREIGN KEY ("client_id") REFERENCES "oauth_clients"("id") ON DELETE CASCADE ON UPDATE CASCADE;
ALTER TABLE "authorization_codes" ADD CONSTRAINT "authorization_codes_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "users"("id") ON DELETE CASCADE ON UPDATE CASCADE;
ALTER TABLE "oauth_tokens" ADD CONSTRAINT "oauth_tokens_client_id_fkey" FOREIGN KEY ("client_id") REFERENCES "oauth_clients"("id") ON DELETE CASCADE ON UPDATE CASCADE;
ALTER TABLE "oauth_tokens" ADD CONSTRAINT "oauth_tokens_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "users"("id") ON DELETE CASCADE ON UPDATE CASCADE;
