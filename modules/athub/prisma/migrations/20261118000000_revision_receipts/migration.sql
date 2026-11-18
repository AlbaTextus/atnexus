-- One row per recorded content change, written by the content transaction
-- itself. A revision left pending in the creator database is settled by
-- checking whether its batch has a receipt here.
CREATE TABLE "RevisionReceipt" (
    "id" UUID PRIMARY KEY,
    "createdAt" TIMESTAMPTZ(3) NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX "RevisionReceipt_createdAt_idx" ON "RevisionReceipt"("createdAt");
