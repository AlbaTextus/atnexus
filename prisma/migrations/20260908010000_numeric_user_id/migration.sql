BEGIN;
LOCK TABLE users IN ACCESS EXCLUSIVE MODE;
CREATE SEQUENCE users_numeric_id_seq AS INTEGER;
ALTER TABLE users ADD COLUMN numeric_id INTEGER;
WITH numbered AS (
  SELECT id, row_number() OVER (ORDER BY created_at, id)::INTEGER AS number FROM users
)
UPDATE users SET numeric_id=numbered.number FROM numbered WHERE users.id=numbered.id;
SELECT setval('users_numeric_id_seq', COALESCE(MAX(numeric_id), 1), COUNT(*) > 0) FROM users;
ALTER TABLE users ALTER COLUMN numeric_id SET DEFAULT nextval('users_numeric_id_seq');
ALTER TABLE users ALTER COLUMN numeric_id SET NOT NULL;
ALTER SEQUENCE users_numeric_id_seq OWNED BY users.numeric_id;
CREATE UNIQUE INDEX users_numeric_id_key ON users(numeric_id);
COMMIT;
