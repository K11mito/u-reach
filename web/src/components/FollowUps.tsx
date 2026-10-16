import { useCallback, useEffect, useRef, useState } from "react"
import { Check, Clock, Headphones, Link2, Loader2, MessageCircleReply, RefreshCw, Send, Sparkles, TriangleAlert, X } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"
import { cn } from "@/lib/utils"
import { errText, nudgeApi, timeAgo, type Nudge, type NudgeQueue } from "@/lib/api"
import { Heat, Initials, VoiceNote } from "./bits"
import { toast } from "./Toast"

/** The follow-up queue, live: refetched whenever the server says it changed. */
export function useNudges() {
  const [queue, setQueue] = useState<NudgeQueue | null>(null)
  const timer = useRef<number | undefined>(undefined)
  const load = useCallback(() => {
    window.clearTimeout(timer.current)
    timer.current = window.setTimeout(() => nudgeApi.queue().then(setQueue).catch(() => {}), 150)
  }, [])
  useEffect(() => {
    load()
    window.addEventListener("ureach:nudges", load)
    return () => {
      window.removeEventListener("ureach:nudges", load)
      window.clearTimeout(timer.current)
    }
  }, [load])
  return { queue, reload: load }
}

const pending = (n: Nudge) => n.status === "draft" || n.status === "failed" || n.status === "sending"

/** "3 days", "20 hours": how long they've been quiet. */
function quietFor(ms: number) {
  const h = Math.max(0, (Date.now() - ms) / 3_600_000)
  if (h < 48) return `${Math.round(h)} hours`
  return `${Math.floor(h / 24)} days`
}

/** Follow-ups for leads who went quiet, drafted by the AI and sent only with John's OK. */
export function FollowUps({ queue, reload }: { queue: NudgeQueue | null; reload: () => void }) {
  const [drafting, setDrafting] = useState(false)
  const [edits, setEdits] = useState<Record<number, { script: string; followup: string }>>({})
  const items = queue?.items ?? []
  const waiting = items.filter(pending)
  const done = items.filter((n) => !pending(n))
  const ready = waiting.filter((n) => n.status === "draft" && n.script && !edits[n.id])
  const days = Math.max(1, Math.round((queue?.after_hours ?? 48) / 24))

  const draftNow = async () => {
    setDrafting(true)
    try {
      const { drafted } = await nudgeApi.draft()
      toast(drafted ? `Drafted ${drafted} follow-up${drafted === 1 ? "" : "s"}` : "No one new has gone quiet")
    } catch (e) {
      toast(errText(e), "error")
    } finally {
      setDrafting(false)
      reload()
    }
  }
  const sendAll = async () => {
    if (!window.confirm(`Send ${ready.length} follow-up${ready.length === 1 ? "" : "s"} in John's voice? They go out a few seconds apart.`)) return
    try {
      await nudgeApi.send(ready.map((n) => n.id))
    } catch (e) {
      toast(errText(e), "error")
    }
    reload()
  }

  return (
    <div className="flex min-h-0 flex-1 flex-col overflow-hidden rounded-xl border bg-card">
      <div className="flex items-center gap-4 border-b px-5 py-3">
        <div className="min-w-0 flex-1">
          <h2 className="text-[15px] font-semibold tracking-tight">Follow-ups for quiet leads</h2>
          <p className="text-xs text-muted-foreground">
            When someone hasn't answered John for {days} day{days === 1 ? "" : "s"}, the AI drafts a short personal follow-up here. Nothing goes out until you approve it.
          </p>
        </div>
        <Button variant="outline" onClick={draftNow} disabled={drafting} title="Draft follow-ups for anyone who has gone quiet, without waiting for the next automatic check">
          {drafting ? <Loader2 className="animate-spin" /> : <Sparkles />}
          {drafting ? "Drafting…" : queue?.waiting ? `Draft ${queue.waiting} now` : "Check now"}
        </Button>
        <Button onClick={sendAll} disabled={!ready.length} className="bg-wa-dark hover:bg-wa-dark/90">
          <Send /> Approve and send {ready.length || ""}
        </Button>
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto p-5">
        {!queue ? (
          <div className="grid grid-cols-[repeat(auto-fill,minmax(460px,1fr))] gap-4">
            {[0, 1].map((i) => (
              <div key={i} className="h-56 animate-pulse rounded-xl bg-muted" />
            ))}
          </div>
        ) : !waiting.length ? (
          <div className="grid h-full min-h-48 place-items-center text-center text-sm text-muted-foreground">
            <div className="max-w-md">
              <MessageCircleReply className="mx-auto mb-2 size-7 text-muted-foreground/70" />
              {queue.waiting
                ? `${queue.waiting} lead${queue.waiting === 1 ? " has" : "s have"} gone quiet. Draft their follow-ups now, or they appear here at the next check.`
                : `Nobody's waiting on a follow-up. When a lead stops replying for ${days} day${days === 1 ? "" : "s"}, a personal nudge shows up here for John to approve.`}
            </div>
          </div>
        ) : (
          <div className="grid grid-cols-[repeat(auto-fill,minmax(460px,1fr))] items-start gap-4">
            {waiting.map((n) => (
              <NudgeCard
                key={n.id}
                n={n}
                edit={edits[n.id]}
                setEdit={(e) =>
                  setEdits((x) => {
                    const next = { ...x }
                    if (e) next[n.id] = e
                    else delete next[n.id]
                    return next
                  })
                }
                reload={reload}
              />
            ))}
          </div>
        )}

        {!!done.length && (
          <div className="mt-6">
            <h3 className="mb-2 text-xs font-semibold tracking-wide text-muted-foreground uppercase">Last two days</h3>
            <div className="divide-y rounded-lg border">
              {done.map((n) => (
                <Outcome key={n.id} n={n} />
              ))}
            </div>
          </div>
        )}
      </div>
    </div>
  )
}

