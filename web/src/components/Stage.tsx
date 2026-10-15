import { useEffect, useLayoutEffect, useMemo, useRef, useState } from "react"
import { AudioLines, BrainCircuit, Check, Ear, Hand, MessageCircle, Mic, Send, Smartphone, X } from "lucide-react"
import { cn } from "@/lib/utils"
import { clock, displayName, useCountUp, type Message, type Run, type State } from "@/lib/api"
import { Equalizer, Initials, JohnAvatar, Linkified, StatusChip, Ticks, VoiceNote } from "./bits"
import { Grain } from "./Grain"

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
    <div className="fixed inset-0 flex flex-col overflow-hidden bg-[#0b0b0c] text-white">
      <header className="flex items-center justify-between px-10 pt-7">
        <div className="flex items-center gap-3.5">
          <JohnAvatar size={44} src={state.ambassador.avatar} />
          <div>
            <div className="text-xl font-semibold tracking-tight">John's AI voice</div>
            <div className="flex items-center gap-2 text-sm text-white/55">
              <span className={cn("size-1.5 rounded-full", state.whatsapp.connected ? "bg-wa" : "bg-white/30")} />
              U-Reach · {state.whatsapp.connected ? `live on WhatsApp +${state.whatsapp.phone}` : "standing by"}
            </div>
          </div>
        </div>
        <div className="flex items-center gap-10">
          <Counter label="hours saved this week" value={state.hours_saved} decimals={1} />
          <Counter label="voice notes this week" value={state.stats.replies_week} />
          <button onClick={onExit} className="rounded-md p-2 text-white/40 transition hover:bg-white/10 hover:text-white" title="Exit (Esc) · F for fullscreen">
            <X />
          </button>
        </div>
      </header>

      <main className="grid min-h-0 flex-1 grid-cols-[1fr_clamp(260px,27vw,420px)] gap-6 px-10 pt-6 pb-8 2xl:gap-8">
        <section className="flex min-h-0 flex-col gap-4">
          <div className="grid grid-cols-5 gap-2 xl:gap-3">
            {STAGES.map((s, i) => {
              const at = run ? run[s.key] : 0
              const prev = i > 0 && run ? run[STAGES[i - 1].key] : 0
              const done = at > 0
              const active = !!run && live && !done && (i === 0 || prev > 0)
              const ms = done ? (i === 0 ? 0 : at - prev) : active && prev ? now - prev : 0
              const Icon = s.icon
              return (
                <div key={s.key} className={cn("relative overflow-hidden rounded-xl border p-4 transition-colors duration-300", active ? "border-transparent" : done ? "border-white/15 bg-white/[0.06]" : "border-white/[0.07] bg-white/[0.02]")}>
                  {/* The step being worked on fills with a drifting grain gradient. */}
                  <Grain seed={`stage-${s.key}`} palette="dusk" className={cn("animate-drift absolute inset-0 transition-opacity duration-500", active ? "opacity-100" : "opacity-0")} />
                  <div className="relative">
                    <div className="flex items-center justify-between">
                      <div className={cn("grid size-9 place-items-center rounded-lg", done ? "bg-wa/15 text-wa" : active ? "bg-white/25 text-white" : "bg-white/[0.06] text-white/35")}>
                        {done && i > 0 ? <Check className="size-[18px]" /> : <Icon className={cn("size-[18px]", active && "animate-pulse")} />}
                      </div>
                      <span className={cn("font-mono text-xs tabular-nums xl:text-sm", active ? "text-white" : "text-white/60")}>{i === 0 ? (at ? clock(at) : "") : ms ? `${(ms / 1000).toFixed(1)}s` : ""}</span>
                    </div>
                    <div className={cn("mt-3 text-base font-medium xl:text-lg", !done && !active && "text-white/45")}>{s.label}</div>
                    <div className={cn("truncate text-xs", active ? "text-white/85" : "text-white/40")}>{s.sub}</div>
                  </div>
                </div>
              )
            })}
          </div>

          {!run ? (
            <div className="grid flex-1 place-items-center rounded-xl border border-white/[0.07] bg-white/[0.02] text-center">
              <div>
                <div className="text-4xl font-semibold tracking-tight">Text John on WhatsApp</div>
                <div className="mt-3 font-mono text-2xl text-white/55">{state.whatsapp.phone ? `+${state.whatsapp.phone}` : "Waiting for the first message…"}</div>
              </div>
            </div>
          ) : (
            <div className="flex min-h-0 flex-1 flex-col overflow-hidden rounded-xl border border-white/[0.08] bg-[#0f1417]">
              <div className="flex items-center gap-3 border-b border-white/[0.08] bg-[#1b2328] px-5 py-3">
                <Initials name={displayName(contact) || "?"} src={contact?.avatar} size={40} />
                <div className="min-w-0 flex-1">
                  <div className="truncate text-lg font-medium text-[#e9edef]">{displayName(contact)}</div>
                  <div className="truncate text-xs text-white/50">{contact?.context || "messaging John on WhatsApp"}</div>
                </div>
                {contact && <StatusChip status={contact.status} className={contact.status === "needs_john" ? "border-amber/40 text-amber" : "border-white/15 text-white/70"} />}
              </div>
              {brain?.handoff && (
                <div className="flex items-center gap-2 border-b border-amber/20 bg-amber/10 px-5 py-2 text-sm font-medium text-amber">
                  <Hand className="size-4 shrink-0" /> Handed to the real John{brain.handoff_reason ? `: ${brain.handoff_reason}` : ""}
                </div>
              )}
              <div ref={scroller} className="min-h-0 flex-1 space-y-2.5 overflow-y-auto px-6 py-5 [scrollbar-width:none]">
                {thread.map((m) => (
                  <StageBubble key={m.id} m={m} current={inCurrent(m)} who={displayName(contact)} />
                ))}
                {live && (
                  <div className="flex justify-end">
                    <div className="inline-flex items-center gap-3 rounded-xl rounded-tr-sm bg-[#005c4b] px-5 py-3 text-lg text-[#e9edef]">
                      {run.thought_at ? (
                        <>
                          <Mic className="size-5 animate-pulse text-wa" /> recording audio…
                        </>
                      ) : (
                        <>
                          <Equalizer className="text-wa" /> thinking…
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
            <div className="flex items-center justify-between text-sm text-white/40">
              <span className="font-mono text-xs">
                {run.model ? `${run.model} · ` : ""}
                {state.ambassador.tts_model}
              </span>
              <span className="font-mono text-base text-white">
                {total > 0 ? `${total.toFixed(1)}s` : ""} <span className="text-white/45">{!live && total > 0 ? "from message to voice note" : ""}</span>
              </span>
            </div>
          )}
        </section>

        {/* The mirrored phone (scrcpy) sits on a grain gradient, like a product shot. */}
        <Grain seed="stage-phone" palette="dusk" className="grid place-items-center rounded-xl">
          <div className="grid aspect-[9/19] w-[72%] place-items-center rounded-[2.2rem] border border-white/30 bg-black/15 backdrop-blur-sm">
            <div className="text-center text-white/70">
              <Smartphone className="mx-auto size-8" />
              <div className="mt-2 text-sm">Mirrored phone here</div>
            </div>
          </div>
        </Grain>
      </main>
    </div>
  )
}

function StageBubble({ m, current, who }: { m: Message; current: boolean; who: string }) {
  const out = m.direction === "out"
  return (
    <div className={cn("flex transition-opacity duration-500", out ? "justify-end" : "justify-start", current ? "opacity-100" : "opacity-55")}>
      <div className={cn("max-w-[78%] rounded-xl px-4 py-3 text-[#e9edef]", out ? "rounded-tr-sm bg-[#005c4b]" : "rounded-tl-sm bg-[#202c33]")}>
        {m.kind === "voice" ? (
          <div className="space-y-2">
            <VoiceNote src={m.audio_path} durationMs={m.duration_ms} seed={m.id} outgoing={out} big={current} dark />
            {m.text && <p className={cn("leading-snug text-white/70 italic", current ? "text-[15px]" : "text-sm")}>“{m.text}”</p>}
          </div>
        ) : (
          <div className={cn("leading-snug whitespace-pre-wrap [overflow-wrap:anywhere]", current ? "text-lg" : "text-base")}>
            <Linkified text={m.text.trim()} dark />
          </div>
        )}
        <div className="mt-1 flex items-center justify-end gap-1.5 text-xs text-white/50">
          {out && m.kind === "voice" && m.status === "played" && <span className="font-medium text-[#53bdeb]">{who} listened</span>}
          {clock(m.created_at)}
          {out && <Ticks status={m.status} voice={m.kind === "voice"} dark />}
        </div>
      </div>
    </div>
  )
}

function Counter({ label, value, decimals = 0 }: { label: string; value: number; decimals?: number }) {
  const shown = useCountUp(value, 1400)
  return (
    <div className="text-right">
      <div className="text-4xl font-semibold tracking-tight tabular-nums">{shown.toFixed(decimals)}</div>
      <div className="text-xs text-white/45">{label}</div>
    </div>
  )
}
