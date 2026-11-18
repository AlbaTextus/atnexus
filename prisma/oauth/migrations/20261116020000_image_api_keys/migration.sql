BEGIN;
CREATE TABLE image_api_keys (
 id TEXT PRIMARY KEY,name TEXT NOT NULL,owner TEXT NOT NULL,
 token_hash CHAR(64) NOT NULL UNIQUE,permissions TEXT[] NOT NULL,
 expires_at TIMESTAMPTZ(6),disabled BOOLEAN NOT NULL DEFAULT FALSE,
 created_at TIMESTAMPTZ(6) NOT NULL DEFAULT now(),updated_at TIMESTAMPTZ(6) NOT NULL DEFAULT now(),
 auto_delete BOOLEAN NOT NULL DEFAULT FALSE
);
CREATE TABLE image_key_imports (
 source_digest CHAR(64) PRIMARY KEY,key_digest CHAR(64) NOT NULL,
 key_count INTEGER NOT NULL CHECK(key_count>=0),completed_at TIMESTAMPTZ(6) NOT NULL DEFAULT now()
);
COMMIT;
