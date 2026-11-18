CREATE TABLE "ApiUser" (
  "id" TEXT PRIMARY KEY,
  "displayName" TEXT NOT NULL DEFAULT '',
  "createdAt" TIMESTAMPTZ(3) NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE "ApiKey" (
  "id" TEXT PRIMARY KEY,
  "userId" TEXT NOT NULL REFERENCES "ApiUser"("id") ON DELETE CASCADE,
  "hash" TEXT NOT NULL UNIQUE,
  "cipher" TEXT NOT NULL,
  "prefix" TEXT NOT NULL,
  "createdAt" TIMESTAMPTZ(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "revokedAt" TIMESTAMPTZ(3)
);
CREATE INDEX "ApiKey_userId_idx" ON "ApiKey"("userId");
CREATE UNIQUE INDEX "ApiKey_one_active" ON "ApiKey"("userId") WHERE "revokedAt" IS NULL;
CREATE TABLE "ApiUsage" (
  "userId" TEXT NOT NULL REFERENCES "ApiUser"("id") ON DELETE CASCADE,
  "bucket" TIMESTAMPTZ(3) NOT NULL,
  "category" TEXT NOT NULL,
  "status" INTEGER NOT NULL,
  "count" BIGINT NOT NULL DEFAULT 0,
  "durationMs" DOUBLE PRECISION NOT NULL DEFAULT 0,
  PRIMARY KEY ("userId", "bucket", "category", "status")
);
