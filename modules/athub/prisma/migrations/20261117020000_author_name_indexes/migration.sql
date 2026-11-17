-- Author lookups compare trimmed names: imports resolve an author within one
-- platform, and author browsing matches the same name across platforms.
CREATE INDEX "Author_platformId_trimmedName_idx" ON "Author"("platformId", btrim("name"));
CREATE INDEX "Author_trimmedName_idx" ON "Author"(btrim("name"));
