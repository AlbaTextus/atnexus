CREATE TABLE "IllustrationLayout" (
    "novelId" INTEGER PRIMARY KEY REFERENCES "Novel"("id") ON DELETE CASCADE,
    blocks JSONB NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(blocks) = 'array'),
    revision INTEGER NOT NULL DEFAULT 1 CHECK (revision > 0)
);
