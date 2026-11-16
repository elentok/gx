import { useEffect, useRef } from "react"
import { RestKindBedtime } from "./generated/server"
import { useRestState } from "./rest"
import { showToast, toaster } from "./ui/toaster"

const toastId = (kind: string) => `rest-warning-${kind}`

// Prints in the server's zone so every device agrees. An unnamed zone falls back to its offset.
function clock(iso: string, zone?: string, offsetSeconds = 0): string {
  const ms = new Date(iso).getTime()
  const opts: Intl.DateTimeFormatOptions = { hour: "2-digit", minute: "2-digit" }
  if (zone) return new Date(ms).toLocaleTimeString("en-GB", { ...opts, timeZone: zone })
  return new Date(ms + offsetSeconds * 1000).toLocaleTimeString("en-GB", {
    ...opts,
    timeZone: "UTC",
  })
}

function leadText(minutes: number): string {
  return `${minutes} ${minutes === 1 ? "minute" : "minutes"}`
}

// Shows the server's Rest warning. One toast per kind: a moved start updates it in place. A closed
// toast stays closed until the warning ends and a new one begins.
export function RestWarningToast() {
  const { warning, now, zone, zoneOffsetSeconds } = useRestState()
  const kind = warning?.kind
  const start = warning?.start
  const breakMinutes = warning?.break_minutes
  // Minutes come from the server's clock; the dependency is the rounded value, not `now`, so the
  // toast only updates when the text changes.
  const lead =
    start && now
      ? Math.max(1, Math.round((Date.parse(start) - Date.parse(now)) / 60000))
      : undefined
  const closed = useRef(false)
  const active = useRef<string | undefined>(undefined)

  useEffect(() => {
    if (active.current && active.current !== kind) toaster.remove(toastId(active.current))
    if (active.current !== kind) closed.current = false
    active.current = kind
    if (!kind || !start || lead === undefined || closed.current) return
    const time = clock(start, zone, zoneOffsetSeconds)
    const bedtime = kind === RestKindBedtime
    showToast({
      id: toastId(kind),
      type: "warning",
      title: bedtime
        ? `Bedtime starts at ${time} (in ${leadText(lead)})`
        : `Break in ${leadText(lead)}`,
      description: bedtime
        ? "Wrap up what you are doing."
        : `Your Break starts at ${time} and lasts ${breakMinutes} minutes.`,
      persistent: true,
      card: true,
      onClose: () => {
        if (active.current === kind) closed.current = true
      },
    })
  }, [kind, start, breakMinutes, lead, zone, zoneOffsetSeconds])
  return null
}