function NudgeCard({
  n,
  edit,
  setEdit,
  reload,
}: {
  n: Nudge
  edit?: { script: string; followup: string }
  setEdit: (e?: { script: string; followup: string }) => void
  reload: () => void
}) {
  const [busy, setBusy] = useState<"" | "listen" | "redraft" | "send" | "skip" | "save">("")
  const script = edit?.script ?? n.script
  const followup = edit?.followup ?? n.followup
  const sending = n.status === "sending"
  const locked = sending || !!busy

  // Every action saves unsaved words first, so what John hears is what goes out.
  const act = async (what: typeof busy, fn: () => Promise<unknown>) => {
    setBusy(what)
    try {
      if (edit && what !== "redraft" && what !== "skip") {
        await nudgeApi.edit(n.id, edit.script, edit.followup)
        setEdit(undefined)
      }
      await fn()
    } catch (e) {
      toast(errText(e), "error")
    } finally {
      setBusy("")
      reload()
    }
  }

  return (
    <div className={cn("rounded-xl border bg-card p-4 shadow-xs transition", sending && "opacity-70")}>
      <div className="flex items-center gap-3">
        <Initials name={n.name} src={n.avatar} size={40} />
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-1.5">
            <span className="truncate text-[15px] font-semibold">{n.name}</span>
            <Heat level={n.interest} reason={n.interest_reason} />
          </div>
          <div className="flex items-center gap-1 text-xs text-muted-foreground">
            <Clock className="size-3" /> Quiet for {quietFor(n.quiet_since)}
            <span className="mx-1">·</span>
            {n.demo ? <span title="A made-up number: the follow-up is added to the chat here, never sent on WhatsApp">Demo lead, stays in the dashboard</span> : <span>Sends on WhatsApp</span>}
          </div>
        </div>
        <Button variant="ghost" size="icon" disabled={locked} onClick={() => act("skip", () => nudgeApi.skip(n.id))} title="Skip: not now. They're offered again only after they write back." aria-label="Skip">
          {busy === "skip" ? <Loader2 className="animate-spin" /> : <X />}
        </Button>
      </div>

      <p className="mt-3 line-clamp-2 rounded-lg bg-muted/60 px-3 py-2 text-[13px] text-muted-foreground">
        {n.last_lead_text ? (
          <>
            <span className="font-medium text-foreground/70">They last said:</span> “{n.last_lead_text}”
          </>
        ) : (
          "They haven't replied to John's first message yet."
        )}
      </p>

      {n.status === "failed" && n.error && (
        <div className="mt-3 flex items-start gap-2 rounded-lg border border-amber-300 bg-amber-50 px-3 py-2 text-[13px] text-amber-900">
          <TriangleAlert className="mt-0.5 size-4 shrink-0" /> {n.error}
        </div>
      )}

      <div className="mt-3 space-y-2">
        <Textarea
          value={script}
          disabled={locked}
          onChange={(e) => setEdit({ script: e.target.value, followup })}
          placeholder="What John says in the voice note"
          className="min-h-24 text-sm leading-relaxed"
        />
        <div className="flex items-center gap-2">
          <Input value={followup} disabled={locked} onChange={(e) => setEdit({ script, followup: e.target.value })} placeholder="Optional text after the voice note" className="h-8 text-sm" />
          {n.link && !n.link_sent && (
            <span className="inline-flex shrink-0 items-center gap-1 rounded-md bg-secondary px-2 py-1 text-xs text-foreground/75" title="They never got John's link, so it's added after the note">
              <Link2 className="size-3" /> + link
            </span>
          )}
        </div>
        {n.audio_path && !edit && (
          <div className="inline-block rounded-xl rounded-br-sm bg-wa-bubble px-3 py-2 ring-1 ring-wa/15">
            <VoiceNote src={n.audio_path} durationMs={n.duration_ms} seed={n.id} outgoing />
          </div>
        )}
      </div>

      <div className="mt-3 flex items-center gap-2">
        {sending ? (
          <span className="flex items-center gap-1.5 text-sm font-medium text-wa-dark">
            <Loader2 className="size-4 animate-spin" /> Recording and sending…
          </span>
        ) : (
          <>
            {!(n.audio_path && !edit) && (
              <Button variant="outline" size="sm" disabled={locked || !script.trim()} onClick={() => act("listen", () => nudgeApi.listen(n.id))} title="Record it in John's voice to hear it first">
                {busy === "listen" ? <Loader2 className="animate-spin" /> : <Headphones />} Listen
              </Button>
            )}
            <Button variant="ghost" size="sm" disabled={locked} onClick={() => act("redraft", () => nudgeApi.redraft(n.id).then(() => setEdit(undefined)))} title="Ask the AI for a different follow-up">
              {busy === "redraft" ? <Loader2 className="animate-spin" /> : <RefreshCw />} Redraft
            </Button>
            {edit && (
              <Button variant="ghost" size="sm" disabled={locked} onClick={() => act("save", async () => {})}>
                {busy === "save" ? <Loader2 className="animate-spin" /> : <Check />} Save
              </Button>
            )}
            <Button size="sm" disabled={locked || !script.trim()} onClick={() => act("send", () => nudgeApi.send([n.id]))} className="ml-auto bg-wa-dark hover:bg-wa-dark/90">
              {busy === "send" ? <Loader2 className="animate-spin" /> : <Send />} {n.status === "failed" && n.script ? "Try again" : "Approve and send"}
            </Button>
          </>
        )}
      </div>
    </div>
  )
}

