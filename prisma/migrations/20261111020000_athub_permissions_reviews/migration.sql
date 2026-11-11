BEGIN;
CREATE TABLE athub_role_policies (
    role VARCHAR(20) PRIMARY KEY REFERENCES athub_key_limits(role),
    review_required BOOLEAN NOT NULL DEFAULT false,
    updated_at TIMESTAMPTZ(6) NOT NULL DEFAULT CURRENT_TIMESTAMP
);
INSERT INTO athub_role_policies(role,review_required)
    SELECT role,role='LEVEL_2' FROM athub_key_limits;
CREATE TABLE athub_access_rules (
    role VARCHAR(20) NOT NULL REFERENCES athub_role_policies(role),
    operation VARCHAR(16) NOT NULL CHECK(operation IN ('read','upload','edit','delete','review')),
    method VARCHAR(16) NOT NULL DEFAULT '',
    path VARCHAR(255) NOT NULL DEFAULT '',
    allowed BOOLEAN NOT NULL,
    total_call_limit BIGINT CHECK(total_call_limit BETWEEN 0 AND 9007199254740991),
    PRIMARY KEY(role,operation,method,path),
    CHECK((method='' AND path='') OR (method<>'' AND path<>''))
);
INSERT INTO athub_access_rules(role,operation,allowed)
SELECT role,operation,CASE
    WHEN operation='read' THEN true
    WHEN operation IN ('upload','edit') THEN role IN ('LEVEL_2','ADMIN','SUPER_ADMIN')
    WHEN operation='review' THEN role IN ('ADMIN','SUPER_ADMIN')
    WHEN operation='delete' THEN role='SUPER_ADMIN'
    ELSE false END
FROM athub_role_policies CROSS JOIN (VALUES('read'),('upload'),('edit'),('delete'),('review')) operations(operation);
ALTER TABLE athub_api_calls ADD COLUMN operation VARCHAR(16) NOT NULL DEFAULT '';
UPDATE athub_api_calls SET operation=CASE
    WHEN method='GET' THEN 'read'
    WHEN method='POST' THEN 'upload'
    WHEN method IN ('PUT','PATCH') THEN 'edit'
    WHEN method='DELETE' THEN 'delete' ELSE '' END;
CREATE INDEX athub_calls_key_operation_idx ON athub_api_calls(key_id,operation) WHERE authenticated;
CREATE INDEX athub_calls_user_operation_idx ON athub_api_calls(user_id,operation) WHERE authenticated AND key_id IS NULL;
CREATE TABLE athub_reviews (
    id UUID PRIMARY KEY,
    user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    contributor JSONB NOT NULL,
    operation VARCHAR(16) NOT NULL CHECK(operation IN ('upload','edit','delete')),
    method VARCHAR(16) NOT NULL,
    path VARCHAR(255) NOT NULL,
    query TEXT NOT NULL DEFAULT '',
    payload JSONB NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','approved','rejected','failed')),
    reviewer_id UUID REFERENCES users(id) ON DELETE SET NULL,
    reviewer JSONB,
    note TEXT NOT NULL DEFAULT '',
    result_status INTEGER,
    result JSONB,
    created_at TIMESTAMPTZ(6) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    reviewed_at TIMESTAMPTZ(6)
);
CREATE INDEX athub_reviews_status_created_idx ON athub_reviews(status,created_at,id);
CREATE INDEX athub_reviews_user_created_idx ON athub_reviews(user_id,created_at,id);
COMMIT;
