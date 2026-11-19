"use client"

import type { IllustrationBlock, NovelIllustration } from "@/lib/athub/athub-client"
import { ILLUSTRATION_CANVAS_WIDTH as WIDTH } from "@/lib/novel/illustration-layout"
import { IllustrationImage } from "./illustration-image"

// A submitted layout as it would look on the canvas. `images` are the novel's
// illustrations by ID; a block whose illustration is not among them says so.
export function IllustrationLayoutPreview({ blocks, images, label }: {
  blocks: IllustrationBlock[]
  images: ReadonlyMap<number, Pick<NovelIllustration, "url" | "width" | "height">>
  label: string
}) {
  if (!blocks.length) return <p className="text-sm text-muted-foreground">这次提交清空了展示画布。</p>
  const height = Math.max(...blocks.map(block => block.y + block.height))
  // A layout may be five times as tall as it is wide, so it scrolls in place.
  return <div className="max-h-[32rem] overflow-y-auto rounded-lg border" role="group" aria-label={label} tabIndex={0}>
    <div className="illustration-canvas" style={{ aspectRatio: `${WIDTH} / ${height}` }}>
      {blocks.map((block, index) => {
        const image = images.get(block.imageId)
        return <div key={index} className="illustration-block"
          style={{ cursor: "default", left: `${block.x / WIDTH * 100}%`, top: `${block.y / height * 100}%`, width: `${block.width / WIDTH * 100}%`, height: `${block.height / height * 100}%` }}>
          {image ? <IllustrationImage image={image} label={`插图 #${block.imageId}`} />
            : <span className="grid size-full place-items-center border border-dashed text-xs text-muted-foreground">插图已不在作品中</span>}
        </div>
      })}
    </div>
  </div>
}
