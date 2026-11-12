import { useSyncExternalStore } from "react"

export type WorkerRegistration =
  | { state: "idle" }
  | { state: "registered" }
  | { state: "failed"; error: string }

let current: WorkerRegistration = { state: "idle" }
const listeners = new Set<() => void>()

function set(next: WorkerRegistration) {
  current = next
  listeners.forEach((l) => l())
}

export function getWorkerRegistration(): WorkerRegistration {
  return current
}

export function useWorkerRegistration(): WorkerRegistration {
  return useSyncExternalStore((l) => {
    listeners.add(l)
    return () => listeners.delete(l)
  }, getWorkerRegistration)
}

// Registers /sw.js. The dev server has no worker, and a failure must never
// reach the user; it is kept so the root-trust hint can read a SecurityError.
export async function registerWorker(): Promise<void> {
  if (import.meta.env.DEV || !window.isSecureContext || !("serviceWorker" in navigator)) return
  try {
    await navigator.serviceWorker.register("/sw.js")
    set({ state: "registered" })
  } catch (err) {
    set({ state: "failed", error: (err as { name?: string } | null)?.name ?? "Error" })
  }
}

export function resetWorkerRegistrationForTest() {
  set({ state: "idle" })
}
