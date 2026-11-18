-- Keyless call records are purged by age. This index lets each purge find them
-- without walking past the key calls that are kept.
CREATE INDEX athub_api_calls_keyless_started_at_idx ON athub_api_calls(started_at) WHERE key_id IS NULL;
