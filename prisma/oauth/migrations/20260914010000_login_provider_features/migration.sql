ALTER TABLE "site_features"
    ADD COLUMN "provider_login_enabled" JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD CONSTRAINT "site_features_provider_settings_object"
        CHECK (jsonb_typeof("provider_login_enabled") = 'object');
