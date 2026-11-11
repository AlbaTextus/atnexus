import { ImageIcon } from "lucide-react";
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia } from "./ui/empty";

export function GallerySection() {
  return (
    <section className="panel mt-4 overflow-hidden p-5 sm:p-6" aria-label="插图">
      <h2 className="content-section-heading illustrations-heading m-0 text-lg font-bold tracking-tight text-foreground sm:text-xl">
        插图
      </h2>
      <Empty>
        <EmptyHeader>
          <EmptyMedia variant="icon">
            <ImageIcon aria-hidden="true" />
          </EmptyMedia>
          <EmptyDescription>暂无插图</EmptyDescription>
        </EmptyHeader>
      </Empty>
    </section>
  );
}
