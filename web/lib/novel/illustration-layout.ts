import type { IllustrationBlock, NovelIllustration } from "@/lib/athub/athub-client"

export const ILLUSTRATION_CANVAS_WIDTH = 1000
export const ILLUSTRATION_CANVAS_MAX_HEIGHT = 5000
export const ILLUSTRATION_BLOCK_MIN_SIZE = 40
export type ResizeHandle = "n" | "ne" | "e" | "se" | "s" | "sw" | "w" | "nw"
export type BlockOperation = ResizeHandle | "move"
export type LayoutGuides = { x?: number; y?: number }
const clamp = (value: number, min: number, max: number) => Math.min(max, Math.max(min, value))

// A submission is shown from what it carries, which is not known to have the
// shape of an illustration or a layout until it has been checked.
export const isPositiveNumber = (value: unknown): value is number => typeof value === "number" && Number.isFinite(value) && value > 0
export function isIllustrationBlock(value: unknown): value is IllustrationBlock {
  if (typeof value !== "object" || value === null) return false
  const { imageId, x, y, width, height } = value as Record<string, unknown>
  return isPositiveNumber(imageId) && isPositiveNumber(width) && isPositiveNumber(height) && [x, y].every(edge => edge === 0 || isPositiveNumber(edge))
}

export function fitIllustrationBlock(block: IllustrationBlock): IllustrationBlock {
  const ratio = block.width / block.height
  const maxWidth = Math.min(ILLUSTRATION_CANVAS_WIDTH, ILLUSTRATION_CANVAS_MAX_HEIGHT * ratio)
  const minWidth = ILLUSTRATION_BLOCK_MIN_SIZE * Math.min(1, ratio)
  const width = clamp(block.width, minWidth, maxWidth)
  const height = width / ratio
  return {
    ...block, width, height,
    x: clamp(block.x, 0, ILLUSTRATION_CANVAS_WIDTH - width),
    y: clamp(block.y, 0, ILLUSTRATION_CANVAS_MAX_HEIGHT - height),
  }
}

// Old layouts could contain a centered image inside a differently shaped box.
// Keep the image's visible position while making its box fit the actual image.
export function normalizeIllustrationBlock(block: IllustrationBlock, image: NovelIllustration): IllustrationBlock {
  if (!(image.width > 0 && image.height > 0)) return block
  const scale = Math.min(block.width / image.width, block.height / image.height)
  const width = image.width * scale, height = image.height * scale
  if (Math.abs(width - block.width) < 1e-7 && Math.abs(height - block.height) < 1e-7) return block
  return fitIllustrationBlock({ ...block, width, height,
    x: block.x + (block.width - width) / 2, y: block.y + (block.height - height) / 2 })
}

function nearest(edges: number[], targets: number[], threshold: number) {
  if (threshold <= 0) return undefined
  let result: { delta: number; target: number } | undefined
  for (const edge of edges) for (const target of targets) {
    const delta = target - edge
    if (Math.abs(delta) <= threshold && (!result || Math.abs(delta) < Math.abs(result.delta))) result = { delta, target }
  }
  return result
}

