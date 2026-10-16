import { useState } from "react"
import { BookOpenText, Check, Link2, Mic, Pencil, Plus, ShieldCheck, Sparkles, UserRoundPen, X } from "lucide-react"
import { cn } from "@/lib/utils"
import type { Autonomy as AutonomyLevel, HarnessPreset, HarnessSnapshot, HarnessStyle, State } from "@/lib/api"
import { JohnAvatar } from "../bits"
import { Autonomy } from "./Autonomy"
import { Section } from "./editors"

const TIMING: Record<HarnessStyle["link_timing"], string> = {
  first_reply: "link in the first reply",
  when_interested: "link once they're interested",
  when_asked: "link only when asked",
}

/** One line describing how a preset sells. */
export function styleSummary(s: HarnessStyle) {
  const len = s.max_words <= 45 ? "Very short notes" : s.max_words <= 60 ? "Short notes" : s.max_words <= 90 ? "Medium notes" : "Long notes"
  return `${len} (${s.min_words}–${s.max_words} words) · ${TIMING[s.link_timing]}`
}

/**
 * The top of John's page: who he is, which preset he sells with, and how
 * much his AI handles alone. Clicking a preset switches to it; hovering it
 * reveals the edit button for its details.
 */
export function Hero({
  state,
  snap,
  onActivate,
  onEditPreset,
  onEditProfile,
  onNewPreset,
  onAutonomy,
}: {
  state: State
  snap: HarnessSnapshot
  onActivate: (p: HarnessPreset) => void
  onEditPreset: (p: HarnessPreset) => void
  onEditProfile: () => void
  onNewPreset: (name: string) => Promise<void>
  onAutonomy: (level: AutonomyLevel) => Promise<boolean>
}) {
  const a = state.ambassador
  const p = snap.profile
  const active = snap.presets.find((x) => x.id === snap.active_id) ?? snap.presets[0]
  const docs = snap.documents.filter((d) => d.active).length
  const level = snap.levels.find((l) => l.key === snap.autonomy)
  const [naming, setNaming] = useState(false)
  const [name, setName] = useState("")
  const create = async () => {
    if (!name.trim()) return
    await onNewPreset(name.trim())
    setNaming(false)
    setName("")
  }

  return (
    <div className="grid shrink-0 gap-4 lg:grid-cols-3">
      {/* Who he is: the page's one accent card, like the headline stat on Analytics. */}
      <section className="flex flex-col rounded-xl bg-gradient-to-br from-indigo-600 to-purple-700 p-4 text-white shadow-lg shadow-indigo-500/25">
        <div className="flex items-start gap-4">
          <div className="rounded-full ring-4 ring-white/20">
            <JohnAvatar size={64} src={a.avatar} />
          </div>
          <div className="min-w-0 flex-1">
            <div className="text-[11px] font-semibold tracking-wide text-white/70 uppercase">U.Center ambassador</div>
            <h2 className="text-xl font-semibold tracking-tight">{p.name}</h2>
            <p className="mt-1 line-clamp-3 text-[13px] leading-snug text-white/85">{p.role}</p>
          </div>
        </div>
        <div className="mt-4 flex flex-wrap gap-1.5 text-[11px] font-medium">
          <Chip icon={Mic}>{a.mock_voice ? "Voice clone not set" : "Cloned voice ready"}</Chip>
          <Chip icon={BookOpenText}>
            {docs} playbook doc{docs === 1 ? "" : "s"}
          </Chip>
          <Chip icon={Link2}>{snap.referral_url ? "Referral link set" : "No referral link"}</Chip>
        </div>
        <div className="mt-auto flex items-center gap-2 pt-5">
          <button
            onClick={onEditProfile}
            className="inline-flex h-9 items-center gap-1.5 rounded-lg bg-white px-3.5 text-sm font-semibold text-indigo-700 shadow-sm transition hover:bg-indigo-50"
            title="Edit who John is: background, voice, phrases, guardrails"
          >
            <UserRoundPen className="size-4" /> Edit John
          </button>
          <span className="text-xs text-white/70">Background, voice, phrases and guardrails</span>
        </div>
      </section>

      <Section title="Selling as" hint="Click a preset to switch, hover it to edit." icon={Sparkles} className="flex flex-col">
        <div className="flex flex-wrap items-center gap-2">
          {snap.presets.map((x) => (
            <PresetPill key={x.id} preset={x} active={x.id === active?.id} onActivate={() => onActivate(x)} onEdit={() => onEditPreset(x)} />
          ))}
          {naming ? (
            <div className="inline-flex h-8 items-center gap-1 rounded-full border bg-background py-1 pr-1 pl-3">
              <input
                autoFocus
                value={name}
                onChange={(e) => setName(e.target.value)}
                onKeyDown={(e) => (e.key === "Enter" ? create() : e.key === "Escape" && setNaming(false))}
                placeholder="New preset name"
                className="w-36 bg-transparent text-xs outline-none"
              />
              <button onClick={create} disabled={!name.trim()} className="grid size-6 place-items-center rounded-full bg-indigo-600 text-white disabled:opacity-40" aria-label="Create preset">
                <Check className="size-3.5" />
              </button>
              <button onClick={() => setNaming(false)} className="grid size-6 place-items-center rounded-full hover:bg-secondary" aria-label="Cancel">
                <X className="size-3.5" />
              </button>
            </div>
          ) : (
            <button
              onClick={() => setNaming(true)}
              className="inline-flex h-8 items-center gap-1 rounded-full border border-dashed border-foreground/25 px-3 text-xs font-medium text-muted-foreground transition hover:border-indigo-400 hover:text-indigo-700"
              title="New preset, starting from the one in use"
            >
              <Plus className="size-3.5" /> New
            </button>
          )}
        </div>
        {active && (
          <div className="mt-auto pt-4">
            <div className="rounded-lg bg-indigo-50/70 px-3.5 py-3 ring-1 ring-indigo-100">
              <div className="flex items-center justify-between gap-2">
                <span className="text-sm font-semibold text-indigo-950">{active.name}</span>
                <button onClick={() => onEditPreset(active)} className="inline-flex items-center gap-1 text-xs font-medium text-indigo-700 hover:underline">
                  <Pencil className="size-3" /> Edit
                </button>
              </div>
              {active.description && <p className="mt-0.5 line-clamp-1 text-xs text-indigo-950/70" title={active.description}>{active.description}</p>}
              <p className="mt-1.5 text-xs font-medium text-indigo-900/80">{styleSummary(active.harness.style)}</p>
            </div>
          </div>
        )}
      </Section>

      <Section
        title="Autonomy"
        hint="What the AI handles before calling John."
        icon={ShieldCheck}
        className="flex flex-col"
        action={level && <span className="rounded-full bg-indigo-50 px-2.5 py-1 text-xs font-semibold text-indigo-700 ring-1 ring-indigo-100">{level.label}</span>}
      >
        <Autonomy value={snap.autonomy} levels={snap.levels} onChange={onAutonomy} />
      </Section>
    </div>
  )
}

