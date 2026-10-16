import { useEffect, useState } from "react"
import { ShieldCheck } from "lucide-react"
import { cn } from "@/lib/utils"
import type { Autonomy as Level, AutonomyLevel } from "@/lib/api"

/**
 * How much John's AI handles before calling the real John: a three-stop
 * slider from Cautious to Full autonomy. Changes apply to the next reply.
 */
export function Autonomy({ value, levels, onChange }: { value: Level; levels: AutonomyLevel[]; onChange: (v: Level) => Promise<boolean> }) {
  // Moves at once; snaps back if saving fails.
  const [shown, setShown] = useState(value)
  useEffect(() => setShown(value), [value])
  const i = Math.max(0, levels.findIndex((l) => l.key === shown))
  const level = levels[i]
  const pick = async (next: Level) => {
    if (next === shown) return
    setShown(next)
    if (!(await onChange(next))) setShown(value)
  }

  return (
    <div>
      <div className="relative h-6 px-2.5">
        {/* Track, fill and stops sit under a transparent native range input. */}
        <div className="absolute inset-x-2.5 top-1/2 h-1.5 -translate-y-1/2 rounded-full bg-indigo-100" />
        <div className="absolute top-1/2 left-2.5 h-1.5 -translate-y-1/2 rounded-full bg-gradient-to-r from-indigo-500 to-purple-600 transition-all duration-300" style={{ width: `calc((100% - 1.25rem) * ${i / Math.max(1, levels.length - 1)})` }} />
        {levels.map((l, j) => (
          <span
            key={l.key}
            className={cn("absolute top-1/2 size-2.5 -translate-x-1/2 -translate-y-1/2 rounded-full transition", j <= i ? "bg-indigo-600" : "bg-indigo-200")}
            style={{ left: `calc(0.625rem + (100% - 1.25rem) * ${j / Math.max(1, levels.length - 1)})` }}
          />
        ))}
        <input
          type="range"
          min={0}
          max={levels.length - 1}
          step={1}
          value={i}
          onChange={(e) => pick(levels[Number(e.target.value)].key)}
          aria-label="Autonomy"
          aria-valuetext={level?.label}
          className="relative h-6 w-full cursor-pointer appearance-none bg-transparent focus-visible:outline-none [&::-moz-range-thumb]:size-5 [&::-moz-range-thumb]:rounded-full [&::-moz-range-thumb]:border-[3px] [&::-moz-range-thumb]:border-indigo-600 [&::-moz-range-thumb]:bg-white [&::-moz-range-thumb]:shadow-md [&::-webkit-slider-thumb]:size-5 [&::-webkit-slider-thumb]:appearance-none [&::-webkit-slider-thumb]:rounded-full [&::-webkit-slider-thumb]:border-[3px] [&::-webkit-slider-thumb]:border-indigo-600 [&::-webkit-slider-thumb]:bg-white [&::-webkit-slider-thumb]:shadow-md [&::-webkit-slider-thumb]:ring-4 [&::-webkit-slider-thumb]:ring-indigo-500/15 [&::-webkit-slider-thumb]:transition focus-visible:[&::-webkit-slider-thumb]:ring-indigo-500/40"
        />
      </div>

      <div className="mt-1.5 grid grid-cols-3 text-xs">
        {levels.map((l, j) => (
          <button
            key={l.key}
            onClick={() => pick(l.key)}
            className={cn("transition", j === 0 ? "text-left" : j === levels.length - 1 ? "text-right" : "text-center", l.key === shown ? "font-semibold text-indigo-700" : "text-muted-foreground hover:text-foreground")}
          >
            {l.label}
          </button>
        ))}
      </div>
      {level && (
        <>
          <p className="mt-2.5 text-[13px] leading-snug text-foreground/80">{level.summary}</p>
          <p className="mt-2 text-xs leading-relaxed text-muted-foreground" title={`Handles himself: ${level.ai_handles.join("; ")}`}>
            <span className="font-semibold text-amber-700">Calls you for: </span>
            {level.hands_off.join(" · ")}
          </p>
          <p className="mt-2 flex gap-1.5 text-[11px] font-medium text-indigo-700">
            <ShieldCheck className="mt-px size-3.5 shrink-0" /> At every level, a reply that promises income is never sent.
          </p>
        </>
      )}
    </div>
  )
}
