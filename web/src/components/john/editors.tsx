import { useState, type ReactNode } from "react"
import { ArrowLeft, Check, Loader2, Plus, X } from "lucide-react"
import { cn } from "@/lib/utils"

/** A titled card on the John page. `hint` says how the AI uses it. */
export function Section({ title, hint, icon: Icon, action, children, className }: { title: string; hint?: string; icon?: typeof Plus; action?: ReactNode; children: ReactNode; className?: string }) {
  return (
    <section className={cn("rounded-xl border bg-card p-5", className)}>
      <div className="mb-4 flex items-start gap-3">
        {Icon && (
          <span className="grid size-8 shrink-0 place-items-center rounded-lg bg-secondary text-foreground/70">
            <Icon className="size-4" />
          </span>
        )}
        <div className="min-w-0 flex-1">
          <h3 className="font-semibold tracking-tight">{title}</h3>
          {hint && <p className="mt-0.5 text-xs text-muted-foreground">{hint}</p>}
        </div>
        {action}
      </div>
      {children}
    </section>
  )
}

/** A small label above a field. */
export function Label({ children, hint }: { children: ReactNode; hint?: string }) {
  return (
    <div className="mb-1.5 flex items-baseline justify-between gap-2">
      <span className="text-xs font-medium text-foreground/80">{children}</span>
      {hint && <span className="text-[11px] text-muted-foreground">{hint}</span>}
    </div>
  )
}

const fieldCls =
  "w-full rounded-lg border border-input bg-background px-3 py-2 text-sm outline-none transition placeholder:text-muted-foreground/70 focus:border-ring focus:ring-[3px] focus:ring-ring/30"

/** A text field that grows with its content. */
export function TextField({ value, onChange, placeholder, rows = 1, mono }: { value: string; onChange: (v: string) => void; placeholder?: string; rows?: number; mono?: boolean }) {
  return (
    <textarea
      value={value}
      onChange={(e) => onChange(e.target.value)}
      placeholder={placeholder}
      rows={rows}
      className={cn(fieldCls, "field-sizing-content resize-none leading-relaxed", mono && "font-mono text-[13px]")}
    />
  )
}

/** An editable list of sentences: one row each, add and remove. */
export function ListEditor({ items, onChange, placeholder, addLabel = "Add" }: { items: string[]; onChange: (v: string[]) => void; placeholder?: string; addLabel?: string }) {
  const set = (i: number, v: string) => onChange(items.map((x, j) => (j === i ? v : x)))
  return (
    <div className="space-y-2">
      {items.map((it, i) => (
        <div key={i} className="group flex items-start gap-2">
          <span className="mt-[15px] size-1.5 shrink-0 rounded-full bg-foreground/25" />
          <TextField value={it} onChange={(v) => set(i, v)} placeholder={placeholder} />
          <button
            onClick={() => onChange(items.filter((_, j) => j !== i))}
            className="mt-1.5 grid size-7 shrink-0 place-items-center rounded-md text-muted-foreground opacity-40 transition group-hover:opacity-100 hover:bg-secondary hover:text-foreground"
            title="Remove"
            aria-label="Remove"
          >
            <X className="size-4" />
          </button>
        </div>
      ))}
      <button onClick={() => onChange([...items, ""])} className="inline-flex items-center gap-1.5 rounded-md px-2 py-1 text-xs font-medium text-muted-foreground hover:bg-secondary hover:text-foreground">
        <Plus className="size-3.5" /> {addLabel}
      </button>
    </div>
  )
}

