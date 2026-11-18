-- Remove retired account, personal-key and usage data; preserve content tables.
BEGIN;
DROP TABLE "ApiUsage";
DROP TABLE "ApiKey";
DROP TABLE "ApiUser";
COMMIT;
