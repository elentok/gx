import { useEffect, useRef } from "react"
import { RestKindBedtime } from "./generated/server"
import { useRestState } from "./rest"
import { showToast, toaster } from "./ui/toaster"

const toastId = (kind: string) => `rest-warning-${kind}`

function clock(iso: string): string {
  return new Date(iso).toLocaleTimeString("en-GB", { hour: "2-digit", minute: "2-digit" })
}

// Shows the server's Rest warning. One toast per kind: a moved start updates it in place. A closed
// toast stays closed until the warning ends and a new one begins.
export function RestWarningToast() {
  const { warning } = useRestState()
  const kind = warning?.kind
  const start = warning?.start
  const breakMinutes = warning?.break_minutes
  const closed = useRef(false)
  const active = useRef<string | undefined>(undefined)

  useEffect(() => {
    if (active.current && active.current !== kind) toaster.remove(toastId(active.current))
    if (active.current !== kind) closed.current = false
    active.current = kind
    if (!kind || !start || closed.current) return
    const time = clock(start)
    const bedtime = kind === RestKindBedtime
    showToast({
      id: toastId(kind),
      type: "warning",
      title: bedtime ? `Bedtime starts at ${time} (in 10 minutes)` : "Break in 2 minutes",
      description: bedtime
        ? "Wrap up what you are doing."
        : `Your Break starts at ${time} and lasts ${breakMinutes} minutes.`,
      persistent: true,
      card: true,
      onClose: () => {
        if (active.current === kind) closed.current = true
      },
    })
  }, [kind, start, breakMinutes])
  return null
}
