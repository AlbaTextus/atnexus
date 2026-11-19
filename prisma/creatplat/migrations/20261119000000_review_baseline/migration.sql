-- What a submission alters, as the content API found when it rehearsed the
-- submission on arrival. Approval hands it back, so that only what the
-- submission altered is written, and the submission is refused when that
-- content has been changed since. Reviews queued before this column existed
-- keep NULL and are applied in full, as before. A nullable column without a
-- default does not rewrite the table.
ALTER TABLE athub_reviews ADD COLUMN baseline JSONB;
