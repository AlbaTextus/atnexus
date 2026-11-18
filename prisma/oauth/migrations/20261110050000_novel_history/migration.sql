CREATE TABLE novel_revisions (
    id UUID PRIMARY KEY,
    batch_id UUID NOT NULL,
    novel_id INTEGER NOT NULL CHECK (novel_id > 0),
    user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    contributor JSONB NOT NULL,
    changes JSONB NOT NULL CHECK (jsonb_typeof(changes) = 'array'),
    status VARCHAR(16) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','completed','failed')),
    created_at TIMESTAMPTZ(6) NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ(6),
    UNIQUE (batch_id, novel_id)
);
-- Novel IDs reference the separate content database; history survives deletion.
CREATE INDEX novel_revisions_novel_history_idx ON novel_revisions (novel_id, created_at DESC, id DESC);
CREATE INDEX novel_revisions_batch_id_idx ON novel_revisions (batch_id);
CREATE INDEX novel_revisions_user_id_idx ON novel_revisions (user_id);
