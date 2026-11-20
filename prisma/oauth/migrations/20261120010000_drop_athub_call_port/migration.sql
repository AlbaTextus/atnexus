-- The port a call arrived on. The backend listens on a single address, so the
-- value no longer tells calls apart and nothing shows it. The recorded ports
-- are deleted for good.
ALTER TABLE athub_api_calls DROP COLUMN IF EXISTS port;
