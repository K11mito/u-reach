import { Fragment, useEffect, useLayoutEffect, useMemo, useRef, useState } from "react"
import { Bot, Hand, Link2, Megaphone, Mic, RotateCcw, Search, Send, Sparkles, TriangleAlert, Undo2, UserRound } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Textarea } from "@/components/ui/textarea"
import { cn } from "@/lib/utils"
import { api, clock, dayLabel, displayName, timeAgo, type Contact, type Message, type Run, type Status } from "@/lib/api"
import { Initials, Linkified, StatusChip, Ticks, VoiceNote } from "./bits"
import { SimulateBox } from "./SimulateBox"
import { toast } from "./Toast"

type Filter = "all" | Status

export function Inbox({
  contacts,
  last,
  messages,
  loadMessages,
  runs,
  onCampaign,
}: {
  contacts: Contact[]
  last: Record<number, Message>
  messages: Record<number, Message[]>
  loadMessages: (id: number) => void
  runs: Run[]
  onCampaign: () => void
}) {
  const [selected, setSelected] = useState<number | null>(null)
  const [filter, setFilter] = useState<Filter>("all")
  const [query, setQuery] = useState("")
  const search = useRef<HTMLInputElement>(null)

  const sorted = useMemo(() => [...contacts].sort((a, b) => (b.last_message_at || b.created_at) - (a.last_message_at || a.created_at)), [contacts])
  const visible = sorted.filter(
    (c) => (filter === "all" || c.status === filter) && (!query || `${displayName(c)} ${c.context} ${c.phone}`.toLowerCase().includes(query.toLowerCase())),
  )
  const current = sorted.find((c) => c.id === selected) ?? visible[0] ?? sorted[0]

  // Unread: a lead's newest message arrived after we last looked at that chat.
  const [seen, setSeen] = useState<Record<number, number>>(() => {
    try {
      const saved = JSON.parse(localStorage.getItem("ureach.seen") ?? "null")
      if (saved && typeof saved === "object") return saved
    } catch {
      /* storage unavailable: fall back to "everything so far is seen" */
    }
    return Object.fromEntries(Object.entries(last).map(([id, m]) => [id, m.id]))
  })
  useEffect(() => {
    try {
      localStorage.setItem("ureach.seen", JSON.stringify(seen))
    } catch {
      /* ignore */
    }
  }, [seen])
  const newest = (id: number) => messages[id]?.at(-1) ?? last[id]
  const currentNewest = current ? newest(current.id)?.id : undefined
  useEffect(() => {
    if (current && currentNewest) setSeen((s) => (s[current.id] === currentNewest ? s : { ...s, [current.id]: currentNewest }))
  }, [current, currentNewest])
  const unread = (c: Contact) => {
    const m = newest(c.id)
    return !!m && m.direction === "in" && m.id > (seen[c.id] ?? 0)
  }
  const counts = useMemo(() => {
    const n: Record<string, number> = { all: contacts.length }
    contacts.forEach((c) => (n[c.status] = (n[c.status] ?? 0) + 1))
    return n
  }, [contacts])

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

  // "/" focuses search, like most inboxes.
  useEffect(() => {
    const key = (e: KeyboardEvent) => {
      if (e.key === "/" && !(e.target instanceof HTMLInputElement || e.target instanceof HTMLTextAreaElement)) {
        e.preventDefault()
        search.current?.focus()
      }
    }
    window.addEventListener("keydown", key)
    return () => window.removeEventListener("keydown", key)
  }, [])

  const liveRuns = new Map(runs.filter((r) => r.status === "running").map((r) => [r.contact_id, r]))
  const lastFinished = current && runs.find((r) => r.contact_id === current.id && r.status !== "running")
  const tabs: { id: Filter; label: string }[] = [
    { id: "all", label: "All" },
    { id: "needs_john", label: "Needs John" },
    { id: "auto", label: "Auto" },
    { id: "human", label: "John" },
    { id: "new", label: "New" },
  ]

  return (
    <div className="grid min-h-0 flex-1 grid-cols-[minmax(300px,340px)_1fr] gap-4">
      <div className="flex min-h-0 flex-col overflow-hidden rounded-2xl border bg-card shadow-sm">
        <div className="space-y-2.5 border-b px-3 pt-3 pb-2.5">
          <div className="flex items-center gap-2 rounded-xl bg-muted px-3 py-2 ring-primary/30 focus-within:ring-2">
            <Search className="size-4 text-muted-foreground" />
            <input
              ref={search}
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              onKeyDown={(e) => e.key === "Escape" && (setQuery(""), e.currentTarget.blur())}
              placeholder="Search leads"
              className="w-full bg-transparent text-sm outline-none placeholder:text-muted-foreground"
            />
            <kbd className="rounded border bg-card px-1.5 text-[10px] text-muted-foreground">/</kbd>
          </div>
          <div className="flex flex-wrap gap-1">
            {tabs.map((t) => (
              <button
                key={t.id}
                onClick={() => setFilter(t.id)}
                className={cn(
                  "shrink-0 rounded-full px-2 py-0.5 text-[11px] font-semibold whitespace-nowrap transition",
                  filter === t.id ? "bg-primary text-primary-foreground" : "text-muted-foreground hover:bg-muted",
                  t.id === "needs_john" && filter !== t.id && counts.needs_john && "text-amber-700",
                )}
              >
                {t.label}
                {!!counts[t.id] && <span className={cn("ml-1 tabular-nums", filter === t.id ? "opacity-80" : "opacity-60")}>{counts[t.id]}</span>}
              </button>
            ))}
          </div>
        </div>
        <div className="min-h-0 flex-1 overflow-y-auto">
          {visible.map((c) => (
            <ChatRow key={c.id} c={c} active={current?.id === c.id} unread={current?.id !== c.id && unread(c)} run={liveRuns.get(c.id)} last={newest(c.id)} onClick={() => setSelected(c.id)} />
          ))}
          {!visible.length && <p className="px-4 py-10 text-center text-sm text-muted-foreground">No leads match.</p>}
        </div>
        <SimulateBox />
      </div>
      {current ? (
        <ChatView
          key={current.id}
          contact={current}
          messages={messages[current.id]}
          run={liveRuns.get(current.id)}
          failed={lastFinished?.status === "failed" ? lastFinished : undefined}
          onCampaign={onCampaign}
        />
      ) : (
        <div className="grid place-items-center rounded-2xl border bg-card text-muted-foreground">No chats yet. Text John's number or use Simulate.</div>
      )}
    </div>
  )
}

