import { useCallback, useEffect, useState } from "react"
import {
  AudioLines,
  Check,
  Copy,
  Link2,
  Loader2,
  Lock,
  MessageSquareQuote,
  Mic,
  MonitorPlay,
  Pencil,
  Plus,
  RotateCcw,
  ScrollText,
  ShieldCheck,
  SlidersHorizontal,
  Target,
  Trash2,
  UserRound,
  X,
} from "lucide-react"
import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"
import { api, errText, harnessApi, type Harness, type HarnessPersona, type HarnessPreset, type HarnessSnapshot, type HarnessStyle, type LinkTiming, type State } from "@/lib/api"
import { JohnAvatar, VoiceNote } from "./bits"
import { Grain } from "./Grain"
import { toast } from "./Toast"
import { ChipEditor, Choice, Label, ListEditor, Section, TextField } from "./john/editors"
import { Playbook } from "./john/Playbook"
import { TryIt } from "./john/TryIt"

const activeOf = (s: HarnessSnapshot) => s.presets.find((p) => p.id === s.active_id) ?? s.presets[0]

/**
 * John's page: everything his AI knows and how it sells, all editable.
 * Presets hold the persona and reply style; the FAQ, referral link and
 * sales playbook are shared by every preset. Saved changes apply to the
 * next reply, no restart.
 */
