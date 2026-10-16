import { useState } from "react"
import { CircleCheck, RotateCcw, SlidersHorizontal, Target, Trash2, UserRoundPen } from "lucide-react"
import { harnessApi, type Harness, type HarnessPreset, type HarnessSnapshot, type HarnessStyle, type LinkTiming } from "@/lib/api"
import { Choice, EditorBar, Label, Section } from "./editors"
import { TryIt } from "./TryIt"
import { toast } from "../Toast"

type Run = (p: Promise<HarnessSnapshot>, ok?: string) => Promise<boolean>

const LENGTHS = [
  { min: 25, max: 45, label: "Very short" },
  { min: 35, max: 60, label: "Short" },
  { min: 50, max: 90, label: "Medium" },
  { min: 80, max: 120, label: "Long" },
]
const secs = (w: number) => Math.round(w / 2.5)

/** Everything about one sales preset: style, steps, openers and closers. */
export function PresetEditor({ snap, preset, run, onBack, onEditProfile }: { snap: HarnessSnapshot; preset: HarnessPreset; run: Run; onBack: () => void; onEditProfile: () => void }) {
  const [draft, setDraft] = useState<Harness>(() => structuredClone(preset.harness))
  const [name, setName] = useState(preset.name)
  const [description, setDescription] = useState(preset.description)
  const [base, setBase] = useState(() => JSON.stringify(preset.harness))
  const [saving, setSaving] = useState(false)
  const inUse = preset.id === snap.active_id
  const harnessDirty = JSON.stringify(draft) !== base
  const dirty = harnessDirty || name !== preset.name || description !== preset.description

  const setStyle = (patch: Partial<HarnessStyle>) => setDraft({ ...draft, style: { ...draft.style, ...patch } })
  const discard = () => {
    setDraft(structuredClone(preset.harness))
    setBase(JSON.stringify(preset.harness))
    setName(preset.name)
    setDescription(preset.description)
  }
  const save = async () => {
    setSaving(true)
    let ok = true
    if (name !== preset.name || description !== preset.description) ok = await run(harnessApi.rename(preset.id, name, description))
    if (ok && harnessDirty) ok = await run(harnessApi.savePreset(preset.id, draft))
    if (ok) {
      setBase(JSON.stringify(draft))
      // The saved preset comes back normalized; adopt it so the form matches.
      const fresh = await harnessApi.get().catch(() => null)
      const saved = fresh?.presets.find((p) => p.id === preset.id)
      if (saved) {
        setDraft(structuredClone(saved.harness))
        setBase(JSON.stringify(saved.harness))
      }
    }
    setSaving(false)
    if (ok) toast(inUse ? `Saved. John uses “${name}” from the next reply.` : `Saved “${name}”.`)
  }
  const back = () => (!dirty || window.confirm("Leave without saving your changes?")) && onBack()

  return (
    <div className="flex min-h-0 flex-1 flex-col gap-4">
      <EditorBar
        onBack={back}
        dirty={dirty}
        saving={saving}
        onSave={save}
        onDiscard={discard}
        title={
          <div className="flex min-w-0 items-center gap-3">
            <div className="min-w-0 flex-1">
              <input value={name} onChange={(e) => setName(e.target.value)} aria-label="Preset name" className="w-full truncate rounded-md bg-transparent px-1 text-base font-semibold tracking-tight outline-none hover:bg-secondary focus:bg-secondary" />
              <input
                value={description}
                onChange={(e) => setDescription(e.target.value)}
                aria-label="Preset description"
                placeholder="One line on when to use this preset"
                className="w-full truncate rounded-md bg-transparent px-1 text-xs text-muted-foreground outline-none hover:bg-secondary focus:bg-secondary"
              />
            </div>
          </div>
        }
        extra={
          <div className="flex items-center gap-1">
            {inUse ? (
              <span className="inline-flex h-8 items-center gap-1.5 rounded-lg bg-wa/15 px-2.5 text-xs font-semibold text-wa-dark">
                <CircleCheck className="size-3.5" /> In use
              </span>
            ) : (
              <button onClick={() => run(harnessApi.activate(preset.id), `John now sells with “${preset.name}”`)} className="h-8 rounded-lg border px-3 text-xs font-medium hover:bg-secondary">
                Use this preset
              </button>
            )}
            {preset.key ? (
              <button
                onClick={async () => window.confirm(`Reset “${preset.name}” to how it shipped?`) && (await run(harnessApi.reset(preset.id), "Preset reset")) && onBack()}
                className="grid size-8 place-items-center rounded-lg text-muted-foreground hover:bg-secondary hover:text-foreground"
                title="Reset to how it shipped"
                aria-label="Reset preset"
              >
                <RotateCcw className="size-4" />
              </button>
            ) : (
              <button
                onClick={async () => window.confirm(`Delete “${preset.name}”?`) && (await run(harnessApi.remove(preset.id), "Preset deleted")) && onBack()}
                className="grid size-8 place-items-center rounded-lg text-muted-foreground hover:bg-red-50 hover:text-red-600"
                title="Delete preset"
                aria-label="Delete preset"
              >
                <Trash2 className="size-4" />
              </button>
            )}
          </div>
        }
      />

      <div className="grid min-h-0 flex-1 grid-cols-[minmax(0,1fr)_minmax(320px,380px)] gap-4">
        <div className="min-h-0 space-y-4 overflow-y-auto pr-1 pb-6">
          <Section title="Reply style" hint="How long his voice notes are and when the link goes out." icon={SlidersHorizontal}>
            <StyleEditor style={draft.style} onChange={setStyle} />
          </Section>
          <Section title="How John sells" hint="Write it the way you'd brief a new teammate: what he leads with, what he asks, how he closes. The AI follows it on every reply." icon={Target}>
            <textarea
              value={draft.approach}
              onChange={(e) => setDraft({ ...draft, approach: e.target.value })}
              placeholder={"e.g. He starts by asking where they're based and what made them curious.\n\nHe connects U-topia to the one thing that matters to them, then offers a quick call with someone from the team."}
              rows={14}
              className="field-sizing-content min-h-72 w-full resize-y rounded-lg border border-input bg-background px-4 py-3 text-sm leading-relaxed outline-none transition placeholder:text-muted-foreground/70 focus:border-ring focus:ring-[3px] focus:ring-ring/30"
            />
            <div className="mt-1.5 flex justify-between text-[11px] text-muted-foreground">
              <span>His openers, closers and phrases come from Edit John.</span>
              <span className="tabular-nums">{draft.approach.split(/\s+/).filter(Boolean).length} words</span>
            </div>
          </Section>
        </div>
        <div className="min-h-0 space-y-4 overflow-y-auto pb-6">
          <TryIt draft={{ preset: draft }} title="Try this preset" hint="Answers with your unsaved edits, plus John's profile, facts and playbook." />
          <div className="rounded-xl border border-dashed p-4 text-xs text-muted-foreground">
            <p>
              Who John is, how he sounds and his guardrails are the same in every preset ({snap.presets.length} presets).
            </p>
            <button onClick={onEditProfile} className="mt-2 inline-flex items-center gap-1.5 font-medium text-foreground hover:underline">
              <UserRoundPen className="size-3.5" /> Edit John
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}

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
            { value: "when_interested", label: "Once ready", hint: "Keeps the chat going; sends it when they ask how to join or are rated hot" },
            { value: "when_asked", label: "Only when asked", hint: "Never pushed; sent when they ask for it" },
          ]}
        />
      </div>
    </div>
  )
}
