import type { CSSProperties, ReactNode } from "react"
import { cn } from "@/lib/utils"

// Soft, skewed gradients with film grain, after OpenAI's editorial artwork.
// Technique from venkr/gradient-gen: a base colour plus 12 radial gradients
// (colour to transparent) on full-size rects, each randomly skewed, rotated,
// scaled and offset. Grain is a separate screen-resolution noise layer
// (.grain in index.css) so it reads the same on an avatar and a hero card.

export const PALETTES = {
  mint: { base: "#22b36b", colors: ["#8be3b4", "#14a05e", "#c4f1d8", "#0c8f52", "#5cd49a"] },
  ocean: { base: "#2b78f0", colors: ["#93c6ff", "#1d6de4", "#cfe4ff", "#0b57cf", "#62a8ff"] },
  blush: { base: "#ee9fc2", colors: ["#c18ff0", "#ffc6cb", "#f59ccd", "#b48af0", "#ffd6dc"] },
  dusk: { base: "#5e7df7", colors: ["#f5a8c9", "#8ca5ff", "#c8a2f5", "#ff9fb4", "#6e8dff"] },
  peach: { base: "#ffab86", colors: ["#ff92a6", "#ffd39e", "#ff9d78", "#ffc4b6", "#f6a2c6"] },
} as const
export type Palette = keyof typeof PALETTES
const NAMES = Object.keys(PALETTES) as Palette[]

function hash(s: string) {
  let h = 2166136261
  for (let i = 0; i < s.length; i++) h = Math.imul(h ^ s.charCodeAt(i), 16777619)
  return h >>> 0
}

function rng(seed: number) {
  return () => {
    seed = (seed + 0x6d2b79f5) | 0
    let t = Math.imul(seed ^ (seed >>> 15), 1 | seed)
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296
  }
}

const cache = new Map<string, string>()

/** A CSS background-image for a seeded grain gradient. Same seed, same image. */
export function gradientImage(seed: string, palette?: Palette) {
  const key = `${seed}|${palette ?? ""}`
  const hit = cache.get(key)
  if (hit) return hit
  const r = rng(hash(seed))
  const p = PALETTES[palette ?? NAMES[hash(seed) % NAMES.length]]
  let defs = ""
  let shapes = ""
  for (let i = 0; i < 12; i++) {
    const color = p.colors[Math.floor(r() * p.colors.length)]
    const fx = (0.1 + r() * 0.3).toFixed(3)
    const sx = (0.7 + r() * 0.8).toFixed(3)
    const sy = (0.7 + r() * 0.8).toFixed(3)
    const skew = (-10 + r() * 20).toFixed(2)
    const rot = (r() * 360).toFixed(1)
    const tx = (-250 + r() * 500).toFixed(1)
    const ty = (-250 + r() * 500).toFixed(1)
    defs += `<radialGradient id="g${i}" fx="${fx}" fy="0.5"><stop offset="0" stop-color="${color}"/><stop offset="1" stop-color="${color}" stop-opacity="0"/></radialGradient>`
    shapes += `<rect width="600" height="600" fill="url(#g${i})" transform="translate(${300 + +tx} ${300 + +ty}) rotate(${rot}) skewX(${skew}) scale(${sx} ${sy}) translate(-300 -300)"/>`
  }
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 600 600" preserveAspectRatio="xMidYMid slice"><defs>${defs}</defs><rect width="600" height="600" fill="${p.base}"/>${shapes}</svg>`
  const url = `url("data:image/svg+xml,${encodeURIComponent(svg)}")`
  cache.set(key, url)
  return url
}

/** A box filled with a grain gradient. */
export function Grain({ seed, palette, className, style, children }: { seed: string; palette?: Palette; className?: string; style?: CSSProperties; children?: ReactNode }) {
  return (
    <div className={cn("grain relative overflow-hidden bg-cover bg-center", className)} style={{ backgroundImage: gradientImage(seed, palette), ...style }}>
      {children}
    </div>
  )
}
