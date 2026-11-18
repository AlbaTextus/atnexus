CREATE TABLE communications (
 id UUID PRIMARY KEY,
 kind VARCHAR(20) NOT NULL CHECK (kind IN ('announcement','email','both')),
 title VARCHAR(100) NOT NULL,
 content TEXT NOT NULL,
 audience VARCHAR(20) NOT NULL DEFAULT 'all' CHECK (audience IN ('all','admins','self')),
 status VARCHAR(20) NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','published','archived','queued')),
 revision INTEGER NOT NULL DEFAULT 1,
 created_by UUID NOT NULL,
 created_at TIMESTAMPTZ(6) NOT NULL DEFAULT NOW(),
 updated_at TIMESTAMPTZ(6) NOT NULL DEFAULT NOW(),
 published_at TIMESTAMPTZ(6)
);
CREATE INDEX communications_kind_created_at_idx ON communications(kind, created_at DESC);
CREATE TABLE mail_deliveries (
 id UUID PRIMARY KEY,
 communication_id UUID NOT NULL REFERENCES communications(id) ON DELETE CASCADE,
 recipient VARCHAR(320) NOT NULL,
 status VARCHAR(20) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','processing','sent','failed')),
 updated_at TIMESTAMPTZ(6) NOT NULL DEFAULT NOW(),
 UNIQUE(communication_id,recipient)
);
CREATE INDEX mail_deliveries_status_updated_at_idx ON mail_deliveries(status,updated_at);
