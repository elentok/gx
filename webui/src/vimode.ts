import { useSyncExternalStore } from "react"
import { useSnapshotField } from "./connection"

// Vi mode is per device, so it lives in localStorage and not on the server.
const KEY = "gx.vi-mode"

const listeners = new Set<() => void>()

// The storage event covers other tabs; `listeners` covers this one.
function subscribe(listener: () => void): () => void {
  listeners.add(listener)
  window.addEventListener("storage", listener)
  return () => {
    listeners.delete(listener)
    window.removeEventListener("storage", listener)
  }
}

// Storage can throw (private window, blocked site data); that reads as off.
function read(): boolean {
  try {
    return localStorage.getItem(KEY) === "on"
  } catch {
    return false
  }
}

export function setViMode(on: boolean): void {
  try {
    localStorage.setItem(KEY, on ? "on" : "off")
  } catch {
    return
  }
  for (const listener of listeners) listener()
}

export function useViMode(): boolean {
  return useSyncExternalStore(subscribe, read)
}

// Reads the key sequence (like "jk") that leaves insert mode; empty means off.
export function useViEscape(): string {
  return useSnapshotField((s) => s.vi_escape) ?? ""
}
