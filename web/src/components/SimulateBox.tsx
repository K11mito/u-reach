import { useState } from "react"
import { FlaskConical } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { api } from "@/lib/api"

/** Runs the real pipeline for a pretend lead; nothing is sent to WhatsApp. */
export function SimulateBox() {
  const [open, setOpen] = useState(false)
  const [name, setName] = useState("Raj")
  const [text, setText] = useState("")
  const go = async () => {
    if (!text.trim()) return
    const phone = "99" + String([...name].reduce((a, c) => a * 31 + c.charCodeAt(0), 7) % 1e10).padStart(10, "0")
    await api.simulate(name, phone, text.trim())
    setText("")
  }
  if (!open)
    return (
      <button onClick={() => setOpen(true)} className="flex items-center gap-2 border-t px-4 py-2.5 text-xs text-muted-foreground hover:bg-muted/60">
        <FlaskConical className="size-3.5" /> Simulate a lead message
      </button>
    )
  return (
    <div className="space-y-2 border-t bg-muted/40 p-3">
      <div className="flex items-center justify-between text-xs font-semibold text-muted-foreground">
        <span className="flex items-center gap-1.5">
          <FlaskConical className="size-3.5" /> Simulate (no WhatsApp send)
        </span>
        <button onClick={() => setOpen(false)} className="hover:text-foreground">
          close
        </button>
      </div>
      <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="Lead name" className="h-8 bg-card" />
      <div className="flex gap-2">
        <Input value={text} onChange={(e) => setText(e.target.value)} onKeyDown={(e) => e.key === "Enter" && go()} placeholder="hey John, what is U.Center?" className="h-8 bg-card" />
        <Button size="sm" onClick={go} disabled={!text.trim()}>
          Send
        </Button>
      </div>
    </div>
  )
}
