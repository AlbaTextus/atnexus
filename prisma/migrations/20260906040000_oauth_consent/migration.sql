CREATE TABLE oauth_consent_requests (
  token_hash CHAR(64) PRIMARY KEY,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  client_id VARCHAR(100) NOT NULL REFERENCES oauth_clients(id) ON DELETE CASCADE,
  session_hash CHAR(64) NOT NULL,
  request_data JSONB NOT NULL,
  client_version TIMESTAMPTZ(6) NOT NULL,
  expires_at TIMESTAMPTZ(6) NOT NULL
);
CREATE INDEX oauth_consent_requests_expires_at_idx ON oauth_consent_requests(expires_at);
CREATE INDEX oauth_consent_requests_user_id_idx ON oauth_consent_requests(user_id);
