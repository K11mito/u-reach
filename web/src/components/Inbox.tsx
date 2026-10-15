import { useEffect, useMemo, useRef, useState } from "react"
import { Bot, Hand, Link2, Mic, Send, Sparkles, Undo2, UserRound } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Textarea } from "@/components/ui/textarea"
import { cn } from "@/lib/utils"
import { api, clock, displayName, timeAgo, type Contact, type Message, type Run } from "@/lib/api"
import { Initials, StatusChip, VoiceNote } from "./bits"
import { SimulateBox } from "./SimulateBox"

export function Inbox({
  contacts,
  last,
  messages,
  loadMessages,
  runs,
}: {
  contacts: Contact[]
  last: Record<number, Message>
  messages: Record<number, Message[]>
  loadMessages: (id: number) => void
  runs: Run[]
}) {
  const [selected, setSelected] = useState<number | null>(null)
  const sorted = useMemo(() => [...contacts].sort((a, b) => (b.last_message_at || b.created_at) - (a.last_message_at || a.created_at)), [contacts])
  const current = sorted.find((c) => c.id === selected) ?? sorted[0]

  useEffect(() => {
    if (current && !messages[current.id]) loadMessages(current.id)
  }, [current, messages, loadMessages])

  // Follow the newest live conversation when a new run starts.
  const latestRun = runs[0]
  const lastFollowed = useRef<number>(0)
  useEffect(() => {
    if (latestRun && latestRun.status === "running" && latestRun.id !== lastFollowed.current) {
      lastFollowed.current = latestRun.id
      setSelected(latestRun.contact_id)
      loadMessages(latestRun.contact_id)
    }
  }, [latestRun, loadMessages])

  const running = new Set(runs.filter((r) => r.status === "running").map((r) => r.contact_id))

  return (
    <div className="grid min-h-0 flex-1 grid-cols-[340px_1fr] gap-4">
      <div className="flex min-h-0 flex-col overflow-hidden rounded-2xl border bg-card shadow-sm">
        <div className="flex items-center justify-between border-b px-4 py-3">
          <h2 className="font-bold">Chats</h2>
          <span className="text-xs text-muted-foreground">{contacts.length} leads</span>
        </div>
        <div className="min-h-0 flex-1 overflow-y-auto">
          {sorted.map((c) => (
            <ChatRow key={c.id} c={c} active={current?.id === c.id} live={running.has(c.id)} last={messages[c.id]?.at(-1) ?? last[c.id]} onClick={() => setSelected(c.id)} />
          ))}
        </div>
        <SimulateBox />
      </div>
      {current ? (
        <ChatView contact={current} messages={messages[current.id] ?? []} live={running.has(current.id)} />
      ) : (
        <div className="grid place-items-center rounded-2xl border bg-card text-muted-foreground">No chats yet</div>
      )}
    </div>
  )
}

function ChatRow({ c, active, live, last, onClick }: { c: Contact; active: boolean; live: boolean; last?: Message; onClick: () => void }) {
  return (
    <button
      onClick={onClick}
      className={cn("flex w-full items-center gap-3 border-b border-border/60 px-4 py-3 text-left transition hover:bg-muted/60", active && "bg-accent/60 hover:bg-accent/60")}
    >
      <div className="relative">
        <Initials name={displayName(c)} size={42} />
        {live && <span className="absolute -right-0.5 -bottom-0.5 size-3.5 animate-pulse rounded-full border-2 border-card bg-wa" />}
      </div>
      <div className="min-w-0 flex-1">
        <div className="flex items-center justify-between gap-2">
          <span className="truncate font-semibold">{displayName(c)}</span>
          <span className="shrink-0 text-[11px] text-muted-foreground">{timeAgo(c.last_message_at)}</span>
        </div>
        <div className="mt-0.5 flex items-center justify-between gap-2">
          <span className="truncate text-xs text-muted-foreground">
            {live ? <span className="font-medium text-wa-dark">recording audio…</span> : last ? preview(last) : c.context}
          </span>
          <StatusChip status={c.status} />
        </div>
      </div>
    </button>
  )
}

function preview(m: Message) {
  const who = m.direction === "out" ? (m.source === "john" ? "You: " : "AI: ") : ""
  return m.kind === "voice" ? `${who}🎤 Voice note` : who + m.text.split("\n")[0]
}

