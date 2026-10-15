import { useLayoutEffect, useRef, useState } from "react"
import { cn } from "@/lib/utils"

export type Series = { name: string; color: string; values: number[]; area?: boolean }

/** A smooth path through the points (Catmull-Rom as cubic Béziers), never overshooting below zero. */
function smooth(pts: [number, number][], floor: number) {
  if (pts.length < 2) return pts.length ? `M${pts[0][0]},${pts[0][1]}` : ""
  let d = `M${pts[0][0]},${pts[0][1]}`
  for (let i = 0; i < pts.length - 1; i++) {
    const [p0, p1, p2, p3] = [pts[i - 1] ?? pts[i], pts[i], pts[i + 1], pts[i + 2] ?? pts[i + 1]]
    const c1y = Math.min(floor, p1[1] + (p2[1] - p0[1]) / 6)
    const c2y = Math.min(floor, p2[1] - (p3[1] - p1[1]) / 6)
    d += ` C${p1[0] + (p2[0] - p0[0]) / 6},${c1y} ${p2[0] - (p3[0] - p1[0]) / 6},${c2y} ${p2[0]},${p2[1]}`
  }
  return d
}

/** A round axis maximum: 1, 2, 2.5 or 5 times a power of ten. */
function niceMax(v: number) {
  if (v <= 0) return 1
  const p = 10 ** Math.floor(Math.log10(v))
  return [1, 2, 2.5, 5, 10].map((m) => m * p).find((m) => m >= v) ?? v
}

function useWidth() {
  const ref = useRef<HTMLDivElement>(null)
  const [width, setWidth] = useState(0)
  useLayoutEffect(() => {
    const el = ref.current
    if (!el) return
    const ro = new ResizeObserver(() => setWidth(el.clientWidth))
    ro.observe(el)
    setWidth(el.clientWidth)
    return () => ro.disconnect()
  }, [])
  return [ref, width] as const
}

/** A line chart with soft area fills, a y grid and a hover readout. */
export function LineChart({
  labels,
  series,
  height = 240,
  format = (v) => String(Math.round(v)),
  tick = (l) => l,
  ticks = 7,
}: {
  labels: string[]
  series: Series[]
  height?: number
  format?: (v: number) => string
  tick?: (label: string, i: number) => string
  ticks?: number
}) {
  const [ref, width] = useWidth()
  const [hover, setHover] = useState<number | null>(null)
  const pad = { l: 44, r: 12, t: 12, b: 26 }
  const w = Math.max(0, width - pad.l - pad.r)
  const h = height - pad.t - pad.b
  const max = niceMax(Math.max(0, ...series.flatMap((s) => s.values)))
  const n = labels.length
  const x = (i: number) => pad.l + (n > 1 ? (i / (n - 1)) * w : w / 2)
  const y = (v: number) => pad.t + h - (v / max) * h
  const every = Math.max(1, Math.ceil(n / ticks))

  return (
    <div ref={ref} className="relative w-full select-none" style={{ height }}>
      {width > 0 && (
        <svg width={width} height={height} className="overflow-visible" onMouseLeave={() => setHover(null)}>
          <defs>
            {series.map((s, k) => (
              <linearGradient key={k} id={`fill-${k}-${s.name.replace(/\W/g, "")}`} x1="0" x2="0" y1="0" y2="1">
                <stop offset="0" stopColor={s.color} stopOpacity="0.22" />
                <stop offset="1" stopColor={s.color} stopOpacity="0" />
              </linearGradient>
            ))}
          </defs>
          {[0, 0.25, 0.5, 0.75, 1].map((f) => (
            <g key={f}>
              <line x1={pad.l} x2={pad.l + w} y1={y(max * f)} y2={y(max * f)} className="stroke-border" strokeDasharray={f === 0 ? undefined : "3 4"} />
              <text x={pad.l - 8} y={y(max * f)} dy="0.32em" textAnchor="end" className="fill-muted-foreground text-[11px] tabular-nums">
                {format(max * f)}
              </text>
            </g>
          ))}
          {labels.map((l, i) =>
            i % every === 0 || i === n - 1 ? (
              <text key={i} x={x(i)} y={height - 6} textAnchor={i === 0 ? "start" : i === n - 1 ? "end" : "middle"} className="fill-muted-foreground text-[11px]">
                {tick(l, i)}
              </text>
            ) : null,
          )}
          {series.map((s, k) => {
            const pts = s.values.map((v, i) => [x(i), y(v)] as [number, number])
            const line = smooth(pts, y(0))
            return (
              <g key={k}>
                {s.area && <path d={`${line} L${x(n - 1)},${y(0)} L${x(0)},${y(0)} Z`} fill={`url(#fill-${k}-${s.name.replace(/\W/g, "")})`} />}
                <path d={line} fill="none" stroke={s.color} strokeWidth={2.25} strokeLinecap="round" strokeLinejoin="round" />
              </g>
            )
          })}
          {hover !== null && (
            <g>
              <line x1={x(hover)} x2={x(hover)} y1={pad.t} y2={pad.t + h} className="stroke-foreground/20" />
              {series.map((s, k) => (
                <circle key={k} cx={x(hover)} cy={y(s.values[hover] ?? 0)} r={4} fill="white" stroke={s.color} strokeWidth={2} />
              ))}
            </g>
          )}
          {/* Invisible columns that pick the nearest point. */}
          {labels.map((_, i) => (
            <rect key={i} x={x(i) - w / Math.max(1, n - 1) / 2} y={pad.t} width={w / Math.max(1, n - 1)} height={h} fill="transparent" onMouseEnter={() => setHover(i)} />
          ))}
        </svg>
      )}
      {hover !== null && (
        <div
          className="pointer-events-none absolute top-2 z-10 min-w-36 rounded-lg border bg-card/95 px-3 py-2 text-xs shadow-lg backdrop-blur"
          style={x(hover) > width / 2 ? { right: width - x(hover) + 12 } : { left: x(hover) + 12 }}
        >
          <div className="mb-1 font-semibold">{tick(labels[hover], -1)}</div>
          {series.map((s) => (
            <div key={s.name} className="flex items-center gap-2">
              <span className="size-2 rounded-full" style={{ background: s.color }} />
              <span className="flex-1 text-muted-foreground">{s.name}</span>
              <span className="font-medium tabular-nums">{format(s.values[hover] ?? 0)}</span>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

/** A tiny trend line for a stat card. */
export function Sparkline({ values, color = "currentColor", className }: { values: number[]; color?: string; className?: string }) {
  const W = 120
  const H = 36
  const max = Math.max(1, ...values)
  const pts = values.map((v, i) => [(i / Math.max(1, values.length - 1)) * W, H - 3 - (v / max) * (H - 6)] as [number, number])
  const line = smooth(pts, H - 3)
  return (
    <svg viewBox={`0 0 ${W} ${H}`} preserveAspectRatio="none" className={cn("h-9 w-full overflow-visible", className)}>
      <path d={`${line} L${W},${H} L0,${H} Z`} fill={color} opacity={0.12} />
      <path d={line} fill="none" stroke={color} strokeWidth={2} vectorEffect="non-scaling-stroke" strokeLinecap="round" />
    </svg>
  )
}
