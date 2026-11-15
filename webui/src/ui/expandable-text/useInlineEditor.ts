import { useEffect, useLayoutEffect, useRef, useState, type RefObject } from "react"
import type CodeEditor from "./CodeEditor"
import { loadCodeEditor, type Selection } from "./LargeEditor"
import { useCoarsePointer } from "../../useWide"
import { useViMode } from "../../vimode"

/**
 * With vi on and a fine pointer the box wants the vi editor. Until its chunk has loaded `showEditor` stays
 * false and the textarea stays. Both hooks run every render: `&&` would skip one when vi turns off and break
 * the hook order.
 */
export function useInlineEditor() {
  const viOn = useViMode()
  const coarse = useCoarsePointer()
  const wantEditor = viOn && !coarse
  const [Editor, setEditor] = useState<typeof CodeEditor | null>(null)
  useEffect(() => {
    if (!wantEditor) return
    let live = true
    void loadCodeEditor().then((m) => live && setEditor(() => m.default))
    return () => {
      live = false
    }
  }, [wantEditor])
  return { wantEditor, Editor, showEditor: wantEditor && Editor !== null }
}

/**
 * The textarea/editor swap carries the selection and focus. They are read during the render that swaps, while
 * the old element is still in the DOM.
 */
export function useSwapCarry(showEditor: boolean, area: RefObject<HTMLTextAreaElement | null>) {
  const wrap = useRef<HTMLDivElement>(null)
  const editorSel = useRef<Selection>({ start: 0, end: 0 })
  const carry = useRef<{ sel: Selection; focus: boolean } | null>(null)
  const read = () =>
    area.current
      ? {
          sel: { start: area.current.selectionStart, end: area.current.selectionEnd },
          focus: document.activeElement === area.current,
        }
      : {
          sel: editorSel.current,
          focus: !!wrap.current?.contains(document.activeElement),
        }
  const shown = useRef(showEditor)
  // A textarea that comes back from the editor must not autofocus: it would take focus from the large editor over it.
  const swapped = useRef(false)
  if (shown.current !== showEditor) {
    shown.current = showEditor
    swapped.current = true
    carry.current = read()
    editorSel.current = carry.current.sel
  }
  useLayoutEffect(() => {
    const c = carry.current
    if (showEditor || !c || !area.current) return
    carry.current = null
    area.current.setSelectionRange(c.sel.start, c.sel.end)
    if (c.focus) area.current.focus()
  }, [showEditor, area])
  return { wrap, editorSel, carry, swapped, read }
}
