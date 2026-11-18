-- Normalize display copy only. Preserve URLs, email addresses, client IDs and
-- technical identifiers, including dotted keys and repository paths.
WITH normalized AS (
  SELECT slug,
    regexp_replace(title, '(?<![A-Za-z0-9_./:@-])albatextus(?![A-Za-z0-9_/@-]|\.[A-Za-z0-9])', 'ALBATEXTUS', 'gi') AS title,
    regexp_replace(description, '(?<![A-Za-z0-9_./:@-])albatextus(?![A-Za-z0-9_/@-]|\.[A-Za-z0-9])', 'ALBATEXTUS', 'gi') AS description,
    regexp_replace(content, '(?<![A-Za-z0-9_./:@-])albatextus(?![A-Za-z0-9_/@-]|\.[A-Za-z0-9])', 'ALBATEXTUS', 'gi') AS content
  FROM legal_documents
)
UPDATE legal_documents AS d
SET title=n.title, description=n.description, content=n.content, revision=d.revision+1, updated_at=NOW()
FROM normalized AS n
WHERE d.slug=n.slug AND (d.title,d.description,d.content) IS DISTINCT FROM (n.title,n.description,n.content);

WITH normalized AS (
  SELECT id,
    regexp_replace(name, '(?<![A-Za-z0-9_./:@-])albatextus(?![A-Za-z0-9_/@-]|\.[A-Za-z0-9])', 'ALBATEXTUS', 'gi') AS name,
    regexp_replace(description, '(?<![A-Za-z0-9_./:@-])albatextus(?![A-Za-z0-9_/@-]|\.[A-Za-z0-9])', 'ALBATEXTUS', 'gi') AS description
  FROM oauth_clients
)
UPDATE oauth_clients AS c
SET name=n.name, description=n.description, updated_at=NOW()
FROM normalized AS n
WHERE c.id=n.id AND (c.name,c.description) IS DISTINCT FROM (n.name,n.description);