export function Profile({ state }: { state: State }) {
  const [snap, setSnap] = useState<HarnessSnapshot | null>(null)
  const [draft, setDraft] = useState<Harness | null>(null)
  const [base, setBase] = useState("") // the saved harness the draft started from
  const [saving, setSaving] = useState(false)
  const active = snap ? activeOf(snap) : undefined
  const dirty = !!draft && JSON.stringify(draft) !== base

  // Take a fresh snapshot and reset the draft to the active preset.
  const adopt = useCallback((s: HarnessSnapshot) => {
    setSnap(s)
    const a = activeOf(s)
    if (a) {
      setDraft(structuredClone(a.harness))
      setBase(JSON.stringify(a.harness))
    }
  }, [])

  useEffect(() => {
    harnessApi.get().then(adopt).catch((e) => toast(errText(e), "error"))
  }, [adopt])
  // Edits from another tab: refresh, but never throw away unsaved work.
  useEffect(() => {
    const on = () =>
      harnessApi
        .get()
        .then((s) => (dirty ? setSnap(s) : adopt(s)))
        .catch(() => {})
    window.addEventListener("ureach:harness", on)
    return () => window.removeEventListener("ureach:harness", on)
  }, [adopt, dirty])

  if (!snap || !draft || !active) {
    return (
      <div className="grid flex-1 place-items-center text-sm text-muted-foreground">
        <Loader2 className="size-5 animate-spin" />
      </div>
    )
  }

  const run = async (p: Promise<HarnessSnapshot>, ok?: string) => {
    try {
      adopt(await p)
      if (ok) toast(ok)
    } catch (e) {
      toast(errText(e), "error")
    }
  }
  const save = async () => {
    setSaving(true)
    await run(harnessApi.saveActive(draft), `Saved “${active.name}”. John uses it from the next reply.`)
    setSaving(false)
  }
  const switchTo = (p: HarnessPreset) => {
    if (p.id === active.id) return
    if (dirty && !window.confirm(`Discard your unsaved changes to “${active.name}”?`)) return
    run(harnessApi.activate(p.id), `John now sells with “${p.name}”`)
  }
  const persona = draft.persona
  const setPersona = (patch: Partial<HarnessPersona>) => setDraft({ ...draft, persona: { ...persona, ...patch } })
  const setStyle = (patch: Partial<HarnessStyle>) => setDraft({ ...draft, style: { ...draft.style, ...patch } })

  return (
    <div className="flex min-h-0 flex-1 flex-col gap-4">
      <PresetBar snap={snap} active={active} draft={draft} dirty={dirty} saving={saving} onSwitch={switchTo} onSave={save} onDiscard={() => adopt(snap)} run={run} />

      <div className="grid min-h-0 flex-1 grid-cols-[minmax(0,1fr)_minmax(340px,400px)] gap-4">
        <div className="min-h-0 space-y-4 overflow-y-auto pr-1 pb-6">
          <Section title="Reply style" hint={`Part of “${active.name}”. How long his voice notes are and when the link goes out.`} icon={SlidersHorizontal}>
            <StyleEditor style={draft.style} onChange={setStyle} />
          </Section>

          <Section title="How John sells" hint="His approach, in order. The AI follows this on every reply." icon={Target}>
            <ListEditor items={persona.how_he_sells} onChange={(v) => setPersona({ how_he_sells: v })} placeholder="e.g. He connects U-topia to something in the lead's life." addLabel="Add a step" />
          </Section>

          <Section title="Openers and closers" hint="Ways he starts and ends a voice note. {name} becomes the lead's first name." icon={MessageSquareQuote}>
            <div className="grid gap-5 xl:grid-cols-2">
              <div>
                <Label>Openers</Label>
                <ListEditor items={persona.openers} onChange={(v) => setPersona({ openers: v })} placeholder="Hey {name}! John here…" addLabel="Add opener" />
              </div>
              <div>
                <Label>Closers</Label>
                <ListEditor items={persona.closers} onChange={(v) => setPersona({ closers: v })} placeholder="Shall we jump on a quick call?" addLabel="Add closer" />
              </div>
            </div>
          </Section>

          <Section title="How he sounds" hint="Tone and phrases the voice clone should carry." icon={AudioLines}>
            <div className="space-y-4">
              <div className="grid gap-4 xl:grid-cols-2">
                <div>
                  <Label>Tone</Label>
                  <TextField value={persona.voice.tone} onChange={(v) => setPersona({ voice: { ...persona.voice, tone: v } })} rows={3} />
                </div>
                <div>
                  <Label>Rhythm</Label>
                  <TextField value={persona.voice.rhythm} onChange={(v) => setPersona({ voice: { ...persona.voice, rhythm: v } })} rows={3} />
                </div>
              </div>
              <div>
                <Label hint="Enter adds, Backspace removes">Signature phrases</Label>
                <ChipEditor items={persona.signature_phrases} onChange={(v) => setPersona({ signature_phrases: v })} placeholder="Add a phrase…" />
              </div>
              <div>
                <Label hint="Real quotes keep the AI close to how he talks">Lines he has really said</Label>
                <ListEditor items={persona.sample_lines} onChange={(v) => setPersona({ sample_lines: v })} addLabel="Add a line" />
              </div>
              <div>
                <Label>Metaphors he uses</Label>
                <ListEditor items={persona.metaphors} onChange={(v) => setPersona({ metaphors: v })} addLabel="Add a metaphor" />
              </div>
            </div>
          </Section>

          <Section title="Guardrails" hint="Lines the AI never crosses." icon={ShieldCheck}>
            <Label>John never</Label>
            <ListEditor items={persona.never} onChange={(v) => setPersona({ never: v })} placeholder="e.g. Never promises income." addLabel="Add a rule" />
            <div className="mt-5 rounded-lg bg-secondary/70 p-3">
              <div className="mb-2 flex items-center gap-1.5 text-xs font-medium">
                <Lock className="size-3.5" /> Always handed to you, in every preset
              </div>
              <div className="flex flex-wrap gap-1.5">
                {snap.always_handoff.map((t) => (
                  <span key={t} className="rounded-md bg-card px-2 py-1 text-[11px] text-foreground/75 ring-1 ring-border">
                    {t}
                  </span>
                ))}
              </div>
              <p className="mt-2 text-[11px] text-muted-foreground">The AI sends a short holding note and the chat waits in your Inbox. This safety net is built in and can't be switched off.</p>
            </div>
          </Section>

          <Section title="Who John is" hint="Background the AI draws on to make replies personal." icon={UserRound}>
            <div className="space-y-4">
              <div className="grid gap-4 xl:grid-cols-[1fr_2fr]">
                <div>
                  <Label>Name</Label>
                  <TextField value={persona.name} onChange={(v) => setPersona({ name: v })} />
                </div>
                <div>
                  <Label>Role</Label>
                  <TextField value={persona.role} onChange={(v) => setPersona({ role: v })} />
                </div>
              </div>
              <div>
                <Label>Background</Label>
                <ListEditor items={persona.background} onChange={(v) => setPersona({ background: v })} addLabel="Add a fact" />
              </div>
              <div>
                <Label>What he believes</Label>
                <ListEditor items={persona.values} onChange={(v) => setPersona({ values: v })} addLabel="Add a value" />
              </div>
            </div>
          </Section>
        </div>

        <div className="min-h-0 space-y-4 overflow-y-auto pr-1 pb-6">
          <Identity state={state} snap={snap} persona={persona} active={active} />
          <Referral snap={snap} run={run} />
          <TryIt dirty={dirty} />
          <Playbook snap={snap} onChange={(s) => (dirty ? setSnap(s) : adopt(s))} />
          <FAQ snap={snap} run={run} />
          <DemoTools />
        </div>
      </div>
    </div>
  )
}

