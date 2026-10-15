import { useEffect, useState } from "react"
import { CircleAlert, CircleCheck } from "lucide-react"
import { cn } from "@/lib/utils"

type Toast = { id: number; text: string; kind: "info" | "error" }
let listeners: ((t: Toast[]) => void)[] = []
let toasts: Toast[] = []
let next = 1

/** Shows a short message in the corner. Errors stay a little longer. */
export function toast(text: string, kind: Toast["kind"] = "info") {
  const t = { id: next++, text: text.replace(/^Error: /, ""), kind }
  toasts = [...toasts, t].slice(-4)
  listeners.forEach((l) => l(toasts))
  window.setTimeout(
    () => {
      toasts = toasts.filter((x) => x.id !== t.id)
      listeners.forEach((l) => l(toasts))
    },
    kind === "error" ? 6000 : 3000,
  )
}

export function Toaster() {
  const [items, setItems] = useState<Toast[]>(toasts)
  useEffect(() => {
    listeners.push(setItems)
    return () => {
      listeners = listeners.filter((l) => l !== setItems)
    }
  }, [])
  return (
    <div className="pointer-events-none fixed right-5 bottom-5 z-50 flex flex-col items-end gap-2">
      {items.map((t) => (
        <div
          key={t.id}
          className={cn(
            "animate-in fade-in slide-in-from-bottom-2 flex max-w-sm items-center gap-2 rounded-lg px-3.5 py-2.5 text-sm font-medium shadow-lg shadow-black/10",
            t.kind === "error" ? "bg-red-600 text-white" : "bg-foreground text-background",
          )}
        >
          {t.kind === "error" ? <CircleAlert className="size-4 shrink-0" /> : <CircleCheck className="size-4 shrink-0 text-wa" />}
          {t.text}
        </div>
      ))}
    </div>
  )
}