function PresetPill({ preset, active, onActivate, onEdit }: { preset: HarnessPreset; active: boolean; onActivate: () => void; onEdit: () => void }) {
  return (
    <div
      className={cn(
        "group/pill inline-flex h-8 items-center rounded-full transition-colors",
        active ? "bg-indigo-600 text-white shadow-sm shadow-indigo-500/25" : "bg-secondary text-foreground/80 hover:bg-indigo-50 hover:text-indigo-800",
      )}
    >
      <button onClick={onActivate} title={active ? `In use: ${preset.description}` : `Switch to ${preset.name}: ${preset.description}`} className="inline-flex h-8 items-center gap-1.5 rounded-full pr-1.5 pl-3 text-xs font-medium outline-none focus-visible:ring-2 focus-visible:ring-indigo-400">
        {active && <Check className="size-3.5" />}
        {preset.name}
      </button>
      {/* Keeps its space and only fades in on hover or keyboard focus: nothing
          changes width, so the row never shifts or flickers. */}
      <button
        onClick={onEdit}
        aria-label={`Edit ${preset.name}`}
        title={`Edit ${preset.name}`}
        className={cn(
          "mr-1 grid size-6 place-items-center rounded-full opacity-0 outline-none transition-opacity group-hover/pill:opacity-100 focus-visible:opacity-100 focus-visible:ring-2 focus-visible:ring-indigo-400",
          active ? "bg-white/20 hover:bg-white/30" : "bg-white hover:bg-indigo-100",
        )}
      >
        <Pencil className="size-3" />
      </button>
    </div>
  )
}

function Chip({ icon: Icon, children }: { icon: typeof Mic; children: React.ReactNode }) {
  return (
    <span className="inline-flex items-center gap-1.5 rounded-md bg-white/15 px-2 py-1 ring-1 ring-white/15">
      <Icon className="size-3" />
      {children}
    </span>
  )
}