function PresetBar({
  snap,
  active,
  draft,
  dirty,
  saving,
  onSwitch,
  onSave,
  onDiscard,
  run,
}: {
  snap: HarnessSnapshot
  active: HarnessPreset
  draft: Harness
  dirty: boolean
  saving: boolean
  onSwitch: (p: HarnessPreset) => void
  onSave: () => void
  onDiscard: () => void
  run: (p: Promise<HarnessSnapshot>, ok?: string) => Promise<void>
}) {
  const [naming, setNaming] = useState(false)
  const [name, setName] = useState("")
  const saveAs = async () => {
    if (!name.trim()) return
    await run(harnessApi.saveAs(name, "Saved from the John page", draft), `Saved as “${name.trim()}”`)
    setNaming(false)
    setName("")
  }
  return (
    <div className="rounded-xl border bg-card p-3">
      <div className="mb-2 flex items-center justify-between gap-3 px-1">
        <div className="text-xs font-medium text-muted-foreground">Sales preset · John replies with the highlighted one</div>
        <div className="flex items-center gap-1">
          <Button
            variant="ghost"
            size="xs"
            onClick={() => {
              const n = window.prompt("Rename this preset", active.name)
              if (n?.trim()) run(harnessApi.rename(active.id, n, active.description))
            }}
          >
            <Pencil /> Rename
          </Button>
          {active.key ? (
            <Button variant="ghost" size="xs" onClick={() => window.confirm(`Reset “${active.name}” to how it shipped?`) && run(harnessApi.reset(active.id), "Preset reset")}>
              <RotateCcw /> Reset
            </Button>
          ) : (
            <Button variant="ghost" size="xs" onClick={() => window.confirm(`Delete “${active.name}”?`) && run(harnessApi.remove(active.id), "Preset deleted")}>
              <Trash2 /> Delete
            </Button>
          )}
        </div>
      </div>
      <div className="flex items-stretch gap-2 overflow-x-auto pb-0.5">
        {snap.presets.map((p) => {
          const on = p.id === active.id
          return (
            <button
              key={p.id}
              onClick={() => onSwitch(p)}
              className={cn("w-52 shrink-0 rounded-lg border px-3 py-2.5 text-left transition", on ? "border-foreground bg-foreground text-background" : "hover:border-foreground/30")}
            >
              <div className="flex items-center gap-1.5 text-sm font-semibold">
                {on && <Check className="size-3.5" />}
                <span className="truncate">{p.name}</span>
              </div>
              <div className={cn("mt-0.5 line-clamp-2 text-[11px] leading-snug", on ? "text-background/70" : "text-muted-foreground")}>{p.description}</div>
            </button>
          )
        })}
        {naming ? (
          <div className="flex w-64 shrink-0 items-center gap-1.5 rounded-lg border border-dashed px-2">
            <input
              autoFocus
              value={name}
              onChange={(e) => setName(e.target.value)}
              onKeyDown={(e) => (e.key === "Enter" ? saveAs() : e.key === "Escape" && setNaming(false))}
              placeholder="Preset name"
              className="min-w-0 flex-1 bg-transparent text-sm outline-none"
            />
            <Button size="icon-sm" onClick={saveAs} disabled={!name.trim()} aria-label="Save preset">
              <Check />
            </Button>
            <Button size="icon-sm" variant="ghost" onClick={() => setNaming(false)} aria-label="Cancel">
              <X />
            </Button>
          </div>
        ) : (
          <button onClick={() => setNaming(true)} className="flex w-40 shrink-0 items-center justify-center gap-1.5 rounded-lg border border-dashed text-xs font-medium text-muted-foreground hover:border-foreground/30 hover:text-foreground">
            <Plus className="size-3.5" /> Save current as new
          </button>
        )}
      </div>
      {dirty && (
        <div className="mt-3 flex items-center gap-3 rounded-lg bg-amber/15 px-3 py-2">
          <span className="size-2 rounded-full bg-amber" />
          <span className="min-w-0 flex-1 text-sm">
            Unsaved changes to <b>{active.name}</b>. John keeps using the saved version until you save.
          </span>
          <Button variant="ghost" size="sm" onClick={onDiscard}>
            Discard
          </Button>
          <Button size="sm" onClick={onSave} disabled={saving}>
            {saving ? <Loader2 className="animate-spin" /> : <Check />} Save
          </Button>
        </div>
      )}
    </div>
  )
}

