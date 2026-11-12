import { isMac } from "../platform"

/** True when the key event's target is somewhere the user types: shortcuts stand down there (except Mod+K). */
export function isTextTarget(el: EventTarget | null): boolean {
  if (!(el instanceof HTMLElement)) return false
  if (el.isContentEditable || ["INPUT", "TEXTAREA", "SELECT"].includes(el.tagName)) return true
  // The CodeMirror surface is contenteditable in a browser; jsdom does not compute that, so match its class too.
  return el.closest(".cm-editor") !== null
}

/** The digit 1 to 9 of an Alt chord. Read from `code`: Mac Option rewrites `key` (Option+1 gives "¡"). */
export function digitOf(e: Pick<KeyboardEvent, "code">): number | null {
  const m = /^Digit([1-9])$/.exec(e.code)
  return m ? Number(m[1]) : null
}

/** Cmd on Mac, Ctrl elsewhere. */
export function isMod(e: Pick<KeyboardEvent, "metaKey" | "ctrlKey">): boolean {
  return isMac() ? e.metaKey : e.ctrlKey
}
