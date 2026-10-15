import { useEffect, useMemo, useState } from "react"
import { AudioLines, BrainCircuit, Check, Ear, MessageCircle, Send, Smartphone, X } from "lucide-react"
import { cn } from "@/lib/utils"
import { clock, displayName, useCountUp, type Contact, type Message, type Run, type State } from "@/lib/api"
import { Equalizer, Initials, Linkified, Ticks, VoiceNote } from "./bits"

const STAGES = [
  { key: "received_at", label: "Received", sub: "message in", icon: MessageCircle },
  { key: "heard_at", label: "Heard", sub: "read & transcribed", icon: Ear },
  { key: "thought_at", label: "Thinking", sub: "writes as John", icon: BrainCircuit },
  { key: "spoken_at", label: "Speaking", sub: "John's cloned voice", icon: AudioLines },
  { key: "sent_at", label: "Sent", sub: "voice note + link", icon: Send },
] as const

/** Full-screen projector view of the live pipeline, next to the mirrored phone. */
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
    if (run) loadMessages(run.contact_id)
  }, [run?.id, run?.status, loadMessages]) // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => {
    const esc = (e: KeyboardEvent) => {
      if (e.key === "Escape" && !document.fullscreenElement) onExit()
      if (e.key === "f" || e.key === "F") {
        if (document.fullscreenElement) document.exitFullscreen()
        else document.documentElement.requestFullscreen().catch(() => {})
      }
    }
    window.addEventListener("keydown", esc)
    return () => window.removeEventListener("keydown", esc)
  }, [onExit])

  const msgs = run ? (messages[run.contact_id] ?? []) : []
  const inbound = useMemo(
    () => (run ? msgs.filter((m) => m.direction === "in" && m.created_at >= run.received_at - 1000 && (!run.heard_at || m.created_at <= run.heard_at)) : []),
    [msgs, run],
  )
  const reply = msgs.filter((m) => run && m.run_id === run.id)
  const note = reply.find((m) => m.kind === "voice")
  const texts = reply.filter((m) => m.kind === "text")
  const brain = useMemo(() => {
    try {
      return run?.brain ? (JSON.parse(run.brain) as { spoken_script: string; handoff: boolean; handoff_reason: string; opt_out: boolean }) : null
    } catch {
      return null
    }
  }, [run?.brain])

  const total = run ? ((run.sent_at || (run.status === "running" ? now : 0)) - run.received_at) / 1000 : 0

  return (
    <div className="fixed inset-0 flex flex-col overflow-hidden bg-[radial-gradient(1200px_700px_at_15%_0%,oklch(0.42_0.2_280),transparent),radial-gradient(900px_600px_at_90%_100%,oklch(0.38_0.2_305),transparent)] bg-[oklch(0.2_0.08_280)] text-white">
      <header className="flex items-center justify-between px-10 pt-8">
        <div className="flex items-center gap-3">
          <div className="grid size-11 place-items-center rounded-2xl bg-white/15 text-xl font-extrabold backdrop-blur">U</div>
          <div>
            <div className="text-xl font-extrabold tracking-tight">U-Reach</div>
            <div className="flex items-center gap-1.5 text-sm text-white/70">
              <span className={cn("size-2 rounded-full", state.whatsapp.connected ? "bg-wa" : "bg-white/40")} />
              John's AI voice · {state.whatsapp.connected ? "live on WhatsApp" : "standing by"}
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

      <main className="grid min-h-0 flex-1 grid-cols-[1fr_clamp(260px,27vw,420px)] gap-8 px-10 py-8 2xl:gap-10">
        <section className="flex min-h-0 flex-col gap-6">
          <div className="grid grid-cols-5 gap-2 xl:gap-3">
            {STAGES.map((s, i) => {
              const at = run ? run[s.key] : 0
              const prev = i > 0 && run ? run[STAGES[i - 1].key] : 0
              const done = at > 0
              const active = !!run && run.status === "running" && !done && (i === 0 || prev > 0)
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
                    <span className="font-mono text-xs tabular-nums text-white/80 xl:text-sm">
                      {i === 0 ? (at ? clock(at) : "") : ms ? `${(ms / 1000).toFixed(1)}s` : ""}
                    </span>
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
                <div className="text-3xl font-bold">Text John on WhatsApp</div>
                <div className="mt-2 text-lg text-white/60">{state.whatsapp.phone ? `+${state.whatsapp.phone}` : "Waiting for the first message…"}</div>
              </div>
            </div>
          ) : (
            <div className="grid min-h-0 flex-1 grid-cols-2 gap-6">
              <Panel title="Incoming" contact={contact}>
                {inbound.length === 0 && <p className="text-white/50">…</p>}
                {inbound.map((m) => (
                  <div key={m.id} className="rounded-2xl rounded-tl-md bg-white px-4 py-3 text-lg leading-snug text-foreground shadow-lg">
                    {m.kind === "voice" ? <span className="text-muted-foreground">🎤 {m.text || "Voice note…"}</span> : m.text}
                  </div>
                ))}
              </Panel>
              <Panel title="John's reply" john>
                {brain?.handoff && (
                  <div className="rounded-2xl bg-amber/20 px-4 py-2.5 text-sm font-semibold text-amber ring-1 ring-amber/40">
                    ⚑ Handed to the real John{brain.handoff_reason ? `: ${brain.handoff_reason}` : ""}
                  </div>
                )}
                {note ? (
                  <div className="rounded-2xl rounded-tr-md bg-wa-bubble px-4 py-3 text-foreground shadow-lg">
                    <VoiceNote src={note.audio_path} durationMs={note.duration_ms} seed={note.id} outgoing big />
                    <div className="mt-1 flex items-center justify-end gap-1.5 text-xs text-muted-foreground">
                      {note.status === "played" && <span className="font-semibold text-sky-600">{displayName(contact)} listened</span>}
                      <Ticks status={note.status} voice />
                    </div>
                  </div>
                ) : run.status === "running" ? (
                  <div className="flex items-center gap-3 text-xl text-wa">
                    <Equalizer className="text-wa" /> {run.thought_at ? "recording audio…" : "thinking…"}
                  </div>
                ) : run.status === "failed" ? (
                  <p className="text-amber">Couldn't reply: {run.error}</p>
                ) : null}
                {texts.map((m) => (
                  <div key={m.id} className="rounded-2xl rounded-tr-md bg-wa-bubble px-4 py-3 text-base whitespace-pre-wrap text-foreground shadow-lg [overflow-wrap:anywhere]">
                    <Linkified text={m.text.trim()} />
                  </div>
                ))}
                {(note?.text || (!note && run.status !== "running" && brain?.spoken_script)) && (
                  <p className="text-base leading-relaxed text-white/75 italic">“{note?.text || brain?.spoken_script}”</p>
                )}
              </Panel>
            </div>
          )}

          {run && (
            <div className="flex items-center justify-between text-sm text-white/60">
              <span>
                {contact ? `${displayName(contact)} · ` : ""}
                {run.model ? `${run.model} · ` : ""}
                {state.ambassador.tts_model}
              </span>
              <span className="font-mono text-base text-white">
                {total > 0 ? `${total.toFixed(1)}s` : ""} {run.status === "sent" || run.status === "handoff" ? "total" : ""}
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

function Counter({ label, value, decimals = 0 }: { label: string; value: number; decimals?: number }) {
  const shown = useCountUp(value, 1400)
  return (
    <div className="text-right">
      <div className="text-4xl font-extrabold tabular-nums">{shown.toFixed(decimals)}</div>
      <div className="text-xs tracking-wide text-white/60 uppercase">{label}</div>
    </div>
  )
}

function Panel({ title, contact, john, children }: { title: string; contact?: Contact; john?: boolean; children: React.ReactNode }) {
  return (
    <div className="flex min-h-0 flex-col gap-4 overflow-y-auto rounded-3xl border border-white/10 bg-white/5 p-6 backdrop-blur [scrollbar-width:none]">
      <div className="flex items-center gap-3">
        {john ? (
          <div className="grid size-10 place-items-center rounded-full bg-gradient-to-br from-indigo-400 to-purple-500 font-extrabold">J</div>
        ) : (
          <Initials name={displayName(contact) || "?"} size={40} />
        )}
        <div>
          <div className="text-xs tracking-wide text-white/50 uppercase">{title}</div>
          <div className="text-lg font-bold">{john ? "John Klensch" : displayName(contact)}</div>
        </div>
      </div>
      {children}
    </div>
  )
}