/** Short phrases as removable chips, with an input to add more. */
export function ChipEditor({ items, onChange, placeholder }: { items: string[]; onChange: (v: string[]) => void; placeholder?: string }) {
  const [draft, setDraft] = useState("")
  const add = () => {
    const v = draft.trim()
    if (v && !items.includes(v)) onChange([...items, v])
    setDraft("")
  }
  return (
    <div className="flex flex-wrap items-center gap-1.5 rounded-lg border border-input bg-background p-2 focus-within:border-ring focus-within:ring-[3px] focus-within:ring-ring/30">
      {items.map((it) => (
        <span key={it} className="inline-flex items-center gap-1 rounded-md bg-secondary py-1 pr-1 pl-2.5 text-xs font-medium">
          {it}
          <button onClick={() => onChange(items.filter((x) => x !== it))} className="grid size-4 place-items-center rounded text-muted-foreground hover:bg-foreground/10 hover:text-foreground" aria-label={`Remove ${it}`}>
            <X className="size-3" />
          </button>
        </span>
      ))}
      <input
        value={draft}
        onChange={(e) => setDraft(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Enter") {
            e.preventDefault()
            add()
          } else if (e.key === "Backspace" && !draft && items.length) onChange(items.slice(0, -1))
        }}
        onBlur={add}
        placeholder={placeholder}
        className="min-w-40 flex-1 bg-transparent px-1 py-1 text-sm outline-none placeholder:text-muted-foreground/70"
      />
    </div>
  )
}

/** Pick one of a few options, each with a short explanation. */
export function Choice<T extends string>({ value, onChange, options }: { value: T; onChange: (v: T) => void; options: { value: T; label: string; hint?: string }[] }) {
  return (
    <div className="grid gap-2" style={{ gridTemplateColumns: `repeat(${options.length}, minmax(0, 1fr))` }}>
      {options.map((o) => (
        <button
          key={o.value}
          onClick={() => onChange(o.value)}
          className={cn(
            "rounded-lg border px-3 py-2.5 text-left transition",
            value === o.value ? "border-foreground bg-foreground text-background" : "bg-background hover:border-foreground/30",
          )}
        >
          <div className="text-sm font-medium">{o.label}</div>
          {o.hint && <div className={cn("mt-0.5 text-[11px] leading-snug", value === o.value ? "text-background/70" : "text-muted-foreground")}>{o.hint}</div>}
        </button>
      ))}
    </div>
  )
}

/** An on/off switch. */
export function Toggle({ on, onChange, label }: { on: boolean; onChange: (v: boolean) => void; label: string }) {
  return (
    <button
      role="switch"
      aria-checked={on}
      aria-label={label}
      title={label}
      onClick={() => onChange(!on)}
      className={cn("relative h-5 w-9 shrink-0 rounded-full transition", on ? "bg-wa" : "bg-foreground/15")}
    >
      <span className={cn("absolute top-0.5 size-4 rounded-full bg-white shadow transition-all", on ? "left-[18px]" : "left-0.5")} />
    </button>
  )
}

/** The top bar of a John sub-page: back, title, save state. */
export function EditorBar({
  onBack,
  title,
  extra,
  dirty,
  saving,
  onSave,
  onDiscard,
}: {
  onBack: () => void
  title: ReactNode
  extra?: ReactNode
  dirty: boolean
  saving: boolean
  onSave: () => void
  onDiscard: () => void
}) {
  return (
    <div className={cn("rounded-xl border bg-card p-3 transition-colors", dirty && "border-amber/60 bg-amber/[0.06]")}>
      <div className="flex items-center gap-3">
        <button onClick={onBack} className="inline-flex h-8 items-center gap-1.5 rounded-lg px-2.5 text-sm font-medium text-muted-foreground hover:bg-secondary hover:text-foreground">
          <ArrowLeft className="size-4" /> John
        </button>
        <span className="h-6 w-px bg-border" />
        <div className="min-w-0 flex-1">{title}</div>
        {extra}
        {dirty ? (
          <>
            <span className="hidden items-center gap-1.5 text-xs font-medium text-amber-900 lg:inline-flex">
              <span className="size-2 rounded-full bg-amber" /> Unsaved
            </span>
            <button onClick={onDiscard} className="h-8 rounded-lg px-3 text-sm font-medium hover:bg-secondary">
              Discard
            </button>
            <button onClick={onSave} disabled={saving} className="inline-flex h-8 items-center gap-1.5 rounded-lg bg-primary px-3.5 text-sm font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-60">
              {saving ? <Loader2 className="size-4 animate-spin" /> : <Check className="size-4" />} Save
            </button>
          </>
        ) : (
          <span className="inline-flex items-center gap-1.5 px-2 text-xs text-muted-foreground">
            <Check className="size-3.5" /> Saved
          </span>
        )}
      </div>
    </div>
  )
}
