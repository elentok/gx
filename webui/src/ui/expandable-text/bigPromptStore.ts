import { createExternalStore } from "../../shell/externalStore"

export const BIG_PROMPT_KEY = "gx.promptbox.big"

function readStored(): boolean {
  try {
    return localStorage.getItem(BIG_PROMPT_KEY) === "true"
  } catch {
    return false
  }
}

function writeStored(big: boolean): void {
  try {
    localStorage.setItem(BIG_PROMPT_KEY, String(big))
  } catch {
    // Private mode or blocked storage: big mode lasts until reload.
  }
}

// A second tab's change arrives as a storage event; follow it while any box is mounted.
const store = createExternalStore(readStored(), () => {
  const onStorage = (e: StorageEvent) => {
    if (e.key === BIG_PROMPT_KEY) store.set(e.newValue === "true")
  }
  window.addEventListener("storage", onStorage)
  return () => window.removeEventListener("storage", onStorage)
})

export const getBigPrompt = store.get

export function setBigPrompt(big: boolean): void {
  store.set(big)
  writeStored(big)
}

export function toggleBigPrompt(): void {
  setBigPrompt(!store.get())
}

export const useBigPrompt = store.use
