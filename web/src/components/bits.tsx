import { useEffect, useMemo, useRef, useState } from "react"
import { Pause, Play } from "lucide-react"
import { cn } from "@/lib/utils"
import type { Status } from "@/lib/api"

const STATUS: Record<Status, { label: string; className: string }> = {
  auto: { label: "Auto", className: "bg-wa/15 text-wa-dark ring-wa/30" },
  needs_john: { label: "Needs John", className: "bg-amber/20 text-amber-800 ring-amber/40" },
  human: { label: "John", className: "bg-primary/10 text-primary ring-primary/25" },
  new: { label: "New", className: "bg-muted text-muted-foreground ring-border" },
}

export function StatusChip({ status, className }: { status: Status; className?: string }) {
  const s = STATUS[status] ?? STATUS.new
  return (
    <span className={cn("inline-flex shrink-0 items-center rounded-full px-2 py-0.5 text-[11px] font-semibold whitespace-nowrap ring-1", s.className, className)}>
      {status === "auto" && <span className="mr-1 size-1.5 rounded-full bg-wa" />}
      {s.label}
    </span>
  )
}

const HUES = [265, 290, 220, 330, 200, 160, 20, 45]

/** A contact's WhatsApp photo, or their initials when there isn't one. */
export function Initials({ name, size = 40, className, src }: { name: string; size?: number; className?: string; src?: string }) {
  const [broken, setBroken] = useState(false)
  if (src && !broken)
    return (
      <img
        src={`/audio/${src}`}
        alt={name}
        onError={() => setBroken(true)}
        className={cn("shrink-0 rounded-full object-cover ring-1 ring-black/5", className)}
        style={{ width: size, height: size }}
      />
    )
  const letters = name
    .replace(/[^\p{L}\s]/gu, "")
    .split(/\s+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((w) => w[0]?.toUpperCase())
    .join("")
  const hue = HUES[[...name].reduce((a, c) => a + c.charCodeAt(0), 0) % HUES.length]
  return (
    <div
      className={cn("grid shrink-0 place-items-center rounded-full font-bold text-white", className)}
      style={{ width: size, height: size, fontSize: size * 0.38, background: `linear-gradient(135deg, oklch(0.65 0.17 ${hue}), oklch(0.5 0.2 ${hue + 25}))` }}
    >
      {letters || "?"}
    </div>
  )
}

/** John's avatar: his WhatsApp photo once linked, else a branded "J". */
export function JohnAvatar({ size = 40, src }: { size?: number; src?: string }) {
  const [broken, setBroken] = useState(false)
  if (src && !broken)
    return (
      <img
        src={`/audio/${src}`}
        alt="John Klensch"
        onError={() => setBroken(true)}
        className="shrink-0 rounded-full object-cover shadow-md ring-2 ring-white/40"
        style={{ width: size, height: size }}
      />
    )
  return (
    <div
      className="grid shrink-0 place-items-center rounded-full bg-gradient-to-br from-indigo-500 to-purple-600 font-extrabold text-white shadow-md shadow-indigo-500/30"
      style={{ width: size, height: size, fontSize: size * 0.42 }}
    >
      J
    </div>
  )
}

function bars(seed: number, n = 36) {
  let x = seed * 9301 + 49297
  return Array.from({ length: n }, (_, i) => {
    x = (x * 9301 + 49297) % 233280
    const env = Math.sin((i / n) * Math.PI) * 0.6 + 0.4
    return Math.max(0.18, (x / 233280) * env)
  })
}

export function fmtDur(ms = 0) {
  const s = Math.round(ms / 1000)
  return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, "0")}`
}

/** A WhatsApp-style voice-note player. */
export function VoiceNote({ src, durationMs, seed, outgoing, big }: { src?: string; durationMs?: number; seed: number; outgoing?: boolean; big?: boolean }) {
  const ref = useRef<HTMLAudioElement>(null)
  const [playing, setPlaying] = useState(false)
  const [progress, setProgress] = useState(0)
  const heights = useMemo(() => bars(seed, big ? 48 : 36), [seed, big])

  useEffect(() => {
    const a = ref.current
    if (!a) return
    const tick = () => setProgress(a.duration ? a.currentTime / a.duration : 0)
    const end = () => {
      setPlaying(false)
      setProgress(0)
    }
    a.addEventListener("timeupdate", tick)
    a.addEventListener("ended", end)
    return () => {
      a.removeEventListener("timeupdate", tick)
      a.removeEventListener("ended", end)
    }
  }, [src])

  const toggle = () => {
    const a = ref.current
    if (!a) return
    if (playing) {
      a.pause()
      setPlaying(false)
    } else {
      document.querySelectorAll("audio").forEach((o) => o !== a && o.pause())
      a.play().then(() => setPlaying(true)).catch(() => setPlaying(false))
    }
  }

  return (
    <div className={cn("flex items-center gap-3", big ? "w-full min-w-0" : "min-w-[230px]")}>
      {src && <audio ref={ref} src={`/audio/${src}`} preload="none" />}
      <button
        onClick={toggle}
        disabled={!src}
        className={cn(
          "grid shrink-0 place-items-center rounded-full transition disabled:opacity-40",
          big ? "size-14" : "size-9",
          outgoing ? "bg-wa-dark text-white hover:bg-wa-dark/90" : "bg-primary text-white hover:bg-primary/90",
        )}
        title={src ? "Play" : "Audio not rendered yet"}
      >
        {playing ? <Pause className={big ? "size-6" : "size-4"} /> : <Play className={cn(big ? "size-6" : "size-4", "translate-x-px")} />}
      </button>
      <div className={cn("flex min-w-0 flex-1 items-center gap-[2px] overflow-hidden", big ? "h-10" : "h-6")}>
        {heights.map((h, i) => (
          <span
            key={i}
            className={cn("w-[3px] min-w-px shrink rounded-full transition-colors", i / heights.length <= progress && playing ? (outgoing ? "bg-wa-dark" : "bg-primary") : "bg-foreground/25")}
            style={{ height: `${h * 100}%` }}
          />
        ))}
      </div>
      <span className={cn("shrink-0 tabular-nums text-muted-foreground", big ? "text-base" : "text-xs")}>{fmtDur(durationMs)}</span>
    </div>
  )
}

export function Equalizer({ className }: { className?: string }) {
  return (
    <span className={cn("animate-eq inline-flex h-4 items-end gap-[2px]", className)}>
      {[0, 0.15, 0.3, 0.45].map((d) => (
        <span key={d} className="w-[3px] rounded-full bg-current" style={{ height: "100%", animationDelay: `${d}s` }} />
      ))}
    </span>
  )
}

/** WhatsApp-style delivery ticks for outbound messages. */
export function Ticks({ status, voice }: { status?: string; voice?: boolean }) {
  if (!status) return null
  const blue = status === "read" || status === "played"
  const label = status === "played" ? (voice ? "Played" : "Read") : status === "read" ? "Read" : status === "delivered" ? "Delivered" : "Sent"
  return (
    <span className={cn("inline-flex items-center", blue ? "text-sky-500" : "text-muted-foreground")} title={label}>
      <svg viewBox="0 0 18 12" className="h-3 w-[18px]" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
        <path d="M1 6.5 4.5 10 11 2" />
        {status !== "sent" && <path d="M7.5 9.5 8 10l6.5-8" />}
      </svg>
    </span>
  )
}

const URL_RE = /(https?:\/\/[^\s]+)/g

/** Text with tappable links, shown like a link preview pill. */
export function Linkified({ text }: { text: string }) {
  const parts = text.split(URL_RE)
  return (
    <>
      {parts.map((p, i) =>
        /^https?:\/\//.test(p) ? (
          <a key={i} href={p} target="_blank" rel="noreferrer" className="mt-1 flex items-center gap-1.5 rounded-lg bg-white/70 px-2 py-1.5 text-[13px] font-semibold text-sky-700 ring-1 ring-black/5 hover:underline">
            <span className="grid size-5 shrink-0 place-items-center rounded bg-gradient-to-br from-indigo-500 to-purple-600 text-[10px] font-extrabold text-white">U</span>
            <span className="truncate">{p.replace(/^https?:\/\//, "")}</span>
          </a>
        ) : (
          <span key={i}>{p}</span>
        ),
      )}
    </>
  )
}
