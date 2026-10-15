import { useState } from "react"
import { BookOpenText, Check, Link2, Mic, Pencil, Plus, UserRoundPen, X } from "lucide-react"
import { cn } from "@/lib/utils"
import type { HarnessPreset, HarnessSnapshot, HarnessStyle, State } from "@/lib/api"
import { JohnAvatar } from "../bits"
import { Grain } from "../Grain"

const TIMING: Record<HarnessStyle["link_timing"], string> = {
  first_reply: "link in the first reply",
  when_interested: "link once they're interested",
  when_asked: "link only when asked",
}

/** One line describing how a preset sells. */
export function styleSummary(s: HarnessStyle) {
  const len = s.max_words <= 45 ? "Very short notes" : s.max_words <= 60 ? "Short notes" : s.max_words <= 90 ? "Medium notes" : "Long notes"
  return `${len} (${s.min_words}–${s.max_words} words) · ${TIMING[s.link_timing]} · ${s.language === "match" ? "replies in the lead's language" : "English"}`
}

/**
 * John's card: who he is and which preset he sells with. Clicking a preset
 * switches to it; hovering it reveals the edit button for its details.
 */
export function Hero({
  state,
  snap,
  onActivate,
  onEditPreset,
  onEditProfile,
  onNewPreset,
}: {
  state: State
  snap: HarnessSnapshot
  onActivate: (p: HarnessPreset) => void
  onEditPreset: (p: HarnessPreset) => void
  onEditProfile: () => void
  onNewPreset: (name: string) => Promise<void>
}) {
  const a = state.ambassador
  const p = snap.profile
  const active = snap.presets.find((x) => x.id === snap.active_id) ?? snap.presets[0]
  const docs = snap.documents.filter((d) => d.active).length
  const [naming, setNaming] = useState(false)
  const [name, setName] = useState("")
  const create = async () => {
    if (!name.trim()) return
    await onNewPreset(name.trim())
    setNaming(false)
    setName("")
  }

  return (
    <Grain seed="john-hero" palette="ocean" className="rounded-xl p-6 text-white [text-shadow:0_1px_2px_rgb(0_0_0/0.12)]">
      <div className="relative flex items-start gap-5">
        <JohnAvatar size={84} src={a.avatar} />
        <div className="min-w-0 flex-1">
          <div className="flex items-start justify-between gap-4">
            <div className="min-w-0">
              <h2 className="text-2xl font-semibold tracking-tight">{p.name}</h2>
              <p className="mt-1 line-clamp-2 max-w-2xl text-sm text-white/90">{p.role}</p>
            </div>
            <button
              onClick={onEditProfile}
              className="inline-flex h-8 shrink-0 items-center gap-1.5 rounded-lg bg-black/20 px-3 text-xs font-medium backdrop-blur transition hover:bg-black/30"
              title="Edit who John is: background, voice, phrases, guardrails"
            >
              <UserRoundPen className="size-3.5" /> Edit John
            </button>
          </div>
          <div className="mt-3 flex flex-wrap gap-1.5 text-[11px] font-medium">
            <Chip icon={Mic}>{a.mock_voice ? "Voice clone not set (mock tone)" : "Cloned voice ready"}</Chip>
            <Chip icon={BookOpenText}>
              {docs} playbook doc{docs === 1 ? "" : "s"} active
            </Chip>
            <Chip icon={Link2}>{snap.referral_url ? "Referral link set" : "No referral link"}</Chip>
          </div>
        </div>
      </div>

      <div className="relative mt-6 border-t border-white/20 pt-5">
        <div className="mb-2.5 text-[11px] font-semibold tracking-wide text-white/75 uppercase">Selling as</div>
        <div className="flex flex-wrap items-center gap-2">
          {snap.presets.map((x) => (
            <PresetPill key={x.id} preset={x} active={x.id === active?.id} onActivate={() => onActivate(x)} onEdit={() => onEditPreset(x)} />
          ))}
          {naming ? (
            <div className="inline-flex h-8 items-center gap-1 rounded-full bg-white/95 py-1 pr-1 pl-3 text-foreground [text-shadow:none]">
              <input
                autoFocus
                value={name}
                onChange={(e) => setName(e.target.value)}
                onKeyDown={(e) => (e.key === "Enter" ? create() : e.key === "Escape" && setNaming(false))}
                placeholder="New preset name"
                className="w-36 bg-transparent text-xs outline-none"
              />
              <button onClick={create} disabled={!name.trim()} className="grid size-6 place-items-center rounded-full bg-foreground text-background disabled:opacity-40" aria-label="Create preset">
                <Check className="size-3.5" />
              </button>
              <button onClick={() => setNaming(false)} className="grid size-6 place-items-center rounded-full hover:bg-secondary" aria-label="Cancel">
                <X className="size-3.5" />
              </button>
            </div>
          ) : (
            <button
              onClick={() => setNaming(true)}
              className="inline-flex h-8 items-center gap-1 rounded-full border border-dashed border-white/50 px-3 text-xs font-medium text-white/90 transition hover:border-white hover:bg-white/10"
              title="New preset, starting from the one in use"
            >
              <Plus className="size-3.5" /> New
            </button>
          )}
        </div>
        {active && (
          <p className="mt-3 text-xs text-white/85">
            <b className="font-semibold">{active.name}:</b> {styleSummary(active.harness.style)}
          </p>
        )}
      </div>
    </Grain>
  )
}

function PresetPill({ preset, active, onActivate, onEdit }: { preset: HarnessPreset; active: boolean; onActivate: () => void; onEdit: () => void }) {
  return (
    <div
      className={cn(
        "group/pill inline-flex h-8 items-center rounded-full transition [text-shadow:none]",
        active ? "bg-white text-foreground shadow-sm shadow-black/10" : "bg-white/15 text-white backdrop-blur hover:bg-white/25",
      )}
    >
      <button onClick={onActivate} title={active ? `In use: ${preset.description}` : `Switch to ${preset.name}: ${preset.description}`} className="inline-flex h-8 items-center gap-1.5 rounded-full pl-3 text-xs font-medium">
        {active && <Check className="size-3.5" />}
        {preset.name}
      </button>
      {/* Hidden until hover or keyboard focus, so the row stays calm. */}
      <button
        onClick={onEdit}
        aria-label={`Edit ${preset.name}`}
        title={`Edit ${preset.name}`}
        className={cn(
          "ml-1 grid h-6 w-0 place-items-center overflow-hidden rounded-full opacity-0 transition-all group-hover/pill:mr-1 group-hover/pill:w-6 group-hover/pill:opacity-100 focus-visible:mr-1 focus-visible:w-6 focus-visible:opacity-100",
          active ? "bg-secondary hover:bg-foreground/10" : "bg-white/20 hover:bg-white/30",
        )}
      >
        <Pencil className="size-3" />
      </button>
      <span className="w-3 transition-all group-hover/pill:w-0" />
    </div>
  )
}

function Chip({ icon: Icon, children }: { icon: typeof Mic; children: React.ReactNode }) {
  return (
    <span className="inline-flex items-center gap-1.5 rounded-md bg-black/20 px-2 py-1 backdrop-blur">
      <Icon className="size-3" />
      {children}
    </span>
  )
}
