-- Existing chapters remain ungrouped; their IDs and global order are unchanged.
ALTER TABLE "Chapter" ADD COLUMN "volumeId" INTEGER;

CREATE TABLE "Volume" (
    "id" SERIAL NOT NULL,
    "novelId" INTEGER NOT NULL,
    "order" INTEGER NOT NULL,
    "title" TEXT NOT NULL,
    "createdAt" TIMESTAMPTZ(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updatedAt" TIMESTAMPTZ(3) NOT NULL,

    CONSTRAINT "Volume_pkey" PRIMARY KEY ("id")
);

CREATE UNIQUE INDEX "Volume_novelId_order_key" ON "Volume"("novelId", "order");
CREATE UNIQUE INDEX "Volume_id_novelId_key" ON "Volume"("id", "novelId");
CREATE INDEX "Chapter_volumeId_novelId_idx" ON "Chapter"("volumeId", "novelId");

ALTER TABLE "Volume" ADD CONSTRAINT "Volume_novelId_fkey"
    FOREIGN KEY ("novelId") REFERENCES "Novel"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- The composite key prevents attaching a chapter to another novel's volume.
-- NO ACTION blocks deleting a nonempty volume but permits whole-novel cascades.
ALTER TABLE "Chapter" ADD CONSTRAINT "Chapter_volumeId_novelId_fkey"
    FOREIGN KEY ("volumeId", "novelId") REFERENCES "Volume"("id", "novelId")
    ON DELETE NO ACTION ON UPDATE CASCADE;
