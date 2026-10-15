import { useEffect, useMemo, useRef, useState, useSyncExternalStore } from "react"
import { Pause, Play } from "lucide-react"
import { cn } from "@/lib/utils"
import { Grain } from "./Grain"
import logo from "@/assets/logo.webp"
import type { Status } from "@/lib/api"

const STATUS: Record<Status, { label: string; dot: string }> = {
  auto: { label: "Auto", dot: "bg-wa" },
  needs_john: { label: "Needs John", dot: "bg-amber" },
  human: { label: "John", dot: "bg-blue" },
  new: { label: "New", dot: "bg-foreground/25" },
}

/** A chat's state as a coloured dot and a word. `bare` drops the outline for dense lists. */
export function StatusChip({ status, bare, className }: { status: Status; bare?: boolean; className?: string }) {
  const s = STATUS[status] ?? STATUS.new
  return (
    <span
      className={cn(
        "inline-flex shrink-0 items-center gap-1.5 text-[11px] font-medium whitespace-nowrap text-muted-foreground",
        !bare && "rounded-full border px-2 py-0.5 text-foreground/75",
        status === "needs_john" && "text-amber-700",
        className,
      )}
    >
      <span className={cn("size-1.5 rounded-full", s.dot)} />
      {s.label}
    </span>
  )
}

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
  return (
    <Grain seed={name} className={cn("grid shrink-0 place-items-center rounded-full font-semibold text-white", className)} style={{ width: size, height: size, fontSize: size * 0.38 }}>
      <span className="drop-shadow-[0_1px_1px_rgb(0_0_0/0.15)]">{letters || "?"}</span>
    </Grain>
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
        className="shrink-0 rounded-full object-cover ring-1 ring-black/5"
        style={{ width: size, height: size }}
      />
    )
  return (
    <Grain seed="John Klensch" palette="dusk" className="grid shrink-0 place-items-center rounded-full font-semibold text-white" style={{ width: size, height: size, fontSize: size * 0.42 }}>
      <span>J</span>
    </Grain>
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

// Playback speed is shared by every voice note and remembered, like WhatsApp.
const SPEEDS = [1, 1.25, 1.5, 2]
let speed = 1
try {
  const saved = Number(localStorage.getItem("ureach.speed"))
  if (SPEEDS.includes(saved)) speed = saved
} catch {
  /* storage unavailable: start at 1x */
}
const speedListeners = new Set<() => void>()
const subscribeSpeed = (l: () => void) => {
  speedListeners.add(l)
  return () => {
    speedListeners.delete(l)
  }
}
function nextSpeed() {
  speed = SPEEDS[(SPEEDS.indexOf(speed) + 1) % SPEEDS.length]
  try {
    localStorage.setItem("ureach.speed", String(speed))
  } catch {
    /* ignore */
  }
  speedListeners.forEach((l) => l())
}

/**
 * A WhatsApp-style voice-note player. Click or drag along the waveform to
 * seek; the pill on the right changes the speed.
 */
export function VoiceNote({ src, durationMs, seed, outgoing, big }: { src?: string; durationMs?: number; seed: number; outgoing?: boolean; big?: boolean }) {
  const ref = useRef<HTMLAudioElement>(null)
  const wave = useRef<HTMLDivElement>(null)
  const [playing, setPlaying] = useState(false)
  const [progress, setProgress] = useState(0)
  const rate = useSyncExternalStore(subscribeSpeed, () => speed)
  const heights = useMemo(() => bars(seed, big ? 48 : 36), [seed, big])

  // Before the file has loaded (preload is "none") the stored duration stands in.
  const length = (a: HTMLAudioElement) => (Number.isFinite(a.duration) && a.duration > 0 ? a.duration : (durationMs ?? 0) / 1000)

  useEffect(() => {
    const a = ref.current
    if (!a) return
    const tick = () => {
      const d = length(a)
      if (d) setProgress(Math.min(1, a.currentTime / d))
    }
    const end = () => {
      setPlaying(false)
      setProgress(0)
    }
    const play = () => setPlaying(true)
    const pause = () => setPlaying(false) // also when another note starts
    a.addEventListener("timeupdate", tick)
    a.addEventListener("ended", end)
    a.addEventListener("play", play)
    a.addEventListener("pause", pause)
    return () => {
      a.removeEventListener("timeupdate", tick)
      a.removeEventListener("ended", end)
      a.removeEventListener("play", play)
      a.removeEventListener("pause", pause)
    }
  }, [src, durationMs])

  useEffect(() => {
    const a = ref.current
    if (!a) return
    a.defaultPlaybackRate = rate
    a.playbackRate = rate
  }, [rate, src])

  const toggle = () => {
    const a = ref.current
    if (!a) return
    if (!a.paused) {
      a.pause()
    } else {
      document.querySelectorAll("audio").forEach((o) => o !== a && o.pause())
      a.playbackRate = rate
      a.play().catch(() => setPlaying(false))
    }
  }

  const seek = (clientX: number) => {
    const a = ref.current
    const el = wave.current
    if (!a || !el) return
    const d = length(a)
    if (!d) return
    const r = el.getBoundingClientRect()
    const f = Math.min(0.999, Math.max(0, (clientX - r.left) / r.width))
    a.currentTime = f * d
    setProgress(f)
  }

  const played = outgoing ? "bg-wa-dark" : "bg-primary"
  return (
    <div className={cn("flex items-center gap-3", big ? "w-full min-w-0" : "min-w-[260px]")}>
      {src && <audio ref={ref} src={`/audio/${src}`} preload="none" />}
      <button
        onClick={toggle}
        disabled={!src}
        className={cn(
          "grid shrink-0 place-items-center rounded-full transition disabled:opacity-40",
          big ? "size-14" : "size-9",
          outgoing ? "bg-wa-dark text-white hover:bg-wa-dark/90" : "bg-primary text-white hover:bg-primary/90",
        )}
        title={src ? (playing ? "Pause" : "Play") : "Audio not rendered yet"}
      >
        {playing ? <Pause className={big ? "size-6" : "size-4"} /> : <Play className={cn(big ? "size-6" : "size-4", "translate-x-px")} />}
      </button>
      <div
        ref={wave}
        onPointerDown={(e) => {
          if (!src) return
          e.currentTarget.setPointerCapture(e.pointerId)
          seek(e.clientX)
        }}
        onPointerMove={(e) => e.currentTarget.hasPointerCapture(e.pointerId) && seek(e.clientX)}
        className={cn("relative min-w-0 touch-none select-none", big ? "h-10" : "h-6", src && "cursor-pointer")}
        title={src ? "Drag to skip" : undefined}
      >
        <div className="flex h-full items-center gap-[2px] overflow-hidden">
          {heights.map((h, i) => (
            <span key={i} className={cn("w-[3px] min-w-px shrink rounded-full", (i + 0.5) / heights.length <= progress ? played : "bg-foreground/25")} style={{ height: `${h * 100}%` }} />
          ))}
        </div>
        {src && <span className={cn("pointer-events-none absolute top-1/2 -translate-x-1/2 -translate-y-1/2 rounded-full", played, big ? "size-4" : "size-3")} style={{ left: `${progress * 100}%` }} />}
      </div>
      <span className={cn("ml-auto shrink-0 text-muted-foreground tabular-nums", big ? "text-base" : "text-xs")}>{fmtDur(progress > 0 ? progress * (durationMs ?? 0) : durationMs)}</span>
      {src && (
        <button
          onClick={nextSpeed}
          title="Playback speed"
          className={cn(
            "shrink-0 rounded-full text-center font-semibold tabular-nums transition",
            big ? "min-w-14 px-2 py-1 text-sm" : "min-w-10 px-1.5 py-0.5 text-[11px]",
            "bg-black/[0.06] text-foreground/70 hover:bg-black/10",
          )}
        >
          {rate}×
        </button>
      )}
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
          <a
            key={i}
            href={p}
            target="_blank"
            rel="noreferrer"
            className="mt-1 flex items-center gap-1.5 rounded-md bg-white/70 px-2 py-1.5 text-[13px] font-medium text-sky-700 ring-1 ring-black/5 hover:underline"
          >
            <img src={logo} alt="" className="size-5 shrink-0" />
            <span className="truncate">{p.replace(/^https?:\/\//, "")}</span>
          </a>
        ) : (
          <span key={i}>{p}</span>
        ),
      )}
    </>
  )
}
