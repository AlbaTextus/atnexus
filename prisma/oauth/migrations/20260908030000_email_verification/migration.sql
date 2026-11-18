ALTER TABLE users ADD COLUMN email_verified BOOLEAN NOT NULL DEFAULT false;

CREATE TABLE email_challenges (
  id UUID PRIMARY KEY,
  purpose VARCHAR(20) NOT NULL,
  email VARCHAR(320) NOT NULL,
  actor VARCHAR(128) NOT NULL,
  previous_email VARCHAR(320) NOT NULL DEFAULT '',
  code_hash CHAR(64) NOT NULL,
  attempts INTEGER NOT NULL DEFAULT 0,
  ready BOOLEAN NOT NULL DEFAULT false,
  expires_at TIMESTAMPTZ(6) NOT NULL,
  UNIQUE (purpose, email, actor)
);
CREATE INDEX email_challenges_expires_at_idx ON email_challenges(expires_at);

CREATE TABLE email_rate_limits (
  key CHAR(64) PRIMARY KEY,
  count INTEGER NOT NULL,
  expires_at TIMESTAMPTZ(6) NOT NULL
);
CREATE INDEX email_rate_limits_expires_at_idx ON email_rate_limits(expires_at);
