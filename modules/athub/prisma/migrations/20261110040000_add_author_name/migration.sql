-- Existing identifiers and novel references are retained; unknown names stay NULL.
ALTER TABLE "Author" ADD COLUMN "name" VARCHAR(200);