export function transformIllustrationBlock(
  initial: IllustrationBlock, operation: BlockOperation, dx: number, dy: number,
  neighbors: IllustrationBlock[], threshold: number,
): { block: IllustrationBlock; guides: LayoutGuides } {
  const xTargets = [0, ILLUSTRATION_CANVAS_WIDTH, ...neighbors.flatMap(block => [block.x, block.x + block.width])]
  const yTargets = [0, ...neighbors.flatMap(block => [block.y, block.y + block.height])]
  const guides: LayoutGuides = {}
  if (operation === "move") {
    let block = fitIllustrationBlock({ ...initial, x: initial.x + dx, y: initial.y + dy })
    const sx = nearest([block.x, block.x + block.width], xTargets, threshold)
    const sy = nearest([block.y, block.y + block.height], yTargets, threshold)
    if (sx) { block.x += sx.delta; guides.x = sx.target }
    if (sy) { block.y += sy.delta; guides.y = sy.target }
    block = fitIllustrationBlock(block)
    return { block, guides }
  }
  const west = operation.includes("w"), east = operation.includes("e")
  const north = operation.includes("n"), south = operation.includes("s")
  const horizontal = west || east, vertical = north || south
  const ratio = initial.width / initial.height
  // Corners keep the opposite corner fixed; side handles keep the opposite
  // side's midpoint fixed where the canvas allows it. Every handle scales
  // both dimensions together.
  const anchorX = west ? initial.x + initial.width : east ? initial.x : initial.x + initial.width / 2
  const anchorY = north ? initial.y + initial.height : south ? initial.y : initial.y + initial.height / 2
  const maxWidth = Math.min(
    west ? anchorX : east ? ILLUSTRATION_CANVAS_WIDTH - anchorX : ILLUSTRATION_CANVAS_WIDTH,
    (north ? anchorY : south ? ILLUSTRATION_CANVAS_MAX_HEIGHT - anchorY : ILLUSTRATION_CANVAS_MAX_HEIGHT) * ratio,
  )
  const minWidth = Math.min(maxWidth, ILLUSTRATION_BLOCK_MIN_SIZE * Math.min(1, ratio))
  const deltaX = west ? -dx : dx, deltaY = (north ? -dy : dy) * ratio
  let width = clamp(initial.width + (horizontal && vertical
    ? Math.abs(deltaX) >= Math.abs(deltaY) ? deltaX : deltaY
    : horizontal ? deltaX : deltaY), minWidth, maxWidth)
  const sx = horizontal ? nearest([anchorX + (west ? -width : width)], xTargets, threshold) : undefined
  const sy = vertical ? nearest([anchorY + (north ? -width / ratio : width / ratio)], yTargets, threshold) : undefined
  // Only one snap can drive an aspect-preserving resize. Show a guide only
  // when that target can be reached within both canvas dimensions.
  const candidates = [
    ...(sx ? [{ width: width + (west ? -sx.delta : sx.delta), distance: Math.abs(sx.delta), x: sx.target }] : []),
    ...(sy ? [{ width: width + (north ? -sy.delta : sy.delta) * ratio, distance: Math.abs(sy.delta), y: sy.target }] : []),
  ].filter(snap => snap.width >= minWidth && snap.width <= maxWidth).sort((a, b) => a.distance - b.distance)
  const snap = candidates[0]
  if (snap) { width = snap.width; Object.assign(guides, "x" in snap ? { x: snap.x } : { y: snap.y }) }
  const height = width / ratio
  return { block: { ...initial, width, height,
    x: west ? anchorX - width : east ? anchorX : clamp(anchorX - width / 2, 0, ILLUSTRATION_CANVAS_WIDTH - width),
    y: north ? anchorY - height : south ? anchorY : clamp(anchorY - height / 2, 0, ILLUSTRATION_CANVAS_MAX_HEIGHT - height) }, guides }
}

export function createIllustrationBlock(image: NovelIllustration, blocks: IllustrationBlock[], point?: { x: number; y: number }): IllustrationBlock {
  const ratio = image.width / image.height
  const width = Math.min(300, 350 * ratio)
  const height = width / ratio
  if (point) return fitIllustrationBlock({ imageId: image.id, x: point.x - width / 2, y: point.y - height / 2, width, height })
  const xs = [0, ...blocks.map(block => block.x + block.width)].sort((a, b) => a - b)
  const ys = [0, ...blocks.map(block => block.y + block.height)].sort((a, b) => a - b)
  for (const y of ys) for (const x of xs) {
    if (x + width > ILLUSTRATION_CANVAS_WIDTH || y + height > ILLUSTRATION_CANVAS_MAX_HEIGHT) continue
    if (!blocks.some(block => x < block.x + block.width && x + width > block.x && y < block.y + block.height && y + height > block.y)) {
      return fitIllustrationBlock({ imageId: image.id, x, y, width, height })
    }
  }
  return fitIllustrationBlock({ imageId: image.id, x: 0, y: 0, width, height })
}
