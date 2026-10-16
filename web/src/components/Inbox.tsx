import { Fragment, useEffect, useLayoutEffect, useMemo, useRef, useState, type ReactNode } from "react"
import { Bot, Flame, Hand, Link2, Megaphone, Mic, Reply, RotateCcw, Search, Send, Sparkles, TriangleAlert, Undo2, UserRound, X } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Textarea } from "@/components/ui/textarea"
import { cn } from "@/lib/utils"
import { api, clock, dayLabel, displayName, errText, explainError, timeAgo, type Contact, type Message, type Run, type Status } from "@/lib/api"
import { fmtDur, Heat, HeatPill, Initials, Linkified, StatusChip, Ticks, VoiceNote } from "./bits"
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
  // Only hot leads: the people to call today. /?ready=1 opens with it on.
  const [ready, setReady] = useState(() => new URLSearchParams(window.location.search).has("ready"))
  const [query, setQuery] = useState("")
  const search = useRef<HTMLInputElement>(null)

  const sorted = useMemo(() => [...contacts].sort((a, b) => (b.last_message_at || b.created_at) - (a.last_message_at || a.created_at)), [contacts])
  const visible = sorted.filter(
    (c) =>
      (filter === "all" || c.status === filter) &&
      (!ready || c.interest === "hot") &&
      (!query || `${displayName(c)} ${c.context} ${c.phone} ${c.interest_reason ?? ""}`.toLowerCase().includes(query.toLowerCase())),
  )
  const hot = contacts.filter((c) => c.interest === "hot").length
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
      <div className="flex min-h-0 flex-col overflow-hidden rounded-xl border bg-card">
        <div className="space-y-2.5 border-b p-3">
          <div className="flex h-8 items-center gap-2 rounded-md border bg-background px-2.5 transition focus-within:border-ring focus-within:ring-[3px] focus-within:ring-ring/20">
            <Search className="size-3.5 text-muted-foreground" />
            <input
              ref={search}
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              onKeyDown={(e) => e.key === "Escape" && (setQuery(""), e.currentTarget.blur())}
              placeholder="Search leads"
              className="w-full bg-transparent text-sm outline-none placeholder:text-muted-foreground"
            />
            <kbd className="rounded border bg-card px-1.5 font-mono text-[10px] text-muted-foreground">/</kbd>
          </div>
          <div className="flex rounded-md bg-muted p-0.5">
            {tabs.map((t) => (
              <button
                key={t.id}
                onClick={() => setFilter(t.id)}
                className={cn(
                  "flex flex-auto items-center justify-center gap-1 rounded-[5px] px-1.5 py-1 text-[11px] font-medium whitespace-nowrap transition",
                  filter === t.id ? "bg-card text-foreground shadow-xs ring-1 ring-black/5" : "text-muted-foreground hover:text-foreground",
                  t.id === "needs_john" && filter !== t.id && counts.needs_john && "text-amber-700",
                )}
              >
                {t.label}
                {!!counts[t.id] && <span className="tabular-nums opacity-55">{counts[t.id]}</span>}
              </button>
            ))}
          </div>
          <button
            onClick={() => setReady((r) => !r)}
            aria-pressed={ready}
            title="Leads the AI rates hot: they asked how to pay or join, or said they're signing up"
            className={cn(
              "flex h-8 w-full items-center gap-2 rounded-md border px-2.5 text-xs font-medium transition",
              ready ? "border-orange-300 bg-orange-50 text-orange-900 shadow-xs" : "border-dashed text-muted-foreground hover:border-solid hover:bg-muted/50 hover:text-foreground",
            )}
          >
            <Flame className={cn("size-3.5", ready || hot ? "fill-orange-400 text-red-500" : "")} />
            Ready to call
            <span className="tabular-nums opacity-60">{hot}</span>
            <span className="ml-auto text-[11px] opacity-70">{ready ? <X className="size-3.5" /> : "hot leads only"}</span>
          </button>
        </div>
        <div className="min-h-0 flex-1 overflow-y-auto">
          {visible.map((c) => (
            <ChatRow key={c.id} c={c} active={current?.id === c.id} unread={current?.id !== c.id && unread(c)} run={liveRuns.get(c.id)} last={newest(c.id)} onClick={() => setSelected(c.id)} />
          ))}
          {!visible.length && (
            <p className="px-4 py-10 text-center text-sm text-muted-foreground">{ready ? "No hot leads right now. The AI flags them as they come in." : "No leads match."}</p>
          )}
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
        <div className="grid place-items-center rounded-xl border bg-card text-sm text-muted-foreground">No chats yet. Text John's number or use Simulate.</div>
      )}
    </div>
  )
}

