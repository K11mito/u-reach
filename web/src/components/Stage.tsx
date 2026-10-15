import { useEffect, useLayoutEffect, useMemo, useRef, useState, type RefObject } from "react"
import { AudioLines, BrainCircuit, Check, Ear, Hand, MessageCircle, Mic, Send, Smartphone, X } from "lucide-react"
import { cn } from "@/lib/utils"
import { clock, displayName, useCountUp, type Message, type Run, type State } from "@/lib/api"
import { Equalizer, Initials, JohnAvatar, Linkified, StatusChip, Ticks, VoiceNote } from "./bits"
import { BrandHeader } from "./BrandHeader"

const STAGES = [
  { key: "received_at", label: "Received", sub: "message in", icon: MessageCircle },
  { key: "heard_at", label: "Heard", sub: "read & transcribed", icon: Ear },
  { key: "thought_at", label: "Thinking", sub: "writes as John", icon: BrainCircuit },
  { key: "spoken_at", label: "Speaking", sub: "John's cloned voice", icon: AudioLines },
  { key: "sent_at", label: "Sent", sub: "voice note + link", icon: Send },
] as const

type Brain = { spoken_script: string; handoff: boolean; handoff_reason: string; opt_out: boolean }

// The header sits exactly where it does on every other page. Everything below
// it is laid out on a fixed canvas (what a 1920×1080 screen has left under the
// header) and scaled to fit, so a projector shows exactly what the laptop
// shows. Ctrl +/- (or Ctrl + scroll) zooms the content: a smaller canvas,
// scaled up to fit.
const CANVAS = { w: 1888, h: 968 }
const ZOOMS = [0.8, 0.9, 1, 1.1, 1.25, 1.5, 1.75, 2]

function useStageZoom() {
  const [zoom, setZoom] = useState(() => {
    try {
      const z = Number(localStorage.getItem("ureach.stageZoom"))
      return ZOOMS.includes(z) ? z : 1
    } catch {
      return 1
    }
  })
  useEffect(() => {
    try {
      localStorage.setItem("ureach.stageZoom", String(zoom))
    } catch {
      /* ignore */
    }
  }, [zoom])
  useEffect(() => {
    const step = (dir: number) => setZoom((z) => (dir === 0 ? 1 : ZOOMS[Math.min(ZOOMS.length - 1, Math.max(0, ZOOMS.indexOf(z) + dir))]))
    const key = (e: KeyboardEvent) => {
      if (!(e.ctrlKey || e.metaKey)) return
      if (e.key === "=" || e.key === "+") step(1)
      else if (e.key === "-" || e.key === "_") step(-1)
      else if (e.key === "0") step(0)
      else return
      e.preventDefault() // ours, not the browser's: its zoom would just be scaled away
    }
    let wheeled = 0
    const wheel = (e: WheelEvent) => {
      if (!e.ctrlKey) return
      e.preventDefault()
      wheeled += e.deltaY
      if (Math.abs(wheeled) >= 100) {
        step(wheeled < 0 ? 1 : -1)
        wheeled = 0
      }
    }
    window.addEventListener("keydown", key)
    window.addEventListener("wheel", wheel, { passive: false })
    return () => {
      window.removeEventListener("keydown", key)
      window.removeEventListener("wheel", wheel)
    }
  }, [])
  return zoom
}

/** How much to scale a w×h canvas so it fits inside `area`. */
function useFit(area: RefObject<HTMLDivElement | null>, w: number, h: number) {
  const [scale, setScale] = useState(1)
  useLayoutEffect(() => {
    const el = area.current
    if (!el) return
    const fit = () => setScale(Math.min(el.clientWidth / w, el.clientHeight / h))
    fit()
    const ro = new ResizeObserver(fit)
    ro.observe(el)
    return () => ro.disconnect()
  }, [area, w, h])
  return scale
}

