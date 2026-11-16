import { useEffect } from "react"
import { apiFetch } from "./apiFetch"
import { useConnectionContext, useConnectionState } from "./connection"
import type {
  RestPeriod,
  RestResult,
  RestRule,
  RestUnlockRequest,
  RestWarning,
} from "./generated/server"
import { guarded } from "./ui/reportWriteFailure"
import { WriteError } from "./useWrite"

export const HEARTBEAT_INTERVAL_MS = 60_000

const INPUT_EVENTS = ["keydown", "pointerdown", "scroll"] as const

// Sends one active minute. The server decides whether it counts.
export async function postRestActive(): Promise<void> {
  const res = await apiFetch("/v1/rest/active", { method: "POST" })
  if (!res.ok)
    throw new WriteError((await res.text().catch(() => "")).trim() || `HTTP ${res.status}`)
}

// Same pattern the server checks. The server stays the judge; this only enables the button.
const UNLOCK_SENTENCE = /^\s*i['’]m\s+.+?\s+and\s+i['’]m\s+addicted\s+to\s+ai\s*$/i

export function isUnlockSentence(text: string): boolean {
  return UNLOCK_SENTENCE.test(text)
}

// Returns the refusal message, or undefined when the unlock went through.
export async function postRestUnlock(sentence: string): Promise<string | undefined> {
  const res = await apiFetch("/v1/rest/unlock", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ sentence } satisfies RestUnlockRequest),
  })
  if (!res.ok)
    throw new WriteError((await res.text().catch(() => "")).trim() || `HTTP ${res.status}`)
  const result = (await res.json()) as RestResult
  return result.refused ? result.message || result.reason || "Unlock refused" : undefined
}

// Input only sets a flag. The once-a-minute timer runs while the tab is visible, and sends a
// heartbeat only if input came in since the last tick.
export function useHeartbeat(on: boolean): void {
  useEffect(() => {
    if (!on) return
    let input = false
    let timer: ReturnType<typeof setInterval> | undefined
    const mark = () => {
      input = true
    }
    const tick = () => {
      if (!input) return
      input = false
      guarded(postRestActive())
    }
    const start = () => {
      if (timer === undefined) timer = setInterval(tick, HEARTBEAT_INTERVAL_MS)
    }
    const stop = () => {
      clearInterval(timer)
      timer = undefined
    }
    const onVisibility = () => (document.visibilityState === "hidden" ? stop() : start())
    for (const e of INPUT_EVENTS) window.addEventListener(e, mark, { passive: true, capture: true })
    document.addEventListener("visibilitychange", onVisibility)
    if (document.visibilityState !== "hidden") start()
    return () => {
      stop()
      for (const e of INPUT_EVENTS) window.removeEventListener(e, mark, { capture: true })
      document.removeEventListener("visibilitychange", onVisibility)
    }
  }, [on])
}

export type RestState = {
  counting: boolean
  period?: RestPeriod
  warning?: RestWarning
  rule?: RestRule
  // The server's clock from the snapshot, so a countdown never uses the browser's.
  now?: string
}

export function useRestState(): RestState {
  const rest = useConnectionState(useConnectionContext()).snapshot?.rest
  return {
    counting: rest?.counting ?? false,
    period: rest?.period,
    warning: rest?.warning,
    rule: rest?.rule,
    now: rest?.now,
  }
}

// Mounted once in App. The Rest screen shows while a period runs, so nothing counts then.
export function useRestHeartbeat(): void {
  const { counting, period } = useRestState()
  useHeartbeat(counting && !period)
}
