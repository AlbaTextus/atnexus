import { Avatar } from "radix-ui";
import { BookOpen, CloudDownload, FileArchive, Globe, HardDrive, UserRound } from "lucide-react";
import { Badge } from "./ui/badge";
import { Button } from "./ui/button";

export function DownloadSection() {
  return (
    <section className="panel resource-card" aria-label="下载资源">
      <header className="resource-header">
        <div className="resource-uploader">
          <Avatar.Root className="flex size-9 shrink-0 overflow-hidden rounded-full">
            <Avatar.Fallback className="flex size-full items-center justify-center bg-muted text-muted-foreground">
              <UserRound className="size-4" aria-hidden="true" />
            </Avatar.Fallback>
          </Avatar.Root>
          <div className="resource-uploader-info">
            <strong>资源发布者</strong>
            <span>待补充</span>
          </div>
        </div>
        <Badge variant="secondary">待补充</Badge>
      </header>

      <div className="resource-tags" aria-label="资源信息">
        <span className="resource-book-tag"><Badge variant="secondary"><BookOpen aria-hidden="true" />小说正文</Badge></span>
        <Badge variant="secondary"><FileArchive aria-hidden="true" />格式待补充</Badge>
        <Badge variant="secondary"><Globe aria-hidden="true" />语言待补充</Badge>
      </div>

      <footer className="resource-footer">
        <span className="resource-source"><HardDrive aria-hidden="true" />来源待补充</span>
        <div className="resource-actions">
          <Button type="button" size="icon" className="rounded-xl" aria-label="获取资源" title="暂无资源" disabled>
            <CloudDownload aria-hidden="true" />
          </Button>
        </div>
      </footer>
    </section>
  );
}
