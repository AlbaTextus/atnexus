BEGIN;

CREATE TABLE athub_reviews (
    id UUID PRIMARY KEY,
    user_id UUID,
    contributor JSONB NOT NULL,
    operation VARCHAR(16) NOT NULL CHECK(operation IN ('upload','edit','delete')),
    method VARCHAR(16) NOT NULL,
    path VARCHAR(255) NOT NULL,
    query TEXT NOT NULL DEFAULT '',
    payload JSONB NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','approved','rejected','failed')),
    reviewer_id UUID,
    reviewer JSONB,
    note TEXT NOT NULL DEFAULT '',
    result_status INTEGER,
    result JSONB,
    created_at TIMESTAMPTZ(6) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    reviewed_at TIMESTAMPTZ(6)
);
CREATE INDEX athub_reviews_status_created_idx ON athub_reviews(status,created_at,id);
CREATE INDEX athub_reviews_user_created_idx ON athub_reviews(user_id,created_at,id);

CREATE TABLE novel_revisions (
    id UUID PRIMARY KEY,
    batch_id UUID NOT NULL,
    novel_id INTEGER NOT NULL CHECK(novel_id>0),
    user_id UUID,
    contributor JSONB NOT NULL,
    changes JSONB NOT NULL CHECK(jsonb_typeof(changes)='array'),
    status VARCHAR(16) NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','completed','failed')),
    created_at TIMESTAMPTZ(6) NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ(6),
    UNIQUE(batch_id,novel_id)
);
CREATE INDEX novel_revisions_novel_history_idx ON novel_revisions(novel_id,created_at DESC,id DESC);
CREATE INDEX novel_revisions_batch_id_idx ON novel_revisions(batch_id);
CREATE INDEX novel_revisions_user_id_idx ON novel_revisions(user_id);

CREATE TABLE creator_migration_state (
    key VARCHAR(32) PRIMARY KEY,
    source_database TEXT NOT NULL,
    source_server TEXT NOT NULL,
    review_digest CHAR(64) NOT NULL,
    history_digest CHAR(64) NOT NULL,
    review_count BIGINT NOT NULL CHECK(review_count>=0),
    history_count BIGINT NOT NULL CHECK(history_count>=0),
    completed_at TIMESTAMPTZ(6) NOT NULL DEFAULT now()
);

COMMIT;
