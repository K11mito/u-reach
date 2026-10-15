import { useEffect, useMemo, useRef, useState } from "react"
import { Pause, Play } from "lucide-react"
import { cn } from "@/lib/utils"
import { Grain } from "./Grain"
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

/** The U-Reach mark: a small grain square. */
export function Mark({ size = 32, className }: { size?: number; className?: string }) {
  return (
    <Grain seed="u-reach" palette="dusk" className={cn("grid shrink-0 place-items-center rounded-[28%] font-semibold text-white", className)} style={{ width: size, height: size, fontSize: size * 0.45 }}>
      <span>U</span>
    </Grain>
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

/** A WhatsApp-style voice-note player. `dark` matches WhatsApp's dark theme. */
export function VoiceNote({ src, durationMs, seed, outgoing, big, dark }: { src?: string; durationMs?: number; seed: number; outgoing?: boolean; big?: boolean; dark?: boolean }) {
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
          dark ? "bg-white/15 text-white hover:bg-white/25" : outgoing ? "bg-wa-dark text-white hover:bg-wa-dark/90" : "bg-primary text-white hover:bg-primary/90",
        )}
        title={src ? "Play" : "Audio not rendered yet"}
      >
        {playing ? <Pause className={big ? "size-6" : "size-4"} /> : <Play className={cn(big ? "size-6" : "size-4", "translate-x-px")} />}
      </button>
      <div className={cn("flex min-w-0 flex-1 items-center gap-[2px] overflow-hidden", big ? "h-10" : "h-6")}>
        {heights.map((h, i) => (
          <span
            key={i}
            className={cn(
              "w-[3px] min-w-px shrink rounded-full transition-colors",
              i / heights.length <= progress && playing ? (dark ? "bg-[#53bdeb]" : outgoing ? "bg-wa-dark" : "bg-primary") : dark ? "bg-white/35" : "bg-foreground/25",
            )}
            style={{ height: `${h * 100}%` }}
          />
        ))}
      </div>
      <span className={cn("shrink-0 tabular-nums", dark ? "text-white/60" : "text-muted-foreground", big ? "text-base" : "text-xs")}>{fmtDur(durationMs)}</span>
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
export function Ticks({ status, voice, dark }: { status?: string; voice?: boolean; dark?: boolean }) {
  if (!status) return null
  const blue = status === "read" || status === "played"
  const label = status === "played" ? (voice ? "Played" : "Read") : status === "read" ? "Read" : status === "delivered" ? "Delivered" : "Sent"
  return (
    <span className={cn("inline-flex items-center", blue ? (dark ? "text-[#53bdeb]" : "text-sky-500") : dark ? "text-white/50" : "text-muted-foreground")} title={label}>
      <svg viewBox="0 0 18 12" className="h-3 w-[18px]" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
        <path d="M1 6.5 4.5 10 11 2" />
        {status !== "sent" && <path d="M7.5 9.5 8 10l6.5-8" />}
      </svg>
    </span>
  )
}

const URL_RE = /(https?:\/\/[^\s]+)/g

/** Text with tappable links, shown like a link preview pill. */
export function Linkified({ text, dark }: { text: string; dark?: boolean }) {
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
            className={cn(
              "mt-1 flex items-center gap-1.5 rounded-md px-2 py-1.5 text-[13px] font-medium hover:underline",
              dark ? "bg-black/20 text-[#53bdeb]" : "bg-white/70 text-sky-700 ring-1 ring-black/5",
            )}
          >
            <Mark size={20} />
            <span className="truncate">{p.replace(/^https?:\/\//, "")}</span>
          </a>
        ) : (
          <span key={i}>{p}</span>
        ),
      )}
    </>
  )
}