/** Full-screen projector view: the live pipeline above the conversation, next to the mirrored phone. */
export function Stage({
  state,
  runs,
  messages,
  loadMessages,
  onExit,
}: {
  state: State
  runs: Run[]
  messages: Record<number, Message[]>
  loadMessages: (id: number) => void
  onExit: () => void
}) {
  const run = runs[0]
  const contact = state.contacts.find((c) => c.id === run?.contact_id)
  const [now, setNow] = useState(Date.now())

  useEffect(() => {
    const t = window.setInterval(() => setNow(Date.now()), 100)
    return () => window.clearInterval(t)
  }, [])
  useEffect(() => {
    if (run && !messages[run.contact_id]) loadMessages(run.contact_id)
  }, [run, messages, loadMessages])
  useEffect(() => {
    const key = (e: KeyboardEvent) => {
      if (e.key === "Escape" && !document.fullscreenElement) onExit()
      if (e.key === "f" || e.key === "F") {
        if (document.fullscreenElement) document.exitFullscreen()
        else document.documentElement.requestFullscreen().catch(() => {})
      }
    }
    window.addEventListener("keydown", key)
    return () => window.removeEventListener("keydown", key)
  }, [onExit])

  const brain = useMemo(() => {
    try {
      return run?.brain ? (JSON.parse(run.brain) as Brain) : null
    } catch {
      return null
    }
  }, [run?.brain])

  // The whole conversation, newest at the bottom; the current exchange is highlighted.
  const thread = (run ? (messages[run.contact_id] ?? []) : []).slice(-16)
  const inCurrent = (m: Message) =>
    !!run && (m.run_id === run.id || (m.direction === "in" && m.created_at >= run.received_at - 1500 && (!run.heard_at || m.created_at <= run.heard_at + 500)))

  const scroller = useRef<HTMLDivElement>(null)
  const scrolled = useRef(false)
  useLayoutEffect(() => {
    const el = scroller.current
    if (!el || !thread.length) return
    // Jump on first paint (a projector shouldn't animate in), glide for new messages.
    el.scrollTo({ top: el.scrollHeight, behavior: scrolled.current ? "smooth" : "auto" })
    scrolled.current = true
  }, [thread.length, run?.status, run?.thought_at])

  const zoom = useStageZoom()
  const w = Math.round(CANVAS.w / zoom)
  const h = Math.round(CANVAS.h / zoom)
  const area = useRef<HTMLDivElement>(null)
  const scale = useFit(area, w, h)

  const live = run?.status === "running"
  const end = run?.sent_at || (live ? now : 0)
  const total = run && end ? (end - run.received_at) / 1000 : 0

  return (
    <div className="fixed inset-0 flex flex-col gap-4 overflow-hidden bg-background bg-[radial-gradient(900px_500px_at_0%_0%,oklch(0.94_0.04_285),transparent)] p-4">
      <BrandHeader />

      <div ref={area} className="relative min-h-0 flex-1">
        <main
          className="absolute top-1/2 left-1/2 grid grid-cols-[1fr_420px] gap-4"
          style={{ width: w, height: h, transform: `translate(-50%, -50%) scale(${scale})` }}
        >
          <section className="flex min-h-0 flex-col gap-4">
            {/* Who is answering, and the week so far. */}
            <div className="flex items-center gap-4 rounded-2xl border bg-card px-5 py-3 shadow-sm">
              <JohnAvatar size={46} src={state.ambassador.avatar} />
              <div className="min-w-0 flex-1">
                <div className="text-xl font-extrabold tracking-tight">John's AI voice</div>
                <div className="flex items-center gap-1.5 text-sm text-muted-foreground">
                  <span className={cn("size-2 rounded-full", state.whatsapp.connected ? "bg-wa" : "bg-foreground/25")} />
                  {state.whatsapp.connected ? `Live on WhatsApp +${state.whatsapp.phone}` : "Standing by"}
                </div>
              </div>
              <Counter label="hours saved this week" value={state.hours_saved} decimals={1} />
              <Counter label="voice notes this week" value={state.stats.replies_week} />
              <button onClick={onExit} className="rounded-xl p-2 text-muted-foreground transition hover:bg-muted hover:text-foreground" title="Exit (Esc) · F for fullscreen">
                <X />
              </button>
            </div>

            <div className="grid grid-cols-5 gap-3">
              {STAGES.map((s, i) => {
                const at = run ? run[s.key] : 0
                const prev = i > 0 && run ? run[STAGES[i - 1].key] : 0
                const done = at > 0
                const active = !!run && live && !done && (i === 0 || prev > 0)
                const ms = done ? (i === 0 ? 0 : at - prev) : active && prev ? now - prev : 0
                const Icon = s.icon
                return (
                  <div
                    key={s.key}
                    className={cn(
                      "rounded-2xl border p-4 transition-all duration-300",
                      active
                        ? "scale-[1.03] border-transparent bg-gradient-to-br from-indigo-600 to-purple-700 text-white shadow-lg shadow-indigo-500/30"
                        : done
                          ? "border-wa/40 bg-card shadow-sm"
                          : "bg-card/60 text-muted-foreground",
                    )}
                  >
                    <div className="flex items-center justify-between">
                      <div className={cn("grid size-10 place-items-center rounded-xl", active ? "bg-white/20" : done ? "bg-wa/15 text-wa-dark" : "bg-muted")}>
                        {done && i > 0 ? <Check className="size-5" /> : <Icon className={cn("size-5", active && "animate-pulse")} />}
                      </div>
                      <span className={cn("font-mono text-sm tabular-nums", active ? "text-white" : "text-muted-foreground")}>
                        {i === 0 ? (at ? clock(at) : "") : ms ? `${(ms / 1000).toFixed(1)}s` : ""}
                      </span>
                    </div>
                    <div className="mt-3 text-lg font-bold">{s.label}</div>
                    <div className={cn("truncate text-xs", active ? "text-white/80" : "text-muted-foreground")}>{s.sub}</div>
                  </div>
                )
              })}
            </div>

            {!run ? (
              <div className="grid flex-1 place-items-center rounded-2xl border bg-card text-center shadow-sm">
                <div>
                  <div className="text-4xl font-extrabold tracking-tight">Text John on WhatsApp</div>
                  <div className="mt-3 font-mono text-2xl text-muted-foreground">{state.whatsapp.phone ? `+${state.whatsapp.phone}` : "Waiting for the first message…"}</div>
                </div>
              </div>
            ) : (
              <div className="flex min-h-0 flex-1 flex-col overflow-hidden rounded-2xl border bg-card shadow-sm">
                <div className="flex items-center gap-3 border-b px-5 py-3">
                  <Initials name={displayName(contact) || "?"} src={contact?.avatar} size={42} />
                  <div className="min-w-0 flex-1">
                    <div className="truncate text-lg font-bold">{displayName(contact)}</div>
                    <div className="truncate text-xs text-muted-foreground">{contact?.context || "messaging John on WhatsApp"}</div>
                  </div>
                  {contact && <StatusChip status={contact.status} />}
                </div>
                {brain?.handoff && (
                  <div className="flex items-center gap-2 border-b border-amber/30 bg-amber/10 px-5 py-2 text-sm font-semibold text-amber-900">
                    <Hand className="size-4 shrink-0 text-amber-600" /> Handed to the real John{brain.handoff_reason ? `: ${brain.handoff_reason}` : ""}
                  </div>
                )}
                <div
                  ref={scroller}
                  className="min-h-0 flex-1 space-y-3 overflow-y-auto bg-[radial-gradient(circle_at_1px_1px,oklch(0.9_0.02_285)_1px,transparent_0)] bg-[length:22px_22px] px-6 py-5 [scrollbar-width:none]"
                >
                  {thread.map((m) => (
                    <StageBubble key={m.id} m={m} current={inCurrent(m)} who={displayName(contact)} />
                  ))}
                  {live && (
                    <div className="flex justify-end">
                      <div className="inline-flex items-center gap-3 rounded-xl rounded-tr-sm bg-wa-bubble px-5 py-3 text-lg font-semibold text-wa-dark ring-1 ring-wa/15">
                        {run.thought_at ? (
                          <>
                            <Mic className="size-5 animate-pulse" /> recording audio…
                          </>
                        ) : (
                          <>
                            <Equalizer /> thinking…
                          </>
                        )}
                      </div>
                    </div>
                  )}
                  {run.status === "failed" && <p className="text-right text-amber-700">Couldn't reply: {run.error}</p>}
                </div>
              </div>
            )}

            {run && (
              <div className="flex items-center justify-between px-1 text-sm text-muted-foreground">
                <span className="font-mono text-xs">
                  {run.model ? `${run.model} · ` : ""}
                  {state.ambassador.tts_model}
                </span>
                <span className="font-mono text-base font-semibold text-foreground">
                  {total > 0 ? `${total.toFixed(1)}s` : ""} <span className="font-normal text-muted-foreground">{!live && total > 0 ? "from message to voice note" : ""}</span>
                </span>
              </div>
            )}
          </section>

          {/* The mirrored phone (scrcpy) goes on top of this panel. */}
          <aside className="grid place-items-center rounded-2xl bg-gradient-to-b from-indigo-600 to-purple-700 shadow-lg shadow-indigo-500/20">
            <div className="grid aspect-[9/19] w-[72%] place-items-center rounded-[2.2rem] border-2 border-dashed border-white/30">
              <div className="text-center text-white/70">
                <Smartphone className="mx-auto size-8" />
                <div className="mt-2 text-sm">Mirrored phone here</div>
              </div>
            </div>
          </aside>
        </main>
      </div>
    </div>
  )
}

function StageBubble({ m, current, who }: { m: Message; current: boolean; who: string }) {
  const out = m.direction === "out"
  return (
    <div className={cn("flex transition-opacity duration-500", out ? "justify-end" : "justify-start", current ? "opacity-100" : "opacity-60")}>
      <div
        className={cn(
          "max-w-[78%] rounded-xl px-4 py-3",
          out ? "rounded-tr-sm bg-wa-bubble ring-1 ring-wa/15" : "rounded-tl-sm bg-card ring-1 ring-border",
          current && (out ? "ring-2 ring-wa/50" : "ring-2 ring-indigo-400/50"),
        )}
      >
        {m.kind === "voice" ? (
          <div className="space-y-2">
            <VoiceNote src={m.audio_path} durationMs={m.duration_ms} seed={m.id} outgoing={out} big={current} />
            {m.text && <p className={cn("leading-snug text-foreground/75 italic", current ? "text-[15px]" : "text-sm")}>“{m.text}”</p>}
          </div>
        ) : (
          <div className={cn("leading-snug whitespace-pre-wrap [overflow-wrap:anywhere]", current ? "text-lg" : "text-base")}>
            <Linkified text={m.text.trim()} />
          </div>
        )}
        <div className="mt-1 flex items-center justify-end gap-1.5 text-xs text-muted-foreground">
          {out && m.kind === "voice" && m.status === "played" && <span className="font-semibold text-sky-600">{who} listened</span>}
          {clock(m.created_at)}
          {out && <Ticks status={m.status} voice={m.kind === "voice"} />}
        </div>
      </div>
    </div>
  )
}

function Counter({ label, value, decimals = 0 }: { label: string; value: number; decimals?: number }) {
  const shown = useCountUp(value, 1400)
  return (
    <div className="border-l pl-5 text-right">
      <div className="text-3xl leading-none font-extrabold tracking-tight tabular-nums">{shown.toFixed(decimals)}</div>
      <div className="mt-1 text-[11px] tracking-wide text-muted-foreground uppercase">{label}</div>
    </div>
  )
}
