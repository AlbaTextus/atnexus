BEGIN;
ALTER TABLE "athub_api_keys" ADD COLUMN "is_current" BOOLEAN NOT NULL DEFAULT true;

-- Prefer the newest usable key. Retain older keys and their audit references.
WITH ranked AS (
    SELECT id, row_number() OVER (
        PARTITION BY user_id
        ORDER BY (revoked_at IS NULL AND (expires_at IS NULL OR expires_at > now())) DESC,
                 created_at DESC, id DESC
    ) AS position
    FROM athub_api_keys
)
UPDATE athub_api_keys k
SET is_current = (r.position = 1),
    revoked_at = CASE WHEN r.position > 1 OR k.expires_at <= now()
                      THEN COALESCE(k.revoked_at, now()) ELSE k.revoked_at END,
    expires_at = NULL
FROM ranked r WHERE k.id = r.id;

-- Disabled keys still occupy the slot; only an explicit reset replaces them.
CREATE UNIQUE INDEX "athub_api_keys_one_current_per_user_idx"
ON "athub_api_keys"("user_id") WHERE "is_current";
COMMIT;
