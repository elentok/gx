import { useCallback, useEffect, useRef } from "react"

// Mounted layers, oldest first. Module-level so a Modal and the editor's jump menu share one order.
const stack: object[] = []

/** Whether any layer is open right now. Call at event time. */
export const hasLayer = (): boolean => stack.length > 0

/** Marks a dialog's first text field, so focus can be handed back to it. Set by useFocusFirstField. */
export const FIRST_FIELD_ATTR = "data-first-field"

/**
 * Moves focus into the topmost open dialog: its first text field where it marks one, else the panel.
 * Does nothing when no dialog is open or `from` (the node focus landed on) is already inside it.
 */
export function focusTopDialog(from?: Node | null): void {
  const dialog = [...document.querySelectorAll<HTMLElement>('[aria-modal="true"]')].at(-1)
  if (!dialog || (from && dialog.contains(from))) return
  ;(dialog.querySelector<HTMLElement>(`[${FIRST_FIELD_ATTR}]`) ?? dialog).focus()
}

/**
 * Registers the calling layer while it is mounted and `active`. Returns a function to call at
 * event time that says whether this layer is the topmost.
 */
export function useIsTopLayer(active = true): () => boolean {
  const token = useRef({})
  useEffect(() => {
    if (!active) return
    const mine = token.current
    stack.push(mine)
    return () => {
      stack.splice(stack.indexOf(mine), 1)
    }
  }, [active])
  return useCallback(() => stack.at(-1) === token.current, [])
}
