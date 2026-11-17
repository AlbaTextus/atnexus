import { useId } from "react"

const radians = Math.PI / 180
function project(latitude: number, longitude: number) {
  const lat = latitude * radians, lon = longitude * radians
  const x = Math.cos(lat) * Math.sin(lon)
  const y = Math.sin(lat), z = Math.cos(lat) * Math.cos(lon)
  const tiltedY = (y - z) * Math.SQRT1_2
  const depth = (y + z) * Math.SQRT1_2
  return { x: 180 + 300 * (x * Math.cos(40 * radians) + tiltedY * Math.sin(40 * radians)), y: 280 - 300 * (-x * Math.sin(40 * radians) + tiltedY * Math.cos(40 * radians)), depth }
}
function curve(points: ReturnType<typeof project>[]) {
  let drawing = false
  return points.map(point => {
    if (point.depth < 0) { drawing = false; return "" }
    const command = drawing ? "L" : "M"
    drawing = true
    return `${command}${point.x.toFixed(2)},${point.y.toFixed(2)}`
  }).join(" ")
}
const parallels = Array.from({ length: 17 }, (_, i) => curve(Array.from({ length: 121 }, (_, j) => project(-80 + i * 10, -180 + j * 3))))
const meridians = Array.from({ length: 36 }, (_, i) => curve(Array.from({ length: 91 }, (_, j) => project(-90 + j * 2, i * 10))))
// The highlighted route follows the same spherical grid, with no live location data.
const route = [
  ...Array.from({ length: 31 }, (_, i) => project(90 - i, -20)),
  ...Array.from({ length: 31 }, (_, i) => project(60, -20 + i * 2)),
  ...Array.from({ length: 16 }, (_, i) => project(60 - i, 40)),
]
const branch = Array.from({ length: 36 }, (_, i) => project(60 - i, -20))
const extraRoute = [...Array.from({ length: 31 }, (_, i) => project(75 - i, -40)), ...Array.from({ length: 41 }, (_, i) => project(45, -40 + i * 2))]
const nodes = [project(60, -20), project(45, 40), project(25, -20), project(45, -40), project(75, -40)]

export function CloudflareNetwork() {
  const id = useId()
  return <div className="cf-network" role="img" aria-label="Cloudflare DNS 与代理网络：弧形地球网格上，光点沿连接路径流动">
    <svg viewBox="0 0 420 390" className="cf-network-globe" aria-hidden="true">
      <defs><radialGradient id={id}><stop offset="0" stopColor="#e5b48d" /><stop offset="1" stopColor="#f2ddcb" /></radialGradient></defs>
      <g fill="none" stroke={`url(#${id})`} strokeWidth="0.85">{[...parallels, ...meridians].map((path, index) => <path key={index} d={path} />)}</g>
      <g fill="none" strokeLinecap="round"><path d={curve(route)} stroke="#d58a4c" strokeWidth="1.15" /><path d={curve(branch)} stroke="#e3ae80" strokeWidth="1" /><path d={curve(extraRoute)} stroke="#d99b67" strokeWidth="1.1" />
        <path className="cf-network-signal" d={curve(route)} stroke="#f38020" strokeWidth="2" pathLength="100" />
        <path className="cf-network-signal" d={curve(extraRoute)} stroke="#f38020" strokeWidth="2" pathLength="100" style={{ animationDelay: "-3s" }} />
      </g>
      {nodes.map((point, index) => <g key={index}><circle className="cf-network-halo" cx={point.x} cy={point.y} r="9" fill="#f38020" style={{ animationDelay: `${index * 0.8}s` }} /><circle cx={point.x} cy={point.y} r="3.5" fill="#dc751c" /></g>)}
    </svg>
  </div>
}