function ChatRow({ c, active, unread, run, last, onClick }: { c: Contact; active: boolean; unread: boolean; run?: Run; last?: Message; onClick: () => void }) {
  return (
    <button
      onClick={onClick}
      className={cn(
        "relative flex w-full items-center gap-3 border-b border-border/60 px-4 py-3 text-left transition hover:bg-muted/60",
        active && "bg-accent/60 hover:bg-accent/60",
      )}
    >
      {active && <span className="absolute inset-y-2 left-0 w-1 rounded-r-full bg-primary" />}
      <div className="relative">
        <Initials name={displayName(c)} src={c.avatar} size={42} />
        {run && <span className="absolute -right-0.5 -bottom-0.5 size-3.5 animate-pulse rounded-full border-2 border-card bg-wa" />}
      </div>
      <div className="min-w-0 flex-1">
        <div className="flex items-center justify-between gap-2">
          <span className={cn("truncate font-semibold", unread && "font-extrabold")}>{displayName(c)}</span>
          <span className={cn("flex shrink-0 items-center gap-1.5 text-[11px] tabular-nums", unread ? "font-bold text-wa-dark" : "text-muted-foreground")}>
            {timeAgo(c.last_message_at || c.created_at)}
            {unread && <span className="size-2 rounded-full bg-wa" />}
          </span>
        </div>
        <div className="mt-0.5 flex items-center justify-between gap-2">
          <span className="flex min-w-0 items-center gap-1 truncate text-xs text-muted-foreground">
            {run ? (
              <span className="font-medium text-wa-dark">{run.thought_at ? "recording audio…" : "thinking…"}</span>
            ) : last ? (
              <>
                {last.direction === "out" && <Ticks status={last.status} voice={last.kind === "voice"} />}
                <span className="truncate">{preview(last)}</span>
              </>
            ) : (
              <span className="truncate">{c.context || "New lead"}</span>
            )}
          </span>
          <StatusChip status={c.status} />
        </div>
      </div>
    </button>
  )
}

