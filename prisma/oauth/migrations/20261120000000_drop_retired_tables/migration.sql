-- Tables that nothing reads or writes any more. Their rows are deleted for
-- good: back up the database first if any of them may still be wanted.

-- Reviews and novel history are kept by the creator database, which was filled
-- from these two tables when it was split off.
DROP TABLE IF EXISTS athub_reviews;
DROP TABLE IF EXISTS novel_revisions;

-- The record of the one-time import of image keys from the SQLite image host.
-- The keys themselves are in image_api_keys and stay.
DROP TABLE IF EXISTS image_key_imports;

-- Wallets, orders, ledger entries and settlements belong to the separate
-- Credit service. These were kept only as the source of its migration.
DROP TABLE IF EXISTS wallet_settlements, wallet_entries, payment_orders, wallets;

-- The privacy policy and the terms of use are no longer served from here.
DROP TABLE IF EXISTS legal_documents;
