import { useSyncExternalStore } from "react"
import { isIOS, isMac, isStandalone } from "../platform"

export type InstallMode = "prompt" | "ios" | "mac-safari" | "none"
export type InstallState = { mode: InstallMode; desktopCaption: boolean }

type PromptEvent = Event & {
  prompt: () => Promise<void>
  userChoice: Promise<{ outcome: "accepted" | "dismissed" }>
}

// A saved event can be prompted only once, so it is dropped as soon as it is used.
let saved: PromptEvent | null = null
let installed = false
const NONE: InstallState = Object.freeze({ mode: "none", desktopCaption: false })
const listeners = new Set<() => void>()

function compute(): InstallState {
  if (installed || isStandalone()) return NONE
  if (saved) {
    const ua = navigator.userAgent
    return { mode: "prompt", desktopCaption: /Chrome|Edg/.test(ua) && !/Android|Mobile/.test(ua) }
  }
  if (isIOS()) return { ...NONE, mode: "ios" }
  const ua = navigator.userAgent
  if (isMac() && /Safari/.test(ua) && !/Chrome|Chromium|Edg|OPR|Firefox/.test(ua))
    return { ...NONE, mode: "mac-safari" }
  return NONE
}

// Node-environment tests import this module without a window.
let current: InstallState = typeof window === "undefined" ? NONE : compute()

/** Recomputes the state and wakes subscribers only when it changed, so the snapshot stays stable. */
function refresh() {
  const next = compute()
  if (next.mode === current.mode && next.desktopCaption === current.desktopCaption) return
  current = next
  listeners.forEach((l) => l())
}

export function getInstallState(): InstallState {
  return current
}

export async function promptInstall(): Promise<void> {
  const event = saved
  if (!event) return
  saved = null
  refresh()
  await event.prompt()
  if ((await event.userChoice).outcome === "accepted") {
    installed = true
    refresh()
  }
}

if (typeof window !== "undefined") {
  window.addEventListener("beforeinstallprompt", (e) => {
    e.preventDefault()
    saved = e as PromptEvent
    refresh()
  })
  window.addEventListener("appinstalled", () => {
    installed = true
    saved = null
    refresh()
  })
}

function subscribe(l: () => void) {
  listeners.add(l)
  return () => listeners.delete(l)
}

export function useInstall(): InstallState & { promptInstall: () => Promise<void> } {
  const state = useSyncExternalStore(subscribe, getInstallState)
  return { ...state, promptInstall }
}