function preview(m: Message) {
  const who = m.direction === "out" ? (m.source === "john" ? "You: " : "") : ""
  if (m.kind === "voice") return `${who}🎤 Voice note${m.duration_ms ? ` (${Math.round(m.duration_ms / 1000)}s)` : ""}`
  return who + m.text.split("\n")[0]
}

function ChatView({ contact, messages, run, failed, onCampaign }: { contact: Contact; messages?: Message[]; run?: Run; failed?: Run; onCampaign: () => void }) {
  const scroller = useRef<HTMLDivElement>(null)
  const composer = useRef<HTMLTextAreaElement>(null)
  const [draft, setDraft] = useState("")
  const [busy, setBusy] = useState(false)
  const first = useRef(true)

  // Jump to the newest message on open; glide for new arrivals.
  useLayoutEffect(() => {
    const el = scroller.current
    if (!el || !messages) return
    el.scrollTo({ top: el.scrollHeight, behavior: first.current ? "auto" : "smooth" })
    first.current = false
  }, [messages?.length, run?.thought_at, messages])

  const paused = contact.status === "human" || contact.status === "needs_john"
  const send = async () => {
    if (!draft.trim()) return
    setBusy(true)
    try {
      await api.send(contact.id, draft.trim())
      setDraft("")
    } catch (e) {
      toast(`Couldn't send: ${e}`, "error")
    } finally {
      setBusy(false)
    }
  }
  const setStatus = async (s: Status) => {
    try {
      await api.setStatus(contact.id, s)
      if (s === "human") window.setTimeout(() => composer.current?.focus(), 50)
    } catch (e) {
      toast(String(e), "error")
    }
  }

  return (
    <div className="flex min-h-0 flex-col overflow-hidden rounded-2xl border bg-card shadow-sm">
      <div className="flex items-center gap-3 border-b px-5 py-3">
        <Initials name={displayName(contact)} src={contact.avatar} size={44} />
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <span className="text-lg font-bold">{displayName(contact)}</span>
            <StatusChip status={contact.status} />
            {contact.link_sent_at > 0 && (
              <span className="inline-flex items-center gap-1 rounded-full bg-secondary px-2 py-0.5 text-[11px] font-semibold whitespace-nowrap text-secondary-foreground">
                <Link2 className="size-3" /> Link sent
              </span>
            )}
          </div>
          <div className="truncate text-xs text-muted-foreground">
            {contact.phone && !contact.phone.startsWith("99") ? `+${contact.phone} · ` : ""}
            {contact.context || (contact.push_name ? `WhatsApp name: ${contact.push_name}` : "New lead")}
          </div>
        </div>
        <Button
          variant="ghost"
          size="icon"
          onClick={async () => {
            if (!window.confirm(`Reset the chat with ${displayName(contact)}? They disappear from the inbox and campaign. If they message again they come back as a new lead (great for re-running the demo).`)) return
            try {
              await api.resetChat(contact.id)
              toast(`Chat with ${displayName(contact)} reset`)
            } catch (e) {
              toast(String(e), "error")
            }
          }}
          className="rounded-full text-muted-foreground"
          title="Reset this chat (removes the lead)"
          aria-label="Reset this chat"
        >
          <RotateCcw />
        </Button>
        {paused ? (
          <Button variant="outline" onClick={() => setStatus("auto")} className="rounded-full">
            <Undo2 /> Hand back to AI
          </Button>
        ) : (
          <Button onClick={() => setStatus("human")} className="rounded-full">
            <Hand /> Take over
          </Button>
        )}
      </div>

      {failed && (
        <div className="flex items-center gap-2 border-b border-red-200 bg-red-50 px-5 py-2 text-sm text-red-800">
          <TriangleAlert className="size-4 shrink-0" />
          <span className="min-w-0 flex-1 truncate">
            <b>Last reply failed</b> ({failed.error || "unknown error"}). The lead got a holding note; take over to answer.
          </span>
        </div>
      )}
      {contact.status === "needs_john" && (
        <div className="flex items-center gap-2 border-b border-amber/40 bg-amber/15 px-5 py-2 text-sm text-amber-900">
          <TriangleAlert className="size-4 shrink-0" />
          <span className="min-w-0 flex-1 truncate">
            <b>John's turn.</b> {contact.handoff_reason || "The AI handed this chat to you."} The AI is paused here.
          </span>
        </div>
      )}
      {!!contact.memory?.length && (
        <div className="flex flex-wrap items-center gap-1.5 border-b bg-muted/40 px-5 py-2 text-xs">
          <span className="font-semibold text-muted-foreground">Remembers:</span>
          {contact.memory.map((m) => (
            <span key={m} className="rounded-full bg-card px-2 py-0.5 text-muted-foreground ring-1 ring-border">
              {m}
            </span>
          ))}
        </div>
      )}

      <div
        ref={scroller}
        className="min-h-0 flex-1 space-y-2 overflow-y-auto bg-[radial-gradient(circle_at_1px_1px,oklch(0.9_0.02_285)_1px,transparent_0)] bg-[length:22px_22px] px-6 py-5"
      >
        {!messages ? (
          <div className="space-y-3">
            {[60, 40, 70].map((w, i) => (
              <div key={i} className={cn("h-12 animate-pulse rounded-2xl bg-muted", i % 2 ? "ml-auto" : "")} style={{ width: `${w}%` }} />
            ))}
          </div>
        ) : messages.length === 0 && !run ? (
          <div className="grid h-full place-items-center text-center text-sm text-muted-foreground">
            <div>
              <Megaphone className="mx-auto mb-2 size-7 text-primary/60" />
              No messages with {displayName(contact)} yet.
              <br />
              <button onClick={onCampaign} className="mt-2 font-semibold text-primary hover:underline">
                Send them a personal voice note from John →
              </button>
            </div>
          </div>
        ) : (
          messages.map((m, i) => (
            <Fragment key={m.id}>
              {(i === 0 || dayLabel(messages[i - 1].created_at) !== dayLabel(m.created_at)) && (
                <div className="sticky top-0 z-10 flex justify-center py-1">
                  <span className="rounded-full bg-card/90 px-3 py-0.5 text-[11px] font-semibold text-muted-foreground shadow-sm ring-1 ring-border backdrop-blur">{dayLabel(m.created_at)}</span>
                </div>
              )}
              <Bubble m={m} />
            </Fragment>
          ))
        )}
        {run && (
          <div className="flex justify-end">
            <div className="inline-flex items-center gap-2 rounded-2xl bg-wa-bubble px-3 py-2 text-sm font-medium text-wa-dark shadow-sm">
              {run.thought_at ? (
                <>
                  <Mic className="size-4 animate-pulse" /> John is recording audio…
                </>
              ) : (
                <>
                  <Sparkles className="size-4 animate-pulse" /> John is thinking…
                </>
              )}
            </div>
          </div>
        )}
      </div>

      {paused ? (
        <div className="flex items-end gap-2 border-t p-3">
          <Textarea
            ref={composer}
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && !e.shiftKey) {
                e.preventDefault()
                send()
              }
            }}
            placeholder={`Reply to ${displayName(contact)} as John… (Enter to send)`}
            className="max-h-32 min-h-11 resize-none rounded-xl"
            rows={1}
          />
          <Button onClick={send} disabled={busy || !draft.trim()} className="h-11 rounded-xl bg-wa-dark hover:bg-wa-dark/90" aria-label="Send">
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
          <div className="text-sm leading-snug whitespace-pre-wrap [overflow-wrap:anywhere]">
            <Linkified text={m.text.trim()} />
          </div>
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
          {out && <Ticks status={m.status} voice={m.kind === "voice"} />}
          {out && m.kind === "voice" && m.status === "played" && <span className="font-semibold text-sky-600">played</span>}
        </div>
      </div>
    </div>
  )
}