function ChatRow({ c, active, unread, run, last, onClick }: { c: Contact; active: boolean; unread: boolean; run?: Run; last?: Message; onClick: () => void }) {
  return (
    <button
      onClick={onClick}
      className={cn(
        "relative flex w-full items-center gap-3 border-b border-border/60 px-3.5 py-2.5 text-left transition hover:bg-muted/50",
        active && "bg-muted hover:bg-muted",
      )}
    >
      <div className="relative">
        <Initials name={displayName(c)} src={c.avatar} size={38} />
        {run && <span className="absolute -right-0.5 -bottom-0.5 size-3 animate-pulse rounded-full border-2 border-card bg-wa" />}
      </div>
      <div className="min-w-0 flex-1">
        <div className="flex items-center justify-between gap-2">
          <span className="flex min-w-0 items-center gap-1.5">
            <span className={cn("truncate text-sm font-medium", unread && "font-semibold")}>{displayName(c)}</span>
            <Heat level={c.interest} reason={c.interest_reason} />
          </span>
          <span className={cn("flex shrink-0 items-center gap-1.5 text-[11px] tabular-nums", unread ? "font-semibold text-wa-dark" : "text-muted-foreground")}>
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
                <Preview m={last} />
              </>
            ) : (
              <span className="truncate">{c.context || "New lead"}</span>
            )}
          </span>
          <StatusChip status={c.status} bare />
        </div>
      </div>
    </button>
  )
}

