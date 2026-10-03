"use client"

import { useId, useRef, useState, useSyncExternalStore, type KeyboardEvent, type PointerEvent } from "react"
import { Database, Fingerprint, Globe2, Images, KeyRound, MessageSquare, Server, ShieldCheck, UserRound, WalletCards, type LucideIcon } from "lucide-react"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { NetworkDiagramConnections, NetworkDiagramNode, networkDiagramSizing as sizes, networkDiagramCanvasStyle } from "@albatextus/ui"

type Position = { x: number; y: number }
type MovableNode = "apps" | "data"
const initialPositions: Record<MovableNode, Position> = {
  apps: { x: 830, y: 90 },
  data: { x: 644, y: 276 },
}
const mobilePositions: Record<MovableNode, Position> = {
  apps: { x: 214, y: 392 },
  data: { x: 54, y: 438 },
}
function subscribeLayout(onChange: () => void) {
  const query = window.matchMedia("(max-width: 760px)")
  query.addEventListener("change", onChange)
  return () => query.removeEventListener("change", onChange)
}
const getMobileLayout = () => window.matchMedia("(max-width: 760px)").matches
const getServerLayout = () => false
const nodes = {
  apps: { width: sizes.group, height: sizes.group, label: "社区应用" },
  data: { width: sizes.node, height: sizes.node, label: "账户数据" },
}

// Selection frames and SVG-coordinate dragging follow ATHub's NetworkIntro.
function SelectionFrame({ width, height }: { width: number; height: number }) {
  return <g className="architecture-frame" aria-hidden="true">
    <rect width={width} height={height} />
    {[[0, 0], [width, 0], [0, height], [width, height]].map(([x, y]) => (
      <rect key={`${x}-${y}`} x={x - sizes.handle / 2} y={y - sizes.handle / 2} width={sizes.handle} height={sizes.handle} className="architecture-handle" />
    ))}
  </g>
}

function IconTile({ icon: Icon, x = sizes.inset, y = sizes.inset, size = sizes.tile, iconSize = sizes.icon }: { icon: LucideIcon; x?: number; y?: number; size?: number; iconSize?: number }) {
  return <g aria-hidden="true">
    <rect x={x} y={y} width={size} height={size} rx={sizes.radius} className="architecture-tile" />
    <Icon x={x + (size - iconSize) / 2} y={y + (size - iconSize) / 2} width={iconSize} height={iconSize} strokeWidth={1.5} />
  </g>
}

function svgPosition(svg: SVGSVGElement, clientX: number, clientY: number) {
  const matrix = svg.getScreenCTM()
  return matrix ? new DOMPoint(clientX, clientY).matrixTransform(matrix.inverse()) : null
}

