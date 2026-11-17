-- The default library list orders by recency; without this every page sorts the whole table.
CREATE INDEX "Novel_updatedAt_id_idx" ON "Novel"("updatedAt" DESC, "id" DESC);
