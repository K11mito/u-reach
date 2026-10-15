import { useEffect, useLayoutEffect, useMemo, useRef, useState } from "react"
import { AudioLines, BrainCircuit, Check, Ear, MessageCircle, Mic, Send, Smartphone, X } from "lucide-react"
import { cn } from "@/lib/utils"
import { clock, displayName, useCountUp, type Message, type Run, type State } from "@/lib/api"
import { Equalizer, Initials, JohnAvatar, Linkified, StatusChip, Ticks, VoiceNote } from "./bits"

const STAGES = [
  { key: "received_at", label: "Received", sub: "message in", icon: MessageCircle },
  { key: "heard_at", label: "Heard", sub: "read & transcribed", icon: Ear },
  { key: "thought_at", label: "Thinking", sub: "writes as John", icon: BrainCircuit },
  { key: "spoken_at", label: "Speaking", sub: "John's cloned voice", icon: AudioLines },
  { key: "sent_at", label: "Sent", sub: "voice note + link", icon: Send },
] as const

type Brain = { spoken_script: string; handoff: boolean; handoff_reason: string; opt_out: boolean }

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

  const live = run?.status === "running"
  const end = run?.sent_at || (live ? now : 0)
  const total = run && end ? (end - run.received_at) / 1000 : 0

  return (
    <div className="fixed inset-0 flex flex-col overflow-hidden bg-[oklch(0.2_0.08_280)] bg-[radial-gradient(1200px_700px_at_15%_0%,oklch(0.42_0.2_280),transparent),radial-gradient(900px_600px_at_90%_100%,oklch(0.38_0.2_305),transparent)] text-white">
      <header className="flex items-center justify-between px-10 pt-7">
        <div className="flex items-center gap-3">
          <JohnAvatar size={46} src={state.ambassador.avatar} />
          <div>
            <div className="text-xl font-extrabold tracking-tight">John's AI voice</div>
            <div className="flex items-center gap-1.5 text-sm text-white/70">
              <span className={cn("size-2 rounded-full", state.whatsapp.connected ? "bg-wa shadow-[0_0_8px] shadow-wa" : "bg-white/40")} />
              U-Reach · {state.whatsapp.connected ? `live on WhatsApp +${state.whatsapp.phone}` : "standing by"}
            </div>
          </div>
        </div>
        <div className="flex items-center gap-8">
          <Counter label="hours saved this week" value={state.hours_saved} decimals={1} />
          <Counter label="voice notes this week" value={state.stats.replies_week} />
          <button onClick={onExit} className="rounded-full p-2 text-white/50 hover:bg-white/10 hover:text-white" title="Exit (Esc) · F for fullscreen">
            <X />
          </button>
        </div>
      </header>

      <main className="grid min-h-0 flex-1 grid-cols-[1fr_clamp(260px,27vw,420px)] gap-8 px-10 pt-6 pb-8 2xl:gap-10">
        <section className="flex min-h-0 flex-col gap-5">
          <div className="grid grid-cols-5 gap-2 xl:gap-3">
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
                    "relative rounded-3xl border p-4 transition-all duration-300",
                    done ? "border-wa/50 bg-wa/15" : active ? "scale-[1.03] border-white/60 bg-white/15 shadow-[0_0_40px_-5px] shadow-purple-400/60" : "border-white/10 bg-white/5",
                  )}
                >
                  <div className="flex items-center justify-between">
                    <div className={cn("grid size-10 place-items-center rounded-2xl", done ? "bg-wa text-white" : active ? "bg-white text-indigo-700" : "bg-white/10 text-white/50")}>
                      {done && i > 0 ? <Check className="size-5" /> : <Icon className={cn("size-5", active && "animate-pulse")} />}
                    </div>
                    <span className="font-mono text-xs text-white/80 tabular-nums xl:text-sm">{i === 0 ? (at ? clock(at) : "") : ms ? `${(ms / 1000).toFixed(1)}s` : ""}</span>
                  </div>
                  <div className="mt-3 text-base font-bold xl:text-lg">{s.label}</div>
                  <div className="truncate text-xs text-white/60">{s.sub}</div>
                </div>
              )
            })}
          </div>

          {!run ? (
            <div className="grid flex-1 place-items-center rounded-3xl border border-dashed border-white/20 text-center">
              <div>
                <div className="text-4xl font-extrabold">Text John on WhatsApp</div>
                <div className="mt-3 font-mono text-2xl text-white/70">{state.whatsapp.phone ? `+${state.whatsapp.phone}` : "Waiting for the first message…"}</div>
              </div>
            </div>
          ) : (
            <div className="flex min-h-0 flex-1 flex-col overflow-hidden rounded-3xl border border-white/10 bg-white/5 backdrop-blur">
              <div className="flex items-center gap-3 border-b border-white/10 px-6 py-3.5">
                <Initials name={displayName(contact) || "?"} src={contact?.avatar} size={42} />
                <div className="min-w-0 flex-1">
                  <div className="truncate text-lg font-bold">{displayName(contact)}</div>
                  <div className="truncate text-xs text-white/60">{contact?.context || "messaging John on WhatsApp"}</div>
                </div>
                {contact && <StatusChip status={contact.status} className="bg-white/90" />}
              </div>
              {brain?.handoff && (
                <div className="border-b border-amber/30 bg-amber/15 px-6 py-2 text-sm font-semibold text-amber">
                  ⚑ Handed to the real John{brain.handoff_reason ? `: ${brain.handoff_reason}` : ""}
                </div>
              )}
              <div ref={scroller} className="min-h-0 flex-1 space-y-3 overflow-y-auto px-6 py-5 [scrollbar-width:none]">
                {thread.map((m) => (
                  <StageBubble key={m.id} m={m} current={inCurrent(m)} who={displayName(contact)} />
                ))}
                {live && (
                  <div className="flex justify-end">
                    <div className="inline-flex items-center gap-3 rounded-2xl rounded-tr-md bg-wa-bubble px-5 py-3 text-lg font-semibold text-wa-dark shadow-lg">
                      {run.thought_at ? (
                        <>
                          <Mic className="size-5 animate-pulse" /> recording audio…
                        </>
                      ) : (
                        <>
                          <Equalizer className="text-wa-dark" /> thinking…
                        </>
                      )}
                    </div>
                  </div>
                )}
                {run.status === "failed" && <p className="text-right text-amber">Couldn't reply: {run.error}</p>}
              </div>
            </div>
          )}

          {run && (
            <div className="flex items-center justify-between text-sm text-white/60">
              <span>
                {run.model ? `${run.model} · ` : ""}
                {state.ambassador.tts_model}
              </span>
              <span className="font-mono text-base text-white">
                {total > 0 ? `${total.toFixed(1)}s` : ""} {!live && total > 0 ? "from message to voice note" : ""}
              </span>
            </div>
          )}
        </section>

        <aside className="grid place-items-center rounded-[2.5rem] border-2 border-dashed border-white/15">
          <div className="text-center text-white/30">
            <Smartphone className="mx-auto size-10" />
            <div className="mt-2 text-sm">Mirrored phone here</div>
          </div>
        </aside>
      </main>
    </div>
  )
}

function StageBubble({ m, current, who }: { m: Message; current: boolean; who: string }) {
  const out = m.direction === "out"
  return (
    <div className={cn("flex transition-opacity duration-500", out ? "justify-end" : "justify-start", current ? "opacity-100" : "opacity-75")}>
      <div
        className={cn(
          "max-w-[78%] rounded-2xl px-4 py-3 text-foreground shadow-lg",
          out ? "rounded-tr-md bg-wa-bubble" : "rounded-tl-md bg-white",
          current && (out ? "ring-2 ring-wa" : "ring-2 ring-white/70"),
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
    <div className="text-right">
      <div className="text-4xl font-extrabold tabular-nums">{shown.toFixed(decimals)}</div>
      <div className="text-xs tracking-wide text-white/60 uppercase">{label}</div>
    </div>
  )
}
