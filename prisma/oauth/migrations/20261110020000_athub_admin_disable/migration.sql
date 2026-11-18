BEGIN;
ALTER TABLE users ADD COLUMN athub_disabled BOOLEAN NOT NULL DEFAULT false;
UPDATE users SET athub_disabled = true WHERE NOT is_active;
UPDATE athub_api_keys k SET revoked_at = COALESCE(k.revoked_at, now())
FROM users u WHERE k.user_id = u.id AND NOT u.is_active;
COMMIT;