const LENGTHS = [
  { min: 25, max: 45, label: "Very short" },
  { min: 35, max: 60, label: "Short" },
  { min: 50, max: 90, label: "Medium" },
  { min: 80, max: 120, label: "Long" },
]
const secs = (w: number) => Math.round(w / 2.5)

function StyleEditor({ style, onChange }: { style: HarnessStyle; onChange: (p: Partial<HarnessStyle>) => void }) {
  const key = `${style.min_words}-${style.max_words}`
  const options = LENGTHS.map((l) => ({ value: `${l.min}-${l.max}`, label: l.label, hint: `${l.min}–${l.max} words · ${secs(l.min)}–${secs(l.max)}s` }))
  if (!options.some((o) => o.value === key)) options.push({ value: key, label: "Custom", hint: `${style.min_words}–${style.max_words} words` })
  return (
    <div className="space-y-5">
      <div>
        <Label>Voice note length</Label>
        <Choice
          value={key}
          onChange={(v) => {
            const [min, max] = v.split("-").map(Number)
            onChange({ min_words: min, max_words: max })
          }}
          options={options}
        />
      </div>
      <div>
        <Label>When he sends his referral link</Label>
        <Choice<LinkTiming>
          value={style.link_timing}
          onChange={(v) => onChange({ link_timing: v })}
          options={[
            { value: "first_reply", label: "First reply", hint: "Right away, with an offer to help set up" },
            { value: "when_interested", label: "Once interested", hint: "After they ask how to join or say they want in" },
            { value: "when_asked", label: "Only when asked", hint: "Never pushed; sent when they ask for it" },
          ]}
        />
      </div>
      <div>
        <Label>Language</Label>
        <Choice
          value={style.language}
          onChange={(v) => onChange({ language: v })}
          options={[
            { value: "english", label: "English", hint: "Always replies in English" },
            { value: "match", label: "Match the lead", hint: "Replies in the language they write in" },
          ]}
        />
      </div>
    </div>
  )
}

function Identity({ state, snap, persona, active }: { state: State; snap: HarnessSnapshot; persona: HarnessPersona; active: HarnessPreset }) {
  const a = state.ambassador
  const docs = snap.documents.filter((d) => d.active).length
  return (
    <Grain seed="john-harness" palette="ocean" className="rounded-xl p-5 text-white [text-shadow:0_1px_2px_rgb(0_0_0/0.15)]">
      <div className="relative flex items-center gap-4">
        <JohnAvatar size={60} src={a.avatar} />
        <div className="min-w-0">
          <div className="text-xl font-semibold tracking-tight">{persona.name}</div>
          <div className="line-clamp-2 text-xs text-white/90">{persona.role}</div>
        </div>
      </div>
      <div className="relative mt-4 flex flex-wrap gap-1.5 text-[11px] font-medium">
        <span className="rounded-md bg-black/20 px-2 py-1">Selling as “{active.name}”</span>
        <span className="rounded-md bg-black/20 px-2 py-1">{a.mock_voice ? "Voice clone not set (mock tone)" : "Cloned voice ready"}</span>
        <span className="rounded-md bg-black/20 px-2 py-1">
          {docs} playbook doc{docs === 1 ? "" : "s"}
        </span>
      </div>
      {(a.samples?.original || a.samples?.clone) && (
        <div className="relative mt-4 space-y-2">
          {a.samples.original && (
            <div className="rounded-xl bg-white/90 px-3 py-2 text-foreground">
              <div className="mb-1 flex items-center gap-1.5 text-[11px] font-semibold text-muted-foreground">
                <MonitorPlay className="size-3.5 text-red-500" /> The real John
              </div>
              <VoiceNote src={a.samples.original} durationMs={14000} seed={11} />
            </div>
          )}
          {a.samples.clone && (
            <div className="rounded-xl bg-wa-bubble px-3 py-2 text-foreground">
              <div className="mb-1 flex items-center gap-1.5 text-[11px] font-semibold text-wa-dark">
                <Mic className="size-3.5" /> His AI voice
              </div>
              <VoiceNote src={a.samples.clone} durationMs={9000} seed={22} outgoing />
            </div>
          )}
        </div>
      )}
    </Grain>
  )
}

