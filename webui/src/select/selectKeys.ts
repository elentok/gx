export type SelectIntent = "enter-select" | "toggle" | "open" | "leave" | "none"

export interface SelectKey {
  key: string
  metaKey?: boolean
  ctrlKey?: boolean
  altKey?: boolean
  shiftKey?: boolean
}

/**
 * Maps a key to an intent. `guarded` is the component's check that a text field, a menu or a dialog
 * has focus (isTextTarget, hasLayer); it is input because those need a DOM.
 */
export function selectIntent(e: SelectKey, selecting: boolean, guarded: boolean): SelectIntent {
  if (guarded || e.metaKey || e.ctrlKey || e.altKey) return "none"
  switch (e.key) {
    case "s":
      return selecting ? "none" : "enter-select"
    case " ":
      return selecting ? "toggle" : "none"
    case "Enter":
      return selecting ? "toggle" : "open"
    case "Escape":
      return selecting ? "leave" : "none"
    default:
      return "none"
  }
}
