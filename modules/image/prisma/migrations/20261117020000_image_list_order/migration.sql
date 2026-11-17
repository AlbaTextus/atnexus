-- The list orders by the numeric timestamp; the previous text index could never
-- satisfy that ordering. This expression is identical to the query's ORDER BY.
DROP INDEX image_files_time;
CREATE INDEX image_files_time ON image_files ((CASE WHEN metadata->>'TimeStamp' ~ '^[0-9]+$' THEN (metadata->>'TimeStamp')::numeric ELSE 0 END) DESC,id);
