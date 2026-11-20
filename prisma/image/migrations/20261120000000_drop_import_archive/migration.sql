-- What the one-time import from the SQLite image host left behind: the
-- encrypted copy of every source row and the record of the import. Their rows
-- are deleted for good. Files, settings and uploads are in the other tables
-- and stay.
DROP TABLE IF EXISTS image_legacy_rows, image_imports;
