import { useRef, useState } from "react"
import { BookOpenText, ChevronDown, FileText, FileType2, Loader2, Trash2, Upload } from "lucide-react"
import { cn } from "@/lib/utils"
import { errText, harnessApi, type HarnessSnapshot, type PlaybookDoc } from "@/lib/api"
import { toast } from "../Toast"
import { Section, Toggle } from "./editors"

const ACCEPT = ".md,.markdown,.txt,.pdf"

/** Uploaded sales strategy John reads before every reply. */
export function Playbook({ snap, onChange }: { snap: HarnessSnapshot; onChange: (s: HarnessSnapshot) => void }) {
  const input = useRef<HTMLInputElement>(null)
  const [busy, setBusy] = useState(false)
  const [over, setOver] = useState(false)
  const used = snap.documents.filter((d) => d.active).reduce((n, d) => n + d.chars, 0)
  const pct = Math.min(100, Math.round((used / snap.playbook_max_chars) * 100))

  const upload = async (files: FileList | null) => {
    if (!files?.length) return
    setBusy(true)
    for (const f of Array.from(files)) {
      try {
        const next = await harnessApi.upload(f)
        onChange(next)
        const doc = next.documents.at(-1)
        toast(doc && !doc.active ? `${f.name} added but switched off: the playbook is full` : `John read ${f.name}`, doc && !doc.active ? "error" : "info")
      } catch (e) {
        toast(errText(e), "error")
      }
    }
    setBusy(false)
    if (input.current) input.current.value = ""
  }

  return (
    <Section title="Sales playbook" hint="Strategy John reads before every reply: objection handling, scripts, what to lead with." icon={BookOpenText}>
      <button
        onClick={() => input.current?.click()}
        onDragOver={(e) => {
          e.preventDefault()
          setOver(true)
        }}
        onDragLeave={() => setOver(false)}
        onDrop={(e) => {
          e.preventDefault()
          setOver(false)
          upload(e.dataTransfer.files)
        }}
        disabled={busy}
        className={cn(
          "flex w-full flex-col items-center gap-1.5 rounded-xl border-2 border-dashed px-4 py-6 text-center transition",
          over ? "border-foreground bg-secondary" : "border-border hover:border-foreground/30 hover:bg-secondary/60",
        )}
      >
        <span className="grid size-10 place-items-center rounded-full bg-secondary">{busy ? <Loader2 className="size-5 animate-spin" /> : <Upload className="size-5" />}</span>
        <span className="text-sm font-medium">{busy ? "Reading…" : "Drop a file or click to upload"}</span>
        <span className="text-xs text-muted-foreground">Markdown, text or PDF with selectable text · up to 15 MB</span>
      </button>
      <input ref={input} type="file" accept={ACCEPT} multiple hidden onChange={(e) => upload(e.target.files)} />

      {snap.documents.length > 0 && (
        <>
          <div className="mt-4 space-y-2">
            {snap.documents.map((d) => (
              <DocRow key={d.id} d={d} onChange={onChange} />
            ))}
          </div>
          <div className="mt-4">
            <div className="mb-1.5 flex justify-between text-[11px] text-muted-foreground">
              <span>Playbook length</span>
              <span className="tabular-nums">
                {Math.round(used / 1000)}k of {Math.round(snap.playbook_max_chars / 1000)}k characters
              </span>
            </div>
            <div className="h-1.5 overflow-hidden rounded-full bg-secondary">
              <div className={cn("h-full rounded-full transition-all", pct > 85 ? "bg-amber" : "bg-foreground/70")} style={{ width: `${pct}%` }} />
            </div>
          </div>
        </>
      )}
    </Section>
  )
}

function DocRow({ d, onChange }: { d: PlaybookDoc; onChange: (s: HarnessSnapshot) => void }) {
  const [open, setOpen] = useState(false)
  const [full, setFull] = useState<string | null>(null)
  const Icon = d.kind === "pdf" ? FileType2 : FileText
  const run = (p: Promise<HarnessSnapshot>, ok?: string) =>
    p.then((s) => (onChange(s), ok && toast(ok))).catch((e) => toast(errText(e), "error"))
  const expand = () => {
    setOpen((o) => !o)
    if (!full) harnessApi.docText(d.id).then((r) => setFull(r.text)).catch(() => setFull(d.preview))
  }
  return (
    <div className={cn("rounded-lg border", !d.active && "bg-secondary/40")}>
      <div className="flex items-center gap-3 px-3 py-2.5">
        <span className={cn("grid size-8 shrink-0 place-items-center rounded-md", d.kind === "pdf" ? "bg-red-50 text-red-600" : "bg-secondary text-foreground/70")}>
          <Icon className="size-4" />
        </span>
        <button onClick={expand} className="min-w-0 flex-1 text-left">
          <div className={cn("truncate text-sm font-medium", !d.active && "text-muted-foreground")}>{d.name}</div>
          <div className="text-[11px] text-muted-foreground">
            {d.words.toLocaleString()} words · {fmtBytes(d.bytes)}
            {!d.active && " · off"}
          </div>
        </button>
        <button onClick={expand} className="grid size-7 place-items-center rounded-md text-muted-foreground hover:bg-secondary" aria-label="Preview">
          <ChevronDown className={cn("size-4 transition", open && "rotate-180")} />
        </button>
        <Toggle on={d.active} onChange={(v) => run(harnessApi.setDocActive(d.id, v))} label={d.active ? "Switch off" : "Switch on"} />
        <button
          onClick={() => window.confirm(`Remove ${d.name} from John's playbook?`) && run(harnessApi.removeDoc(d.id), `${d.name} removed`)}
          className="grid size-7 place-items-center rounded-md text-muted-foreground hover:bg-red-50 hover:text-red-600"
          aria-label="Remove"
          title="Remove"
        >
          <Trash2 className="size-4" />
        </button>
      </div>
      {open && (
        <pre className="max-h-72 overflow-y-auto border-t bg-secondary/40 px-4 py-3 font-mono text-[12px] leading-relaxed whitespace-pre-wrap text-foreground/80">
          {full ?? d.preview}
        </pre>
      )}
    </div>
  )
}

function fmtBytes(n: number) {
  return n < 1024 ? `${n} B` : n < 1024 * 1024 ? `${Math.round(n / 1024)} KB` : `${(n / 1024 / 1024).toFixed(1)} MB`
}
