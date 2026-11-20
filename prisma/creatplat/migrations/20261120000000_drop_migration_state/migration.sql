-- The record of the one-time import of reviews and history from the account
-- database. The import is finished, its source tables are dropped, and nothing
-- compares against this record any more.
DROP TABLE IF EXISTS creator_migration_state;
