import type { DiffView } from "./DiffBody"

const KEY = "gx.diff.view"

// Storage is a convenience; the page works without it.
export function readView(): DiffView {
  try {
    return localStorage.getItem(KEY) === "split" ? "split" : "unified"
  } catch {
    return "unified"
  }
}

export function writeView(v: DiffView) {
  try {
    localStorage.setItem(KEY, v)
  } catch {
    // See above.
  }
}
