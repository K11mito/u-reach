import { useState } from "react"
import { AudioLines, Lock, Quote, ShieldCheck, UserRound } from "lucide-react"
import { harnessApi, type HarnessProfile, type HarnessSnapshot } from "@/lib/api"
import { toast } from "../Toast"
import { ChipEditor, EditorBar, Label, ListEditor, Section, TextField } from "./editors"
import { TryIt } from "./TryIt"

type Run = (p: Promise<HarnessSnapshot>, ok?: string) => Promise<boolean>

/** Who John is, the same in every preset: identity, voice, words, guardrails. */
export function ProfileEditor({ snap, run, onBack }: { snap: HarnessSnapshot; run: Run; onBack: () => void }) {
  const [draft, setDraft] = useState<HarnessProfile>(() => structuredClone(snap.profile))
  const [base, setBase] = useState(() => JSON.stringify(snap.profile))
  const [saving, setSaving] = useState(false)
  const dirty = JSON.stringify(draft) !== base
  const set = (patch: Partial<HarnessProfile>) => setDraft({ ...draft, ...patch })
  const setVoice = (patch: Partial<HarnessProfile["voice"]>) => setDraft({ ...draft, voice: { ...draft.voice, ...patch } })

  const save = async () => {
    setSaving(true)
    if (await run(harnessApi.saveProfile(draft))) {
      const fresh = await harnessApi.get().catch(() => null)
      const saved = fresh?.profile ?? draft
      setDraft(structuredClone(saved))
      setBase(JSON.stringify(saved))
      toast("Saved. John sounds like this in every preset from the next reply.")
    }
    setSaving(false)
  }
  const discard = () => {
    setDraft(structuredClone(snap.profile))
    setBase(JSON.stringify(snap.profile))
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
          <div className="min-w-0">
            <div className="truncate px-1 text-base font-semibold tracking-tight">Who John is</div>
            <div className="truncate px-1 text-xs text-muted-foreground">The same in every preset. Presets only change how he sells.</div>
          </div>
        }
      />

      <div className="grid min-h-0 flex-1 grid-cols-[minmax(0,1fr)_minmax(320px,380px)] gap-4">
        <div className="min-h-0 space-y-4 overflow-y-auto pr-1 pb-6">
          <Section title="Identity" hint="How the AI introduces John and what it knows about his life." icon={UserRound}>
            <div className="space-y-4">
              <div className="grid gap-4 xl:grid-cols-[2fr_1fr]">
                <div>
                  <Label>Full name</Label>
                  <TextField value={draft.name} onChange={(v) => set({ name: v })} />
                </div>
                <div>
                  <Label>What leads call him</Label>
                  <TextField value={draft.short_name} onChange={(v) => set({ short_name: v })} />
                </div>
              </div>
              <div>
                <Label>Role</Label>
                <TextField value={draft.role} onChange={(v) => set({ role: v })} />
              </div>
              <div>
                <Label>Background</Label>
                <ListEditor items={draft.background} onChange={(v) => set({ background: v })} addLabel="Add a fact" />
              </div>
              <div>
                <Label>What he believes</Label>
                <ListEditor items={draft.values} onChange={(v) => set({ values: v })} addLabel="Add a value" />
              </div>
            </div>
          </Section>

          <Section title="How he sounds" hint="Tone and rhythm the cloned voice should carry." icon={AudioLines}>
            <div className="space-y-4">
              <div className="grid gap-4 xl:grid-cols-2">
                <div>
                  <Label>Tone</Label>
                  <TextField value={draft.voice.tone} onChange={(v) => setVoice({ tone: v })} rows={3} />
                </div>
                <div>
                  <Label>Rhythm</Label>
                  <TextField value={draft.voice.rhythm} onChange={(v) => setVoice({ rhythm: v })} rows={3} />
                </div>
                <div>
                  <Label>His English</Label>
                  <TextField value={draft.voice.english} onChange={(v) => setVoice({ english: v })} rows={2} />
                </div>
                <div>
                  <Label>Brand words</Label>
                  <TextField value={draft.voice.brand_words} onChange={(v) => setVoice({ brand_words: v })} rows={2} />
                </div>
              </div>
              <div>
                <Label hint="Enter adds, Backspace removes">Signature phrases</Label>
                <ChipEditor items={draft.signature_phrases} onChange={(v) => set({ signature_phrases: v })} placeholder="Add a phrase…" />
              </div>
            </div>
          </Section>

          <Section title="His own words" hint="Real quotes and images keep the AI close to how he actually talks." icon={Quote}>
            <div className="space-y-4">
              <div>
                <Label>Lines he has really said</Label>
                <ListEditor items={draft.sample_lines} onChange={(v) => set({ sample_lines: v })} addLabel="Add a line" />
              </div>
              <div>
                <Label>Metaphors he uses</Label>
                <ListEditor items={draft.metaphors} onChange={(v) => set({ metaphors: v })} addLabel="Add a metaphor" />
              </div>
            </div>
          </Section>

          <Section title="Guardrails" hint="Lines the AI never crosses, in any preset." icon={ShieldCheck}>
            <Label>John never</Label>
            <ListEditor items={draft.never} onChange={(v) => set({ never: v })} placeholder="e.g. Never promises income." addLabel="Add a rule" />
            <div className="mt-5 rounded-lg bg-secondary/70 p-3">
              <div className="mb-2 flex items-center gap-1.5 text-xs font-medium">
                <Lock className="size-3.5" /> Always handed to you
              </div>
              <div className="flex flex-wrap gap-1.5">
                {snap.always_handoff.map((t) => (
                  <span key={t} className="rounded-md bg-card px-2 py-1 text-[11px] text-foreground/75 ring-1 ring-border">
                    {t}
                  </span>
                ))}
              </div>
              <p className="mt-2 text-[11px] text-muted-foreground">Everything else, earnings and "is it legit" included, the AI answers itself. For these it answers what it can, says you'll follow up, and the chat waits in your Inbox. Income promises are always blocked. Built in; can't be switched off.</p>
            </div>
          </Section>
        </div>
        <div className="min-h-0 overflow-y-auto pb-6">
          <TryIt draft={{ profile: draft }} title="Try John" hint="Answers with your unsaved edits and the preset in use." />
        </div>
      </div>
    </div>
  )
}
