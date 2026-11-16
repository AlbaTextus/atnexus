import type { IllustrationBlock, NovelIllustration } from "@/lib/athub-client"

export const ILLUSTRATION_CANVAS_WIDTH = 1000
export const ILLUSTRATION_CANVAS_MAX_HEIGHT = 5000
export const ILLUSTRATION_BLOCK_MIN_SIZE = 40
export type ResizeHandle = "n" | "ne" | "e" | "se" | "s" | "sw" | "w" | "nw"
export type BlockOperation = ResizeHandle | "move"
export type LayoutGuides = { x?: number; y?: number }
const clamp = (value: number, min: number, max: number) => Math.min(max, Math.max(min, value))
const round = (value: number) => Math.round(value * 100) / 100

export function fitIllustrationBlock(block: IllustrationBlock): IllustrationBlock {
  const width = round(clamp(block.width, ILLUSTRATION_BLOCK_MIN_SIZE, ILLUSTRATION_CANVAS_WIDTH))
  const height = round(clamp(block.height, ILLUSTRATION_BLOCK_MIN_SIZE, ILLUSTRATION_CANVAS_MAX_HEIGHT))
  return {
    ...block, width, height,
    x: round(clamp(block.x, 0, ILLUSTRATION_CANVAS_WIDTH - width)),
    y: round(clamp(block.y, 0, ILLUSTRATION_CANVAS_MAX_HEIGHT - height)),
  }
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
  let left = initial.x, right = initial.x + initial.width, top = initial.y, bottom = initial.y + initial.height
  if (operation.includes("w")) {
    left = clamp(left + dx, 0, right - ILLUSTRATION_BLOCK_MIN_SIZE)
    const snap = nearest([left], xTargets, threshold)
    if (snap && snap.target <= right - ILLUSTRATION_BLOCK_MIN_SIZE) { left = snap.target; guides.x = snap.target }
  }
  if (operation.includes("e")) {
    right = clamp(right + dx, left + ILLUSTRATION_BLOCK_MIN_SIZE, ILLUSTRATION_CANVAS_WIDTH)
    const snap = nearest([right], xTargets, threshold)
    if (snap && snap.target >= left + ILLUSTRATION_BLOCK_MIN_SIZE) { right = snap.target; guides.x = snap.target }
  }
  if (operation.includes("n")) {
    top = clamp(top + dy, 0, bottom - ILLUSTRATION_BLOCK_MIN_SIZE)
    const snap = nearest([top], yTargets, threshold)
    if (snap && snap.target <= bottom - ILLUSTRATION_BLOCK_MIN_SIZE) { top = snap.target; guides.y = snap.target }
  }
  if (operation.includes("s")) {
    bottom = clamp(bottom + dy, top + ILLUSTRATION_BLOCK_MIN_SIZE, ILLUSTRATION_CANVAS_MAX_HEIGHT)
    const snap = nearest([bottom], yTargets, threshold)
    if (snap && snap.target >= top + ILLUSTRATION_BLOCK_MIN_SIZE) { bottom = snap.target; guides.y = snap.target }
  }
  return { block: fitIllustrationBlock({ ...initial, x: left, y: top, width: right - left, height: bottom - top }), guides }
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
