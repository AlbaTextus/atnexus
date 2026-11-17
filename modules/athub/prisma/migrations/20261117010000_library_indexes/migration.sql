-- The library's default order.
CREATE INDEX "Novel_updatedAt_id_idx" ON "Novel" ("updatedAt" DESC, "id" DESC);
-- Author matching by trimmed name: cross-platform browsing and upload lookup.
CREATE INDEX "Author_trimmed_name_idx" ON "Author" (btrim("name"));
CREATE INDEX "Author_platform_trimmed_name_idx" ON "Author" ("platformId", btrim("name"));
