-- Non-whitespace Unicode characters, maintained by PostgreSQL on every write so
-- the table of contents never has to read chapter bodies. Adding a stored
-- generated column rewrites "Chapter" once and backfills existing rows.
ALTER TABLE "Chapter" ADD COLUMN "wordCount" INTEGER NOT NULL GENERATED ALWAYS AS (
    char_length(regexp_replace("content", '[\u0009-\u000D\u0020\u00A0\u1680\u2000-\u200A\u2028\u2029\u202F\u205F\u3000\uFEFF]+', '', 'g'))
) STORED;
