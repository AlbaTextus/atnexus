"use client"

import { useLayoutEffect, useRef, useState } from "react"

type Link = { target: string; path: string; start: [number, number]; end: [number, number] }

export function ProfileImageLinks() {
  const svg = useRef<SVGSVGElement>(null)
  const [drawing, setDrawing] = useState<{ width: number; height: number; links: Link[] }>()

  useLayoutEffect(() => {
    const layout = svg.current?.parentElement
    if (!layout) return
    const measure = () => {
      const bounds = layout.getBoundingClientRect()
      if (!bounds.width || !bounds.height) return
      const links: Link[] = []
      for (const target of ["avatar", "background"]) {
        const source = layout.querySelector(`[data-profile-source="${target}"] .profile-image-upload`)
        const preview = layout.querySelector(`[data-profile-preview="${target}"]`)
        if (!source || !preview) continue
        const image = source.getBoundingClientRect()
        const crop = preview.getBoundingClientRect()
        const left = target === "avatar"
        const start: [number, number] = [(left ? crop.left : crop.right) - bounds.left, crop.top + crop.height / 2 - bounds.top]
        const end: [number, number] = [(left ? image.right : image.left) - bounds.left, image.top + image.height / 2 - bounds.top]
        const middle = (start[0] + end[0]) / 2
        links.push({ target, start, end, path: `M ${start[0]} ${start[1]} C ${middle} ${start[1]}, ${middle} ${end[1]}, ${end[0]} ${end[1]}` })
      }
      setDrawing({ width: bounds.width, height: bounds.height, links })
    }
    const observer = new ResizeObserver(measure)
    observer.observe(layout)
    layout.querySelectorAll("[data-profile-source], [data-profile-preview]").forEach(element => observer.observe(element))
    window.addEventListener("resize", measure)
    measure()
    return () => { observer.disconnect(); window.removeEventListener("resize", measure) }
  }, [])

  return <svg ref={svg} className="profile-image-links" viewBox={drawing ? `0 0 ${drawing.width} ${drawing.height}` : undefined} aria-hidden="true">
    {drawing?.links.map(link => <g key={link.target} data-profile-link={link.target}>
      <path d={link.path} />
      <circle cx={link.start[0]} cy={link.start[1]} r="3" />
      <circle cx={link.end[0]} cy={link.end[1]} r="3" />
    </g>)}
  </svg>
}
