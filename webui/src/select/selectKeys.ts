export type SelectIntent = "enter-select" | "toggle" | "open" | "menu" | "leave" | "none"

/** The item menu's trigger; roving areas take it out of the Tab order, so a key has to reach it. */
export const MENU_TRIGGER = `button[aria-label="More actions"]`

/** Opens the item menu inside `scope`, if it has one. */
export function openItemMenu(scope: Element | null | undefined): boolean {
  const btn = scope?.querySelector<HTMLElement>(MENU_TRIGGER)
  btn?.click()
  return !!btn
}

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
    case ".":
      return "menu"
    case "F10":
      return e.shiftKey ? "menu" : "none"
    default:
      return "none"
  }
}

/** The key event the dispatcher acts on; a KeyboardEvent satisfies it. */
export type DispatchKey = SelectKey & { preventDefault(): void; stopPropagation(): void }

export interface SelectKeyHandlers {
  /** `s`: return false when there is nothing to select, so the key is left alone. */
  enter: () => boolean | void
  /** The select key of the focused item, or null when no item has focus. */
  itemKey: string | null
  toggle: (key: string) => void
  /** Enter outside select mode. Omit when the surface has no open action. */
  open?: (key: string) => void
  /** `.` or Shift+F10: open the focused item's menu. Return false when it has none, so the key is left alone. */
  menu?: (key: string) => boolean | void
}

/**
 * Runs the enter, toggle and open intents for a surface and claims the event when it acts. Leave
 * (Escape) is not handled here: useSelectApi owns it.
 */
export function dispatchSelectKey(
  e: DispatchKey,
  selecting: boolean,
  guarded: boolean,
  h: SelectKeyHandlers,
) {
  const intent = selectIntent(e, selecting, guarded)
  if (intent === "enter-select") {
    if (h.enter() === false) return
    e.preventDefault()
    return
  }
  if (!h.itemKey) return
  if (intent === "toggle") {
    e.preventDefault()
    e.stopPropagation()
    h.toggle(h.itemKey)
  } else if (intent === "open" && h.open) {
    e.preventDefault()
    e.stopPropagation()
    h.open(h.itemKey)
  } else if (intent === "menu" && h.menu) {
    if (h.menu(h.itemKey) === false) return
    e.preventDefault()
    e.stopPropagation()
  }
}
