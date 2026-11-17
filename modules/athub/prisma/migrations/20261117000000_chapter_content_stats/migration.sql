-- Stored per-chapter statistics so history snapshots and the table of contents
-- never read chapter bodies. Adding stored generated columns rewrites the table once.
ALTER TABLE "Chapter"
  ADD COLUMN "contentHash" TEXT GENERATED ALWAYS AS (md5("content")) STORED,
  ADD COLUMN "contentLength" INTEGER GENERATED ALWAYS AS (length("content")) STORED,
  ADD COLUMN "wordCount" INTEGER GENERATED ALWAYS AS (char_length(regexp_replace("content", '[\u0009-\u000D\u0020\u00A0\u1680\u2000-\u200A\u2028\u2029\u202F\u205F\u3000\uFEFF]+', '', 'g'))) STORED;