function Preview({ m }: { m: Message }) {
  const who = m.direction === "out" && m.source === "john" ? "You: " : ""
  if (m.kind === "voice")
    return (
      <span className="flex min-w-0 items-center gap-1 truncate">
        {who}
        <Mic className="size-3 shrink-0" /> Voice note{m.duration_ms ? ` · ${fmtDur(m.duration_ms)}` : ""}
      </span>
    )
  return <span className="truncate">{who + m.text.split("\n")[0].replace(/https?:\/\//g, "")}</span>
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
      toast(`Couldn't send: ${errText(e)}`, "error")
    } finally {
      setBusy(false)
    }
  }
  const setStatus = async (s: Status) => {
    try {
      await api.setStatus(contact.id, s)
      if (s === "human") window.setTimeout(() => composer.current?.focus(), 50)
    } catch (e) {
      toast(errText(e), "error")
    }
  }

  return (
    <div className="flex min-h-0 flex-col overflow-hidden rounded-xl border bg-card">
      <div className="flex h-[68px] shrink-0 items-center gap-3.5 border-b px-5">
        <Initials name={displayName(contact)} src={contact.avatar} size={46} />
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2.5">
            <span className="truncate text-[17px] font-semibold tracking-tight">{displayName(contact)}</span>
            <HeatPill level={contact.interest} reason={contact.interest_reason} className="max-w-[340px]" />
            {contact.status === "needs_john" ? (
              <span className="inline-flex shrink-0 items-center gap-1.5 rounded-full bg-amber-100 px-2.5 py-1 text-xs font-semibold text-amber-800 ring-1 ring-amber-300">
                <TriangleAlert className="size-3.5" /> Needs John
              </span>
            ) : (
              <StatusChip status={contact.status} className="text-xs" />
            )}
          </div>
          <div className="mt-0.5 flex min-w-0 items-center gap-1.5 text-[13px] text-muted-foreground">
            <span className="truncate">
              {contact.phone && !contact.phone.startsWith("99") ? `+${contact.phone} · ` : ""}
              {contact.context || (contact.push_name ? `WhatsApp name: ${contact.push_name}` : "New lead")}
            </span>
            {contact.link_sent_at > 0 && (
              <span className="inline-flex shrink-0 items-center gap-1 whitespace-nowrap">
                · <Link2 className="size-3.5" /> Link sent
              </span>
            )}
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
              toast(errText(e), "error")
            }
          }}
          className="text-muted-foreground"
          title="Reset this chat (removes the lead)"
          aria-label="Reset this chat"
        >
          <RotateCcw />
        </Button>
        {paused ? (
          <Button onClick={() => setStatus("auto")} className="h-10 bg-indigo-600 px-4 text-white shadow-sm shadow-indigo-500/25 hover:bg-indigo-700">
            <Undo2 /> Hand back to AI
          </Button>
        ) : (
          <Button onClick={() => setStatus("human")} className="h-10 px-4">
            <Hand /> Take over
          </Button>
        )}
      </div>

      {(failed || contact.status === "needs_john") && (
        <div className="space-y-2 border-b px-5 py-3">
          {failed && (
            <Notice
              tone="red"
              title="The last reply failed"
              body={`${explainError(failed.error)} ${failed.error?.startsWith("brain:") ? "The lead got a holding note." : "Nothing reached the lead."}`}
              detail={failed.error}
            >
              {!paused && (
                <Button size="sm" onClick={() => setStatus("human")} className="bg-red-600 text-white hover:bg-red-700">
                  <Hand /> Take over
                </Button>
              )}
            </Notice>
          )}
          {contact.status === "needs_john" && (
            <Notice
              tone="amber"
              title="John's turn: the AI is paused in this chat"
              body={
                contact.handoff_reason.startsWith("assistant unavailable")
                  ? "The AI couldn't reply this time. Answer as John, or hand back to the AI once it's working and it answers them straight away."
                  : sentence(contact.handoff_reason || "The AI handed this chat to you.")
              }
            >
              <Button size="sm" onClick={() => composer.current?.focus()} className="bg-amber-600 text-white hover:bg-amber-700">
                <Reply /> Reply as John
              </Button>
            </Notice>
          )}
        </div>
      )}
      {!!contact.memory?.length && (
        <div className="flex flex-wrap items-center gap-1.5 border-b px-4 py-2 text-xs">
          <span className="font-medium text-muted-foreground">Remembers</span>
          {contact.memory.map((m) => (
            <span key={m} className="rounded-md bg-secondary px-2 py-0.5 text-foreground/75">
              {m}
            </span>
          ))}
        </div>
      )}

      <div
        ref={scroller}
        className="min-h-0 flex-1 overflow-y-auto bg-[radial-gradient(circle_at_1px_1px,oklch(0.9_0.02_285)_1px,transparent_0)] bg-[length:22px_22px] px-6 py-5"
      >
        <div className="mx-auto w-full max-w-[960px] space-y-2.5">
          {!messages ? (
            <div className="space-y-3">
              {[60, 40, 70].map((w, i) => (
                <div key={i} className={cn("h-12 animate-pulse rounded-xl bg-muted", i % 2 ? "ml-auto" : "")} style={{ width: `${w}%` }} />
              ))}
            </div>
          ) : messages.length === 0 && !run ? (
            <div className="grid h-full place-items-center text-center text-sm text-muted-foreground">
              <div>
                <Megaphone className="mx-auto mb-2 size-6 text-muted-foreground/70" />
                No messages with {displayName(contact)} yet.
                <br />
                <button onClick={onCampaign} className="mt-2 font-medium text-foreground underline-offset-4 hover:underline">
                  Send them a personal voice note from John →
                </button>
              </div>
            </div>
          ) : (
            messages.map((m, i) => (
              <Fragment key={m.id}>
                {(i === 0 || dayLabel(messages[i - 1].created_at) !== dayLabel(m.created_at)) && (
                  <div className="flex justify-center py-1.5">
                    <span className="rounded-md border bg-card px-2.5 py-0.5 text-xs font-medium text-muted-foreground">{dayLabel(m.created_at)}</span>
                  </div>
                )}
                <Bubble m={m} />
              </Fragment>
            ))
          )}
          {run && (
            <div className="flex justify-end">
              <div className="inline-flex items-center gap-2 rounded-xl bg-wa-bubble px-3 py-2 text-sm font-medium text-wa-dark ring-1 ring-wa/15">
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
      </div>

      {paused ? (
        <div className="border-t p-3">
          <div className="mx-auto flex w-full max-w-[960px] items-end gap-2">
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
              className="max-h-32 min-h-10 resize-none"
              rows={1}
            />
            <Button onClick={send} disabled={busy || !draft.trim()} className="size-10" aria-label="Send">
              <Send />
            </Button>
          </div>
        </div>
      ) : (
        <div className="flex items-center gap-2 border-t px-4 py-2.5 text-xs text-muted-foreground">
          <span className="relative flex size-1.5">
            <span className="absolute inset-0 animate-ping rounded-full bg-wa opacity-60" />
            <span className="size-1.5 rounded-full bg-wa" />
          </span>
          John's AI voice is answering this chat. Take over anytime.
        </div>
      )}
    </div>
  )
}