export function ServiceArchitecture() {
  const id = useId()
  const mobile = useSyncExternalStore(subscribeLayout, getMobileLayout, getServerLayout)
  const layout = mobile ? "mobile" : "desktop"
  const [layouts, setLayouts] = useState({ desktop: initialPositions, mobile: mobilePositions })
  const positions = layouts[layout]
  const [dragging, setDragging] = useState<MovableNode | null>(null)
  const drag = useRef<{ node: MovableNode; pointerId: number; offset: Position } | null>(null)

  function moveNode(node: MovableNode, position: Position) {
    const { width, height } = nodes[node]
    // Keep the frame handles inside the viewBox.
    const next = {
      x: Math.max(36, Math.min(position.x, (mobile ? 384 : 1084) - width)),
      y: Math.max(36, Math.min(position.y, (mobile ? 552 : 352) - height)),
    }
    setLayouts(current => ({ ...current, [layout]: { ...current[layout], [node]: next } }))
  }

  function startDrag(event: PointerEvent<SVGGElement>, node: MovableNode) {
    if (event.button !== 0 || !event.isPrimary || drag.current) return
    const svg = event.currentTarget.ownerSVGElement
    const point = svg && svgPosition(svg, event.clientX, event.clientY)
    if (!point) return
    event.preventDefault()
    event.currentTarget.focus({ preventScroll: true })
    event.currentTarget.setPointerCapture(event.pointerId)
    drag.current = { node, pointerId: event.pointerId, offset: { x: point.x - positions[node].x, y: point.y - positions[node].y } }
    setDragging(node)
  }

  function moveDrag(event: PointerEvent<SVGGElement>) {
    const active = drag.current
    const svg = event.currentTarget.ownerSVGElement
    if (!active || active.pointerId !== event.pointerId || !svg) return
    const point = svgPosition(svg, event.clientX, event.clientY)
    if (point) moveNode(active.node, { x: point.x - active.offset.x, y: point.y - active.offset.y })
  }

  function endDrag(event: PointerEvent<SVGGElement>) {
    if (drag.current?.pointerId !== event.pointerId) return
    drag.current = null
    setDragging(null)
    if (event.currentTarget.hasPointerCapture(event.pointerId)) event.currentTarget.releasePointerCapture(event.pointerId)
  }

  function moveWithKeyboard(event: KeyboardEvent<SVGGElement>, node: MovableNode) {
    if (event.key === "Escape" && !drag.current) {
      event.preventDefault()
      setLayouts({ desktop: initialPositions, mobile: mobilePositions })
      return
    }
    const directions: Record<string, Position> = {
      ArrowLeft: { x: -1, y: 0 }, ArrowRight: { x: 1, y: 0 },
      ArrowUp: { x: 0, y: -1 }, ArrowDown: { x: 0, y: 1 },
    }
    const direction = directions[event.key]
    if (!direction || drag.current) return
    event.preventDefault()
    const step = event.shiftKey ? 24 : 8
    moveNode(node, { x: positions[node].x + direction.x * step, y: positions[node].y + direction.y * step })
  }

  return <Card className="architecture-card">
    <CardHeader className="sr-only">
      <CardTitle><h2 id={`${id}-title`}>服务架构</h2></CardTitle>
    </CardHeader>
    <CardContent className="px-0">
      <div className="architecture-viewport" data-network-diagram-viewport>
        <svg className="architecture-canvas" data-network-diagram-canvas style={networkDiagramCanvasStyle(mobile ? 420 : 1120)} viewBox={mobile ? "0 0 420 610" : "0 0 1120 410"} fill="none" role="group" aria-labelledby={`${id}-title`} aria-describedby={`${id}-description ${id}-help`}>
          <desc id={`${id}-description`}>用户通过密码、通行密钥或第三方身份登录 OAuth 账户中心。账户中心管理身份与会话，使用 PostgreSQL 保存账户数据，并通过 OAuth 2.0 向论坛、积分、图床和 ATHub 等应用提供授权。</desc>
          <desc id={`${id}-help`}>彩色节点可拖动。聚焦节点后可使用方向键移动，按住 Shift 加快，按 Escape 重置布局。</desc>
          <NetworkDiagramConnections>
            <path d={mobile ? "M95 102H212" : "M133 168C188 168 202 143 256 143"} />
            <path d={mobile ? "M266 132.5C266 166 285 176 285 213" : "M364 143C435 143 447 171 525 171"} />
            <path d={mobile ? "M107 238C176 238 196 243 255 243" : "M327 289C428 289 441 186 525 186"} />
            <path d={mobile
              ? `M315 243C406 243 ${positions.apps.x + nodes.apps.width + 38} ${positions.apps.y + nodes.apps.height / 2} ${positions.apps.x + nodes.apps.width} ${positions.apps.y + nodes.apps.height / 2}`
              : `M585 171C710 171 ${positions.apps.x - 100} ${positions.apps.y + nodes.apps.height / 2} ${positions.apps.x} ${positions.apps.y + nodes.apps.height / 2}`} />
            <path d={mobile
              ? `M285 273C285 339 ${positions.data.x + nodes.data.width / 2} ${positions.data.y - 64} ${positions.data.x + nodes.data.width / 2} ${positions.data.y}`
              : `M555 201C555 249 ${positions.data.x + nodes.data.width / 2} ${positions.data.y - 48} ${positions.data.x + nodes.data.width / 2} ${positions.data.y}`} />
          </NetworkDiagramConnections>

          <NetworkDiagramNode muted width={sizes.smallNode} height={sizes.smallNode} className="architecture-muted" transform={mobile ? "translate(53 81)" : "translate(91 147)"}>
            <SelectionFrame width={sizes.smallNode} height={sizes.smallNode} />
            <UserRound x={11} y={11} width={sizes.icon} height={sizes.icon} strokeWidth={1.6} aria-hidden="true" />
          </NetworkDiagramNode>

          <NetworkDiagramNode muted width={sizes.group} height={61} className="architecture-muted" transform={mobile ? "translate(212 71.5)" : "translate(256 112.5)"}>
            <SelectionFrame width={sizes.group} height={61} />
            <IconTile icon={KeyRound} y={10} size={sizes.mutedTile} iconSize={sizes.mutedIcon} />
            <IconTile icon={Fingerprint} x={57} y={10} size={sizes.mutedTile} iconSize={sizes.mutedIcon} />
          </NetworkDiagramNode>

          <NetworkDiagramNode muted width={sizes.compactNode} height={sizes.compactNode} className="architecture-muted" transform={mobile ? "translate(69 219)" : "translate(289 270)"}>
            <SelectionFrame width={sizes.compactNode} height={sizes.compactNode} />
            <Globe2 x={sizes.inset} y={sizes.inset} width={sizes.icon} height={sizes.icon} strokeWidth={1.6} aria-hidden="true" />
          </NetworkDiagramNode>

          <NetworkDiagramNode width={sizes.node} height={sizes.node} className="architecture-core" transform={mobile ? "translate(255 213)" : "translate(525 141)"}>
            <SelectionFrame width={sizes.node} height={sizes.node} />
            <IconTile icon={ShieldCheck} />
          </NetworkDiagramNode>

          {(["apps", "data"] as const).map(node => (
            <NetworkDiagramNode key={node} width={nodes[node].width} height={nodes[node].height} className="architecture-movable" data-node={node}
              transform={`translate(${positions[node].x} ${positions[node].y})`}
              data-dragging={dragging === node || undefined}
              role="group" aria-roledescription="可拖动节点" aria-label={nodes[node].label}
              aria-describedby={`${id}-help`} tabIndex={0}
              onPointerDown={event => startDrag(event, node)} onPointerMove={moveDrag}
              onPointerUp={endDrag} onPointerCancel={endDrag} onLostPointerCapture={endDrag}
              onKeyDown={event => moveWithKeyboard(event, node)}>
              <SelectionFrame width={nodes[node].width} height={nodes[node].height} />
              {node === "apps" ? <>
                {[
                  { icon: MessageSquare, label: "论坛", column: 0, row: 0 },
                  { icon: WalletCards, label: "积分", column: 1, row: 0 },
                  { icon: Images, label: "图床", column: 0, row: 1 },
                  { icon: Server, label: "ATHub", column: 1, row: 1 },
                ].map(app => <g key={app.label}>
                  <IconTile icon={app.icon} x={sizes.inset + app.column * (sizes.tile + sizes.gap)} y={sizes.inset + app.row * (sizes.tile + sizes.gap)} />
                </g>)}
              </> : <>
                <IconTile icon={Database} />
              </>}
            </NetworkDiagramNode>
          ))}
        </svg>
      </div>
    </CardContent>
  </Card>
}
