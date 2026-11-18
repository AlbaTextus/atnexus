-- Preserve existing hashes and credentials. Older clients have no recoverable copy.
ALTER TABLE "oauth_clients" ADD COLUMN "secret_ciphertext" TEXT;
