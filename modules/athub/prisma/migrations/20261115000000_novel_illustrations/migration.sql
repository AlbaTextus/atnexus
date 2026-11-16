CREATE TABLE "Illustration" (
    "id" SERIAL NOT NULL,
    "novelId" INTEGER NOT NULL,
    "url" VARCHAR(2048) NOT NULL,
    "sha256" VARCHAR(64) NOT NULL,
    "width" INTEGER NOT NULL,
    "height" INTEGER NOT NULL,
    "createdAt" TIMESTAMPTZ(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT "Illustration_pkey" PRIMARY KEY ("id"),
    CONSTRAINT "Illustration_dimensions_check" CHECK ("width" > 0 AND "height" > 0),
    CONSTRAINT "Illustration_novelId_fkey" FOREIGN KEY ("novelId") REFERENCES "Novel"("id") ON DELETE CASCADE ON UPDATE CASCADE
);
CREATE UNIQUE INDEX "Illustration_novelId_sha256_key" ON "Illustration"("novelId", "sha256");
CREATE INDEX "Illustration_novelId_id_idx" ON "Illustration"("novelId", "id");