const OUTCOME: Record<string, { label: string; className: string }> = {
  sent: { label: "Sent", className: "text-wa-dark" },
  skipped: { label: "Skipped", className: "text-muted-foreground" },
  stale: { label: "Not needed", className: "text-muted-foreground" },
}

function Outcome({ n }: { n: Nudge }) {
  const o = OUTCOME[n.status] ?? OUTCOME.skipped
  return (
    <div className="flex items-center gap-3 px-3 py-2 text-sm">
      <Initials name={n.name} src={n.avatar} size={26} />
      <span className="w-36 shrink-0 truncate font-medium">{n.name}</span>
      <span className={cn("w-24 shrink-0 text-xs font-semibold", o.className)}>
        {n.status === "sent" && <Check className="mr-1 inline size-3.5" />}
        {o.label}
      </span>
      <span className="min-w-0 flex-1 truncate text-xs text-muted-foreground">
        {n.status === "sent" ? (n.demo ? "Added to the chat (demo lead)" : `“${n.script}”`) : n.status === "stale" ? sentence(n.error || "the chat moved on") : "Offered again if they write back and go quiet"}
      </span>
      <span className="shrink-0 text-xs text-muted-foreground tabular-nums">{timeAgo(n.sent_at || n.updated_at)}</span>
    </div>
  )
}

const sentence = (s: string) => {
  const t = s.trim()
  return t.charAt(0).toUpperCase() + t.slice(1)
}
