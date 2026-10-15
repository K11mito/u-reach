import { useEffect, useRef, useState } from "react"
import { FlaskConical, Mic, Send, Square, X } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { cn } from "@/lib/utils"
import { api } from "@/lib/api"
import { toast } from "./Toast"

// A stable fake number per name, so repeated tests land in the same chat.
const fakePhone = (name: string) => "99" + String([...name.toLowerCase()].reduce((a, c) => (a * 31 + c.charCodeAt(0)) % 1e10, 7)).padStart(10, "0")

/** Runs the real pipeline for a pretend lead; nothing is sent to WhatsApp. */
export function SimulateBox() {
  const [open, setOpen] = useState(false)
  const [name, setName] = useState("Raj")
  const [text, setText] = useState("")
  const [recording, setRecording] = useState(false)
  const [secs, setSecs] = useState(0)
  const rec = useRef<MediaRecorder | null>(null)
  const timer = useRef<number | undefined>(undefined)

  useEffect(() => () => window.clearInterval(timer.current), [])

  const sendText = async () => {
    if (!text.trim()) return
    try {
      await api.simulate(name, fakePhone(name), text.trim())
      setText("")
    } catch (e) {
      toast(`Simulate failed: ${e}`, "error")
    }
  }

  const startRec = async () => {
    try {
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true })
      const r = new MediaRecorder(stream)
      const chunks: Blob[] = []
      r.ondataavailable = (e) => chunks.push(e.data)
      r.onstop = async () => {
        stream.getTracks().forEach((t) => t.stop())
        window.clearInterval(timer.current)
        setRecording(false)
        const blob = new Blob(chunks, { type: r.mimeType })
        if (blob.size < 2000) return toast("Too short, hold on a bit longer", "error")
        try {
          await api.simulateVoice(name, fakePhone(name), blob)
          toast(`Voice note from ${name} sent to the pipeline`)
        } catch (e) {
          toast(`Simulate failed: ${e}`, "error")
        }
      }
      r.start()
      rec.current = r
      setSecs(0)
      setRecording(true)
      timer.current = window.setInterval(() => setSecs((s) => s + 1), 1000)
    } catch {
      toast("Microphone not available (allow it in the browser)", "error")
    }
  }

  if (!open)
    return (
      <button onClick={() => setOpen(true)} className="flex items-center gap-2 border-t px-4 py-2.5 text-xs font-medium text-muted-foreground transition hover:bg-muted/60 hover:text-foreground">
        <FlaskConical className="size-3.5" /> Simulate a lead message
      </button>
    )
  return (
    <div className="space-y-2 border-t bg-muted/40 p-3">
      <div className="flex items-center justify-between text-xs font-medium text-muted-foreground">
        <span className="flex items-center gap-1.5">
          <FlaskConical className="size-3.5" /> Simulate a lead (nothing goes to WhatsApp)
        </span>
        <button onClick={() => setOpen(false)} className="rounded p-0.5 hover:bg-muted hover:text-foreground" aria-label="Close">
          <X className="size-3.5" />
        </button>
      </div>
      <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="Lead name" className="h-8 bg-card" />
      <div className="flex gap-2">
        {recording ? (
          <div className="flex h-8 flex-1 items-center gap-2 rounded-md bg-red-50 px-3 text-xs font-medium text-red-600 ring-1 ring-red-200">
            <span className="size-2 animate-pulse rounded-full bg-red-500" /> Recording… 0:{String(secs).padStart(2, "0")}
          </div>
        ) : (
          <Input
            value={text}
            onChange={(e) => setText(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && sendText()}
            placeholder="hey John, what is U.Center?"
            className="h-8 bg-card"
          />
        )}
        {text.trim() && !recording ? (
          <Button size="sm" onClick={sendText} aria-label="Send text">
            <Send />
          </Button>
        ) : (
          <Button
            size="sm"
            variant={recording ? "destructive" : "outline"}
            onClick={() => (recording ? rec.current?.stop() : startRec())}
            className={cn(!recording && "bg-card")}
            aria-label={recording ? "Stop and send voice note" : "Record a voice note"}
            title={recording ? "Stop and send" : "Record a voice note"}
          >
            {recording ? <Square /> : <Mic />}
          </Button>
        )}
      </div>
    </div>
  )
}
