CREATE TABLE "Tag" (
    "id" SERIAL NOT NULL,
    "name" TEXT NOT NULL,
    CONSTRAINT "Tag_pkey" PRIMARY KEY ("id")
);

CREATE UNIQUE INDEX "Tag_name_key" ON "Tag"("name");

CREATE TABLE "NovelTag" (
    "novelId" INTEGER NOT NULL,
    "tagId" INTEGER NOT NULL,
    CONSTRAINT "NovelTag_pkey" PRIMARY KEY ("novelId", "tagId")
);

CREATE INDEX "NovelTag_tagId_idx" ON "NovelTag"("tagId");

ALTER TABLE "NovelTag" ADD CONSTRAINT "NovelTag_novelId_fkey"
    FOREIGN KEY ("novelId") REFERENCES "Novel"("id") ON DELETE CASCADE ON UPDATE CASCADE;
ALTER TABLE "NovelTag" ADD CONSTRAINT "NovelTag_tagId_fkey"
    FOREIGN KEY ("tagId") REFERENCES "Tag"("id") ON DELETE RESTRICT ON UPDATE CASCADE;
