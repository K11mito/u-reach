import { useState } from "react"
import { Check, Loader2, Megaphone, Plus, Send, Sparkles, X } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"
import { cn } from "@/lib/utils"
import { api, displayName, type CampaignStatus, type Contact, type Draft } from "@/lib/api"
import { Initials, StatusChip, VoiceNote } from "./bits"

/** Outbound: personal voice-note openers in John's voice, approved before sending. */
export function Campaign({ contacts, statuses }: { contacts: Contact[]; statuses: Record<number, CampaignStatus> }) {
  const [chosen, setPicked] = useState<number[]>([])
  const picked = chosen.filter((id) => contacts.some((c) => c.id === id)) // a reset chat drops out
  const [drafts, setDrafts] = useState<Draft[]>([])
  const [loading, setLoading] = useState(false)
  const [sent, setSent] = useState(false)
  const [edited, setEdited] = useState<Record<number, boolean>>({})

  const toggle = (id: number) => setPicked((p) => (p.includes(id) ? p.filter((x) => x !== id) : [...p, id]))
  const write = async () => {
    setLoading(true)
    setSent(false)
    setEdited({})
    setDrafts(picked.map((id) => ({ contact_id: id, name: displayName(contacts.find((c) => c.id === id)), script: "", followup: "", audio_path: "", duration_ms: 0 })))
    try {
      setDrafts(await api.draft(picked))
    } finally {
      setLoading(false)
    }
  }
  const send = async () => {
    await api.sendCampaign(drafts.filter((d) => !d.error && d.script))
    setSent(true)
  }
  const update = (id: number, patch: Partial<Draft>) => setDrafts((ds) => ds.map((d) => (d.contact_id === id ? { ...d, ...patch } : d)))
  const ordered = [...contacts].sort((a, b) => Number(b.status === "new") - Number(a.status === "new"))

  return (
    <div className="grid min-h-0 flex-1 grid-cols-[360px_1fr] gap-4">
      <div className="flex min-h-0 flex-col overflow-hidden rounded-xl border bg-card">
        <div className="border-b px-4 py-3">
          <h2 className="text-[15px] font-semibold tracking-tight">Who should John reach out to?</h2>
          <p className="text-xs text-muted-foreground">Each gets a personal voice note in his voice, then his link.</p>
        </div>
        <AddContact />
        <div className="min-h-0 flex-1 overflow-y-auto">
          {ordered.map((c) => (
            <label key={c.id} className={cn("flex cursor-pointer items-center gap-3 border-b border-border/60 px-4 py-2.5 hover:bg-muted/50", picked.includes(c.id) && "bg-muted")}>
              <input type="checkbox" checked={picked.includes(c.id)} onChange={() => toggle(c.id)} className="size-4 accent-[var(--primary)]" />
              <Initials name={displayName(c)} src={c.avatar} size={34} />
              <div className="min-w-0 flex-1">
                <div className="truncate text-sm font-medium">{displayName(c)}</div>
                <div className="truncate text-xs text-muted-foreground">{c.context || `+${c.phone}`}</div>
              </div>
              <StatusChip status={c.status} bare />
            </label>
          ))}
        </div>
        <div className="border-t p-3">
          <Button onClick={write} disabled={!picked.length || loading} className="h-10 w-full">
            {loading ? <Loader2 className="animate-spin" /> : <Sparkles />} Write {picked.length || ""} personal opener{picked.length === 1 ? "" : "s"}
          </Button>
        </div>
      </div>

      <div className="flex min-h-0 flex-col overflow-hidden rounded-xl border bg-card">
        <div className="flex items-center justify-between border-b px-5 py-3">
          <div>
            <h2 className="text-[15px] font-semibold tracking-tight">Openers to approve</h2>
            <p className="text-xs text-muted-foreground">Listen, tweak the words if you like, then send. They go out a few seconds apart.</p>
          </div>
          <Button onClick={send} disabled={!drafts.length || loading || sent || drafts.every((d) => !!d.error)} className="bg-wa-dark hover:bg-wa-dark/90">
            <Send /> {sent ? "Sending…" : `Send ${drafts.filter((d) => !d.error).length || ""}`}
          </Button>
        </div>
        <div className="min-h-0 flex-1 space-y-3 overflow-y-auto p-5">
          {!drafts.length && (
            <div className="grid h-full place-items-center text-center text-muted-foreground">
              <div>
                <Megaphone className="mx-auto mb-2 size-6 text-muted-foreground/70" />
                Pick a few people on the left and John's AI writes each one a personal voice note.
              </div>
            </div>
          )}
          {drafts.map((d) => {
            const st = statuses[d.contact_id]
            return (
              <div key={d.contact_id} className="rounded-lg border p-4">
                <div className="mb-3 flex items-center gap-3">
                  <Initials name={d.name} size={36} />
                  <div className="flex-1 text-sm font-medium">{d.name}</div>
                  {sent && <SendState st={st} />}
                  {!sent && (
                    <button onClick={() => setDrafts((ds) => ds.filter((x) => x.contact_id !== d.contact_id))} className="text-muted-foreground hover:text-foreground" title="Remove">
                      <X className="size-4" />
                    </button>
                  )}
                </div>
                {d.error ? (
                  <p className="text-sm text-destructive">{d.error}</p>
                ) : !d.script ? (
                  <div className="flex items-center gap-2 text-sm text-muted-foreground">
                    <Loader2 className="size-4 animate-spin" /> Writing and recording in John's voice…
                  </div>
                ) : (
                  <div className="space-y-2">
                    <div className="inline-block rounded-xl rounded-br-sm bg-wa-bubble px-3 py-2 ring-1 ring-wa/15">
                      <VoiceNote src={edited[d.contact_id] ? undefined : d.audio_path} durationMs={d.duration_ms} seed={d.contact_id} outgoing />
                    </div>
                    {edited[d.contact_id] && <p className="text-xs text-amber-700">Edited: John's voice re-records it when you send.</p>}
                    <Textarea
                      value={d.script}
                      disabled={sent}
                      onChange={(e) => {
                        update(d.contact_id, { script: e.target.value })
                        setEdited((x) => ({ ...x, [d.contact_id]: true }))
                      }}
                      className="min-h-20 text-sm"
                    />
                    <Input value={d.followup} disabled={sent} onChange={(e) => update(d.contact_id, { followup: e.target.value })} placeholder="Text after the note (link is added automatically)" className="text-sm" />
                  </div>
                )}
              </div>
            )
          })}
        </div>
      </div>
    </div>
  )
}

function SendState({ st }: { st?: CampaignStatus }) {
  if (!st) return <span className="text-xs text-muted-foreground">queued</span>
  if (st.status === "sending")
    return (
      <span className="flex items-center gap-1 text-xs font-medium text-wa-dark">
        <Loader2 className="size-3.5 animate-spin" /> recording…
      </span>
    )
  if (st.status === "sent")
    return (
      <span className="flex items-center gap-1 text-xs font-medium text-wa-dark">
        <Check className="size-3.5" /> sent
      </span>
    )
  return <span className="text-xs text-destructive">failed: {st.error}</span>
}

function AddContact() {
  const [open, setOpen] = useState(false)
  const [name, setName] = useState("")
  const [phone, setPhone] = useState("")
  const [context, setContext] = useState("")
  const [err, setErr] = useState("")
  const save = async () => {
    setErr("")
    try {
      await api.addContact(name, phone, context)
      setName("")
      setPhone("")
      setContext("")
      setOpen(false)
    } catch (e) {
      setErr(String(e).replace(/^Error: \d+ /, ""))
    }
  }
  if (!open)
    return (
      <button onClick={() => setOpen(true)} className="flex items-center gap-2 border-b px-4 py-2.5 text-sm font-medium text-foreground/80 hover:bg-muted/50 hover:text-foreground">
        <Plus className="size-4" /> Add a contact
      </button>
    )
  return (
    <div className="space-y-2 border-b bg-muted/40 p-3">
      <div className="flex gap-2">
        <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="Name (e.g. Raj)" className="h-8 bg-card" />
        <Input value={phone} onChange={(e) => setPhone(e.target.value)} placeholder="+66 81 234 5678" className="h-8 bg-card" />
      </div>
      <Input value={context} onChange={(e) => setContext(e.target.value)} placeholder="How John knows them (met at the Bangkok meetup)" className="h-8 bg-card" />
      {err && <p className="text-xs text-destructive">{err}</p>}
      <div className="flex justify-end gap-2">
        <Button size="sm" variant="ghost" onClick={() => setOpen(false)}>
          Cancel
        </Button>
        <Button size="sm" onClick={save} disabled={!name || !phone}>
          Save
        </Button>
      </div>
    </div>
  )
}