/** Ends a reason with a full stop, so it reads as a sentence. */
const sentence = (s: string) => (/[.!?]$/.test(s.trim()) ? s.trim() : `${s.trim()}.`)


/** A compact card for a state John has to act on. */
function Notice({ tone, title, body, detail, children }: { tone: "amber" | "red"; title: string; body: string; detail?: string; children?: ReactNode }) {
  return (
    <div className={cn("flex items-center gap-3 rounded-xl border p-3 shadow-sm", tone === "amber" ? "border-amber-300 bg-amber-50" : "border-red-200 bg-red-50")}>
      <div className={cn("grid size-9 shrink-0 place-items-center rounded-lg", tone === "amber" ? "bg-amber-100 text-amber-700" : "bg-red-100 text-red-700")}>
        <TriangleAlert className="size-5" />
      </div>
      <div className="min-w-0 flex-1">
        <div className={cn("text-sm font-semibold", tone === "amber" ? "text-amber-950" : "text-red-950")}>{title}</div>
        <div className={cn("text-[13px] leading-snug", tone === "amber" ? "text-amber-900" : "text-red-900")}>{body}</div>
        {detail && (
          <div className="mt-0.5 truncate font-mono text-[11px] text-red-900/55" title={detail}>
            {detail}
          </div>
        )}
      </div>
      {children && <div className="shrink-0">{children}</div>}
    </div>
  )
}

function Bubble({ m }: { m: Message }) {
  const out = m.direction === "out"
  const ai = m.source === "ai" || m.source === "seed"
  return (
    <div className={cn("flex", out ? "justify-end" : "justify-start")}>
      <div className={cn("max-w-[65%] rounded-xl px-4 py-2.5", out ? "rounded-br-sm bg-wa-bubble ring-1 ring-wa/15" : "rounded-bl-sm bg-card ring-1 ring-border")}>
        {m.kind === "voice" ? (
          <div className="space-y-1.5 py-1">
            <VoiceNote src={m.audio_path} durationMs={m.duration_ms} seed={m.id} outgoing={out} />
            {m.text && <p className="border-t border-foreground/10 pt-2 text-[14px] leading-[1.45] text-foreground/80 italic">“{m.text}”</p>}
          </div>
        ) : (
          <div className="text-[14px] leading-[1.45] whitespace-pre-wrap [overflow-wrap:anywhere]">
            <Linkified text={m.text.trim()} />
          </div>
        )}
        <div className="mt-1 flex items-center justify-end gap-1.5 text-[11px] text-muted-foreground">
          {out && ai && (
            <span className="inline-flex items-center gap-0.5 font-medium text-foreground/60">
              <Bot className="size-3" /> AI
            </span>
          )}
          {out && m.source === "john" && (
            <span className="inline-flex items-center gap-0.5 font-medium text-foreground/60">
              <UserRound className="size-3" /> John
            </span>
          )}
          {clock(m.created_at)}
          {out && <Ticks status={m.status} voice={m.kind === "voice"} />}
          {out && m.kind === "voice" && m.status === "played" && <span className="font-medium text-sky-600">played</span>}
        </div>
      </div>
    </div>
  )
}
