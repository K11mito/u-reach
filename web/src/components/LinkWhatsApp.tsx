import { useEffect, useState } from "react"
import { CheckCircle2, Loader2, X } from "lucide-react"
import type { WhatsAppStatus } from "@/lib/api"

/** Shows the pairing QR so the demo phone can be linked without a terminal. */
export function LinkWhatsApp({ wa, onClose }: { wa: WhatsAppStatus; onClose: () => void }) {
  const [tick, setTick] = useState(0)
  // WhatsApp rotates the QR every ~20 s; refresh the image often.
  useEffect(() => {
    const t = window.setInterval(() => setTick((n) => n + 1), 3000)
    return () => window.clearInterval(t)
  }, [])
  useEffect(() => {
    if (!wa.connected) return
    const t = window.setTimeout(onClose, 1800)
    return () => window.clearTimeout(t)
  }, [wa.connected, onClose])
  useEffect(() => {
    const esc = (e: KeyboardEvent) => e.key === "Escape" && onClose()
    window.addEventListener("keydown", esc)
    return () => window.removeEventListener("keydown", esc)
  }, [onClose])

  return (
    <div className="fixed inset-0 z-40 grid place-items-center bg-black/40 backdrop-blur-sm" onClick={onClose}>
      <div className="animate-in fade-in zoom-in-95 relative w-[420px] rounded-3xl bg-card p-7 shadow-2xl" onClick={(e) => e.stopPropagation()}>
        <button onClick={onClose} className="absolute top-4 right-4 rounded-full p-1 text-muted-foreground hover:bg-muted" aria-label="Close">
          <X className="size-4" />
        </button>
        <h2 className="text-xl font-extrabold">Link John's WhatsApp</h2>
        <p className="mt-1 text-sm text-muted-foreground">On the demo phone: WhatsApp → Settings → Linked devices → Link a device, then scan.</p>
        <div className="mt-5 grid aspect-square place-items-center overflow-hidden rounded-2xl border bg-white">
          {wa.connected ? (
            <div className="text-center text-wa-dark">
              <CheckCircle2 className="mx-auto size-14" />
              <div className="mt-2 font-bold">Linked as +{wa.phone}</div>
            </div>
          ) : wa.logged_out ? (
            <p className="px-8 text-center text-sm text-muted-foreground">This session was logged out from the phone. Restart the server to get a fresh QR code.</p>
          ) : wa.has_qr ? (
            <img src={`/api/whatsapp/qr.png?t=${tick}`} alt="WhatsApp pairing QR code" className="size-full object-contain p-2 [image-rendering:pixelated]" />
          ) : (
            <div className="flex items-center gap-2 text-sm text-muted-foreground">
              <Loader2 className="size-4 animate-spin" /> Getting a QR code from WhatsApp…
            </div>
          )}
        </div>
        <p className="mt-4 text-xs text-muted-foreground">The session is saved, so restarts won't ask again. Use a spare number, never a personal one.</p>
      </div>
    </div>
  )
}
