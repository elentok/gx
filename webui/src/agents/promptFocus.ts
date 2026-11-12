import { useEffect, type RefObject } from "react"
import { useLocation } from "react-router"

/** Navigation state that asks the chat page it lands on to focus its prompt. A click carries none. */
export const FOCUS_PROMPT_STATE = { focusPrompt: true }

/** Focuses the textarea inside `container` when this location was opened with a focus request. */
export function usePromptFocus(container: RefObject<HTMLElement | null>) {
  const { key, state } = useLocation()
  const wanted = (state as { focusPrompt?: boolean } | null)?.focusPrompt === true
  useEffect(() => {
    if (wanted) container.current?.querySelector("textarea")?.focus()
  }, [container, key, wanted])
}
