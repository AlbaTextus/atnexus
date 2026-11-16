BEGIN;
CREATE TABLE image_files (
 id TEXT PRIMARY KEY,
 value TEXT NOT NULL DEFAULT '',
 metadata JSONB NOT NULL CHECK(jsonb_typeof(metadata)='object'),
 state VARCHAR(16) NOT NULL DEFAULT 'ready' CHECK(state IN ('pending','ready','deleting','moving')),
 revision BIGINT NOT NULL DEFAULT 1,
 created_at TIMESTAMPTZ(6) NOT NULL DEFAULT now(),
 updated_at TIMESTAMPTZ(6) NOT NULL DEFAULT now()
);
CREATE INDEX image_files_time ON image_files ((metadata->>'TimeStamp') DESC,id);
CREATE INDEX image_files_channel ON image_files ((metadata->>'ChannelName'));
CREATE INDEX image_files_metadata ON image_files USING GIN(metadata);
CREATE UNIQUE INDEX image_files_upload_receipt ON image_files ((metadata->>'UploadSession')) WHERE metadata ? 'UploadSession';
CREATE TABLE image_settings (key TEXT PRIMARY KEY,value TEXT NOT NULL);
CREATE TABLE image_legacy_rows (
 source_table TEXT NOT NULL,row_key TEXT NOT NULL,ciphertext TEXT NOT NULL,
 digest CHAR(64) NOT NULL,PRIMARY KEY(source_table,row_key)
);
CREATE TABLE image_imports (
 key TEXT PRIMARY KEY,source TEXT NOT NULL,digest CHAR(64) NOT NULL,
 counts JSONB NOT NULL,completed TIMESTAMPTZ(6) NOT NULL DEFAULT now()
);
CREATE TABLE image_uploads (
 id TEXT PRIMARY KEY,owner TEXT NOT NULL,data JSONB NOT NULL,expires_at TIMESTAMPTZ(6) NOT NULL
);
CREATE INDEX image_uploads_expiry ON image_uploads(expires_at);
CREATE TABLE image_upload_parts (
 upload_id TEXT NOT NULL REFERENCES image_uploads(id) ON DELETE CASCADE,
 number INTEGER NOT NULL CHECK(number>=0),size BIGINT NOT NULL CHECK(size>=0),digest CHAR(64) NOT NULL,
 PRIMARY KEY(upload_id,number)
);
COMMIT;