function Referral({ snap, run }: { snap: HarnessSnapshot; run: (p: Promise<HarnessSnapshot>, ok?: string) => Promise<void> }) {
  const [url, setUrl] = useState(snap.referral_url)
  useEffect(() => setUrl(snap.referral_url), [snap.referral_url])
  const changed = url.trim() !== snap.referral_url
  return (
    <Section title="Referral link" hint="Sent as a text after his voice note. Leads who join through it get John as their sponsor." icon={Link2}>
      <div className="flex gap-2">
        <input
          value={url}
          onChange={(e) => setUrl(e.target.value)}
          onKeyDown={(e) => e.key === "Enter" && changed && run(harnessApi.setReferral(url), "Referral link saved")}
          placeholder="https://u.center/?ref=…"
          className="h-9 min-w-0 flex-1 rounded-lg border border-input bg-background px-3 font-mono text-[13px] outline-none focus:border-ring focus:ring-[3px] focus:ring-ring/30"
        />
        {changed ? (
          <Button onClick={() => run(harnessApi.setReferral(url), "Referral link saved")}>Save</Button>
        ) : (
          <Button
            variant="outline"
            size="icon"
            disabled={!url}
            onClick={() =>
              navigator.clipboard
                .writeText(url)
                .then(() => toast("Referral link copied"))
                .catch(() => toast("Couldn't copy", "error"))
            }
            aria-label="Copy link"
            title="Copy link"
          >
            <Copy />
          </Button>
        )}
      </div>
    </Section>
  )
}

function FAQ({ snap, run }: { snap: HarnessSnapshot; run: (p: Promise<HarnessSnapshot>, ok?: string) => Promise<void> }) {
  const [text, setText] = useState(snap.faq)
  useEffect(() => setText(snap.faq), [snap.faq])
  const changed = text !== snap.faq
  return (
    <Section
      title="Facts John can share"
      hint="The only facts the AI states, shared by every preset. Anything not here, he offers to cover on a call."
      icon={ScrollText}
      action={
        changed && (
          <div className="flex gap-1">
            <Button variant="ghost" size="sm" onClick={() => setText(snap.faq)}>
              Discard
            </Button>
            <Button size="sm" onClick={() => run(harnessApi.setFAQ(text), "Facts saved")}>
              Save
            </Button>
          </div>
        )
      }
    >
      <div className="max-h-[28rem] overflow-y-auto rounded-lg">
        <TextField value={text} onChange={setText} rows={12} mono />
      </div>
      <div className="mt-1.5 text-right text-[11px] text-muted-foreground tabular-nums">{text.split(/\s+/).filter(Boolean).length.toLocaleString()} words · Markdown</div>
    </Section>
  )
}

/** Rehearsal helper: put the seeded demo back. The WhatsApp link is kept. */
function DemoTools() {
  const [busy, setBusy] = useState(false)
  const reset = async () => {
    if (!window.confirm("Reset to the demo leads? This deletes every conversation since the last reset. WhatsApp stays linked, and John's settings are kept.")) return
    setBusy(true)
    try {
      await api.resetDemo() // every open dashboard reloads on the "reset" event
    } catch (e) {
      toast(`Reset failed: ${errText(e)}`, "error")
      setBusy(false)
    }
  }
  return (
    <div className="flex items-center gap-4 rounded-xl border border-dashed p-5">
      <div className="min-w-0 flex-1">
        <div className="font-semibold">Demo tools</div>
        <p className="text-xs text-muted-foreground">Between rehearsals: restore the 10 seeded leads and clear live chats. John's settings and playbook stay as they are.</p>
      </div>
      <Button variant="outline" onClick={reset} disabled={busy}>
        {busy ? <Loader2 className="animate-spin" /> : <RotateCcw />} Reset demo
      </Button>
    </div>
  )
}