function ChatView({ contact, messages, live }: { contact: Contact; messages: Message[]; live: boolean }) {
  const bottom = useRef<HTMLDivElement>(null)
  const [draft, setDraft] = useState("")
  const [busy, setBusy] = useState(false)
  useEffect(() => bottom.current?.scrollIntoView({ behavior: "smooth" }), [messages.length, live])

  const paused = contact.status === "human" || contact.status === "needs_john"
  const send = async () => {
    if (!draft.trim()) return
    setBusy(true)
    try {
      await api.send(contact.id, draft.trim())
      setDraft("")
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="flex min-h-0 flex-col overflow-hidden rounded-2xl border bg-card shadow-sm">
      <div className="flex items-center gap-3 border-b px-5 py-3">
        <Initials name={displayName(contact)} size={44} />
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <span className="text-lg font-bold">{displayName(contact)}</span>
            <StatusChip status={contact.status} />
            {contact.link_sent_at > 0 && (
              <span className="inline-flex items-center gap-1 rounded-full bg-secondary px-2 py-0.5 text-[11px] font-semibold text-secondary-foreground">
                <Link2 className="size-3" /> Link sent
              </span>
            )}
          </div>
          <div className="truncate text-xs text-muted-foreground">
            {contact.phone && !contact.phone.startsWith("99") ? `+${contact.phone} · ` : ""}
            {contact.context || (contact.push_name ? `WhatsApp name: ${contact.push_name}` : "New lead")}
          </div>
        </div>
        {paused ? (
          <Button variant="outline" onClick={() => api.setStatus(contact.id, "auto")} className="rounded-full">
            <Undo2 /> Hand back to AI
          </Button>
        ) : (
          <Button onClick={() => api.setStatus(contact.id, "human")} className="rounded-full">
            <Hand /> Take over
          </Button>
        )}
      </div>

      {(contact.handoff_reason || (contact.memory?.length ?? 0) > 0) && (
        <div className="flex flex-wrap items-center gap-1.5 border-b bg-muted/40 px-5 py-2 text-xs">
          {contact.status === "needs_john" && contact.handoff_reason && (
            <span className="rounded-full bg-amber/25 px-2 py-0.5 font-semibold text-amber-900">⚑ {contact.handoff_reason}</span>
          )}
          {contact.memory?.map((m) => (
            <span key={m} className="rounded-full bg-card px-2 py-0.5 text-muted-foreground ring-1 ring-border">
              {m}
            </span>
          ))}
        </div>
      )}

      <div className="min-h-0 flex-1 space-y-2 overflow-y-auto bg-[radial-gradient(circle_at_1px_1px,oklch(0.9_0.02_285)_1px,transparent_0)] bg-[length:22px_22px] px-6 py-5">
        {messages.map((m) => (
          <Bubble key={m.id} m={m} />
        ))}
        {live && (
          <div className="flex justify-end">
            <div className="inline-flex items-center gap-2 rounded-2xl bg-wa-bubble px-3 py-2 text-sm font-medium text-wa-dark shadow-sm">
              <Mic className="size-4 animate-pulse" /> John is recording audio…
            </div>
          </div>
        )}
        <div ref={bottom} />
      </div>

      {paused ? (
        <div className="flex items-end gap-2 border-t p-3">
          <Textarea
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && !e.shiftKey) {
                e.preventDefault()
                send()
              }
            }}
            placeholder={`Reply to ${displayName(contact)} as John…`}
            className="min-h-11 resize-none rounded-xl"
            rows={1}
          />
          <Button onClick={send} disabled={busy || !draft.trim()} className="h-11 rounded-xl bg-wa-dark hover:bg-wa-dark/90">
            <Send />
          </Button>
        </div>
      ) : (
        <div className="flex items-center gap-2 border-t bg-wa/5 px-5 py-2.5 text-xs text-wa-dark">
          <Sparkles className="size-3.5" /> John's AI voice is answering this chat. Take over anytime.
        </div>
      )}
    </div>
  )
}

function Bubble({ m }: { m: Message }) {
  const out = m.direction === "out"
  const ai = m.source === "ai" || m.source === "seed"
  return (
    <div className={cn("flex", out ? "justify-end" : "justify-start")}>
      <div className={cn("max-w-[72%] rounded-2xl px-3.5 py-2 shadow-sm", out ? "rounded-br-md bg-wa-bubble" : "rounded-bl-md bg-card ring-1 ring-border/60")}>
        {m.kind === "voice" ? (
          <div className="space-y-1.5 py-1">
            <VoiceNote src={m.audio_path} durationMs={m.duration_ms} seed={m.id} outgoing={out} />
            {m.text && <p className="border-t border-foreground/10 pt-1.5 text-[13px] leading-snug text-foreground/80 italic">“{m.text}”</p>}
          </div>
        ) : (
          <p className="text-sm leading-snug whitespace-pre-wrap [overflow-wrap:anywhere]">{m.text}</p>
        )}
        <div className="mt-1 flex items-center justify-end gap-1.5 text-[10px] text-muted-foreground">
          {out && ai && (
            <span className="inline-flex items-center gap-0.5 font-semibold text-primary">
              <Bot className="size-3" /> AI
            </span>
          )}
          {out && m.source === "john" && (
            <span className="inline-flex items-center gap-0.5 font-semibold text-foreground/70">
              <UserRound className="size-3" /> John
            </span>
          )}
          {clock(m.created_at)}
        </div>
      </div>
    </div>
  )
}
