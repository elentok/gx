import { defaultKeymap, history, historyKeymap } from "@codemirror/commands"
import { Compartment, EditorState, Prec, type Extension } from "@codemirror/state"
import { drawSelection, EditorView, keymap, placeholder as placeholderExt } from "@codemirror/view"
import { getCM, vim, Vim } from "@replit/codemirror-vim"
import { Box } from "@chakra-ui/react"
import {
  useEffect,
  useImperativeHandle,
  useRef,
  type CSSProperties,
  type MutableRefObject,
  type Ref,
  type RefObject,
} from "react"
import type { MenuKey } from "../../commands/useCompletion"
import type { Selection } from "./LargeEditor"
import { useViEscape } from "../../vimode"
import { fieldLook, PROMPT_BOX_ATTR } from "./look"

export type ViMode = "INSERT" | "NORMAL" | "VISUAL"

/**
 * The editor's host carries its vi mode here while vi is on. InterruptControl reads it by this name from the
 * DOM, which keeps it free of CodeMirror and lets it judge an Esc before vim handles it.
 */
const VI_MODE_ATTR = "data-vi-mode"

/** What the host can ask of the editor. */
export type CodeEditorHandle = {
  /** Sends vi Escape and keeps focus in the editor. */
  escape: () => void
  /** Focuses the editor and puts the selection back. `leaveVisual` first sends vi Escape, so VISUAL comes back as NORMAL with a caret at the selection's end. */
  restore: (selection: Selection, leaveVisual: boolean) => void
}

// The vim registry is module-level, so ZZ finds its editor's callback through the view it fires in.
const closers = new WeakMap<object, () => void>()
let mapped = false

function mapOnce() {
  if (mapped) return
  mapped = true
  for (const ctx of ["normal", "visual"]) {
    // Screen lines, not paragraphs.
    Vim.map("j", "gj", ctx)
    Vim.map("k", "gk", ctx)
    Vim.map("gs", "0", ctx)
    // Replaces vim's "end of previous word".
    Vim.map("ge", "$", ctx)
  }
  Vim.defineAction("closeEditor", (cm: object) => closers.get(cm)?.())
  Vim.mapCommand("ZZ", "action", "closeEditor", {}, { context: "normal" })
}

// The vim registry is global, so the configured escape sequence is too: remap only when it changes.
let escapeKeys = ""

function mapEscape(keys: string) {
  if (keys === escapeKeys) return
  if (escapeKeys) Vim.unmap(escapeKeys, "insert")
  escapeKeys = keys
  if (keys) Vim.map(keys, "<Esc>", "insert")
}

function modeOf(mode: string): ViMode {
  if (mode === "visual") return "VISUAL"
  return mode === "insert" || mode === "replace" ? "INSERT" : "NORMAL"
}

const theme = EditorView.theme({
  "&": {
    height: "100%",
    fontSize: fieldLook.fontSize,
    color: "inherit",
    backgroundColor: "transparent",
  },
  "&.cm-focused": { outline: "none" },
  ".cm-cursor": { borderLeftColor: "currentColor" },
  // CodeMirror's defaults are light (lavender selection, white panel), which hides the light text. Use the theme tokens.
  ".cm-selectionBackground": { background: "var(--chakra-colors-sidebar-current)" },
  "&.cm-focused > .cm-scroller > .cm-selectionLayer .cm-selectionBackground": {
    background: "var(--chakra-colors-sidebar-selected)",
  },
  ".cm-content ::selection": { background: "var(--chakra-colors-sidebar-selected)" },
  // The vim message and command line under the text: a quiet status line, never the error red.
  ".cm-panels": {
    backgroundColor: "var(--chakra-colors-surface-raised)",
    color: "var(--chakra-colors-text-dim)",
    border: "none",
  },
  ".cm-panels-bottom": { borderTop: "none" },
  ".cm-vim-panel": { fontFamily: "inherit", fontSize: "12px" },
  ".cm-vim-panel, .cm-vim-panel *": { color: "var(--chakra-colors-text-dim) !important" },
  ".cm-vim-panel input": { backgroundColor: "transparent" },
  ".cm-scroller": { fontFamily: "inherit", lineHeight: "1.55", overflow: "auto" },
  ".cm-content": { padding: "12px" },
  ".cm-line": { padding: "0" },
  ".cm-placeholder": { color: "var(--chakra-colors-text-dim)" },
})

// The prompt box's look, after `theme`: the textarea's padding and line height, and a height of `--prompt-rows` lines that scrolls.
// The panel sits under the scroller, so it grows the box instead of being clipped.
const inlineTheme = EditorView.theme({
  "&.cm-editor": { height: "auto" },
  "&.cm-editor .cm-scroller": {
    lineHeight: "1.5",
    height: "calc(var(--prompt-rows) * 1.5em + 20px)",
  },
  "&.cm-editor .cm-content": { padding: "10px 46px 10px 12px" },
})

function lockExt(disabled: boolean) {
  return disabled
    ? [
        EditorState.readOnly.of(true),
        EditorView.editable.of(false),
        EditorView.contentAttributes.of({ tabindex: "-1" }),
      ]
    : []
}

/**
 * A Compartment that is reconfigured when `dep` changes. The first run is the creation, so it is skipped.
 * `before` and `after` bracket the reconfigure.
 */
function useSlot(
  view: RefObject<EditorView | null>,
  dep: unknown,
  make: () => Extension,
  around?: { before?: () => void; after?: (v: EditorView) => void },
) {
  const slot = useRef(new Compartment())
  const seen = useRef(dep)
  // `dep` is the trigger; the rest is read at run time, so only a change of `dep` reconfigures.
  const latest = useRef({ view, make, around })
  latest.current = { view, make, around }
  useEffect(() => {
    if (Object.is(seen.current, dep)) return
    seen.current = dep
    const { view, make, around } = latest.current
    const v = view.current
    if (!v) return
    around?.before?.()
    v.dispatch({ effects: slot.current.reconfigure(make()) })
    around?.after?.(v)
  }, [dep])
  return slot.current
}

/** The first and last differing positions, so an outside edit keeps the caret where it was. */
function diff(a: string, b: string) {
  let start = 0
  const max = Math.min(a.length, b.length)
  while (start < max && a[start] === b[start]) start++
  let endA = a.length
  let endB = b.length
  while (endA > start && endB > start && a[endA - 1] === b[endB - 1]) {
    endA--
    endB--
  }
  return { from: start, to: endA, insert: b.slice(start, endB) }
}

/** A CodeMirror 6 editor over the parent's text. Loaded lazily by LargeEditor. */
export default function CodeEditor({
  label,
  value,
  onChange,
  placeholder,
  selection,
  onSelectionChange,
  viMode,
  onModeChange,
  onZZ,
  onSubmit,
  onToggle,
  onMenuKey,
  caret,
  handle,
  autoFocus = true,
  bare,
  inline,
}: {
  label: string
  value: string
  onChange: (text: string) => void
  placeholder?: string
  /** The selection to open with. Read once. */
  selection: Selection
  onSelectionChange?: (selection: Selection) => void
  /** Vi keys on or off. Applies live. */
  viMode: boolean
  /** The vi mode, on every change. Not called with vi mode off. */
  onModeChange?: (mode: ViMode) => void
  /** ZZ in normal mode. */
  onZZ?: () => void
  /** Mod-Enter. The key is handled either way, so it never inserts a line or reaches a form. */
  onSubmit?: () => void
  /** Mod-g. Left unhandled when absent. */
  onToggle?: () => void
  /** Every keydown first; true means the host used it (a completion menu), so the editor skips it. */
  onMenuKey?: (e: MenuKey) => boolean
  /** Where the caret goes after the next outside edit, then cleared. */
  caret?: MutableRefObject<number | null>
  handle?: Ref<CodeEditorHandle>
  /** Focus the editor when it mounts. Default true. */
  autoFocus?: boolean
  /** Draw no border, for an editor that sits inside a bordered card. */
  bare?: boolean
  /** Sit inside a prompt box: `rows` lines tall, no border. Fixed for the editor's life; `rows` and `disabled` apply live. */
  inline?: {
    rows: number
    /** An outside edit to empty text does not reset the mode to INSERT (a large editor is editing the text). */
    holdMode?: boolean
    /** The content element's `name`, which the page reads to tell boxes apart. */
    name?: string
    /** Read-only, dimmed and skipped by Tab. Applies live. */
    disabled?: boolean
  }
}) {
  const host = useRef<HTMLDivElement>(null)
  const view = useRef<EditorView | null>(null)
  const callbacks = useRef({
    onChange,
    onSelectionChange,
    onModeChange,
    onZZ,
    onSubmit,
    onToggle,
    onMenuKey,
  })
  callbacks.current = {
    onChange,
    onSelectionChange,
    onModeChange,
    onZZ,
    onSubmit,
    onToggle,
    onMenuKey,
  }
  const viEscape = useViEscape()
  mapEscape(viEscape)
  const external = useRef(false)
  const stopVi = useRef<(() => void) | null>(null)

  // Turns the engine on: it starts in insert mode and reports every mode change.
  const startVi = (v: EditorView) => {
    mapOnce()
    const cm = getCM(v)
    if (!cm) return
    const dom = v.dom.parentElement
    const onMode = (e: { mode: string }) => {
      const mode = modeOf(e.mode)
      dom?.setAttribute(VI_MODE_ATTR, mode)
      callbacks.current.onModeChange?.(mode)
    }
    cm.on("vim-mode-change", onMode)
    dom?.setAttribute(VI_MODE_ATTR, "INSERT")
    closers.set(cm, () => callbacks.current.onZZ?.())
    // Entering insert mode with i collapses the selection; put it back.
    const { anchor, head } = v.state.selection.main
    Vim.handleKey(cm, "i", "mapping")
    v.dispatch({ selection: { anchor, head } })
    callbacks.current.onModeChange?.("INSERT")
    stopVi.current = () => {
      // A vim notice (yanked, a refused :q) holds a 5 s timer that the engine never clears on destroy.
      ;(cm.state as { currentNotificationClose?: () => void }).currentNotificationClose?.()
      cm.off("vim-mode-change", onMode)
      dom?.removeAttribute(VI_MODE_ATTR)
      closers.delete(cm)
    }
  }

  const viSlot = useSlot(view, viMode, () => (viMode ? vim() : []), {
    before: () => {
      stopVi.current?.()
      stopVi.current = null
    },
    after: (v) => viMode && startVi(v),
  })
  const disabled = !!inline?.disabled
  const lockSlot = useSlot(view, disabled, () => lockExt(disabled))

  // The latest props for the effects that must not re-run on them: the mount effect and the value sync.
  const latest = useRef({
    value,
    selection,
    caret,
    viMode,
    disabled,
    placeholder,
    label,
    autoFocus,
    inline,
    startVi,
    viSlot,
    lockSlot,
  })
  latest.current = {
    value,
    selection,
    caret,
    viMode,
    disabled,
    placeholder,
    label,
    autoFocus,
    inline,
    startVi,
    viSlot,
    lockSlot,
  }

  useImperativeHandle(
    handle,
    () => ({
      escape() {
        const v = view.current
        const cm = v && getCM(v)
        if (cm) Vim.handleKey(cm, "<Esc>", "user")
        v?.focus()
      },
      restore(sel, leaveVisual) {
        const v = view.current
        if (!v) return
        const cm = getCM(v)
        const max = v.state.doc.length
        v.focus()
        if (leaveVisual && cm) Vim.handleKey(cm, "<Esc>", "user")
        const end = Math.min(sel.end, max)
        v.dispatch({
          selection: leaveVisual
            ? { anchor: end }
            : { anchor: Math.min(sel.start, max), head: end },
        })
      },
    }),
    [],
  )

  useEffect(() => {
    const {
      value,
      selection,
      viMode,
      disabled,
      placeholder,
      label,
      autoFocus,
      inline,
      startVi,
      viSlot,
      lockSlot,
    } = latest.current
    const clamp = (n: number) => Math.min(n, value.length)
    const v = new EditorView({
      parent: host.current!,
      state: EditorState.create({
        doc: value,
        selection: { anchor: clamp(selection.start), head: clamp(selection.end) },
        extensions: [
          // Highest, or the vim keymap consumes these in normal mode.
          Prec.highest(
            keymap.of([
              {
                key: "Mod-Enter",
                // Why the flags depend on `inline`: CodeMirror applies them even when `run` returns false, so the inline box (which may leave the key to the page) sets none; a handled key is still default-prevented.
                preventDefault: !inline,
                stopPropagation: !inline,
                run: () => {
                  const submit = callbacks.current.onSubmit
                  if (!submit && inline) return false
                  submit?.()
                  return true
                },
              },
              {
                key: "Mod-g",
                preventDefault: true,
                stopPropagation: true,
                run: () => {
                  if (!callbacks.current.onToggle) return false
                  callbacks.current.onToggle()
                  return true
                },
              },
            ]),
          ),
          // Before vim, so Escape closes an open menu before it leaves insert mode. Esc in INSERT and VISUAL
          // belongs to vim alone: stop it here, judged by the mode before vim changes it, and still let vim and
          // the menu run.
          Prec.highest(
            EditorView.domEventHandlers({
              keydown: (e, v) => {
                const mode = v.dom.parentElement?.getAttribute(VI_MODE_ATTR)
                if (e.key === "Escape" && mode && mode !== "NORMAL") e.stopPropagation()
                return callbacks.current.onMenuKey?.(e) ?? false
              },
            }),
          ),
          viSlot.of(viMode ? vim() : []),
          // Draws the caret itself (vim does the same), in the text color so it shows on a dark surface.
          drawSelection(),
          history(),
          // No indentWithTab: Tab and Shift-Tab keep moving focus.
          keymap.of([...defaultKeymap, ...historyKeymap]),
          EditorView.lineWrapping,
          placeholderExt(placeholder ?? ""),
          EditorView.contentAttributes.of({
            "aria-label": label,
            "aria-multiline": "true",
            ...(inline
              ? {
                  [PROMPT_BOX_ATTR]: "",
                  // No iOS AutoFill (contact) bar over the keyboard.
                  autocomplete: "off",
                  ...(inline.name ? { name: inline.name } : {}),
                }
              : {}),
          }),
          lockSlot.of(lockExt(disabled)),
          theme,
          inline ? inlineTheme : [],
          // A click or tap forgets the stored column, so the next j starts from the clicked spot.
          EditorView.domEventHandlers({
            pointerdown: (_e, v) => {
              const vimState = getCM(v)?.state.vim
              if (vimState) vimState.lastMotion = null
            },
          }),
          EditorView.updateListener.of((u) => {
            if (u.docChanged && !external.current)
              callbacks.current.onChange(u.state.doc.toString())
            if (u.selectionSet || u.docChanged) {
              const { from, to } = u.state.selection.main
              callbacks.current.onSelectionChange?.({ start: from, end: to })
            }
          }),
        ],
      }),
    })
    view.current = v
    if (viMode) startVi(v)
    if (autoFocus) v.focus()
    // The editor is created once; later props are applied by the effects below.
    return () => {
      stopVi.current?.()
      stopVi.current = null
      v.destroy()
    }
  }, [])

  useEffect(() => {
    const v = view.current
    if (!v) return
    const current = v.state.doc.toString()
    if (current === value) return
    const { caret, inline, viMode } = latest.current
    external.current = true
    try {
      const at = caret?.current
      if (caret) caret.current = null
      v.dispatch({
        changes: diff(current, value),
        ...(at == null ? {} : { selection: { anchor: Math.min(at, value.length) } }),
      })
    } finally {
      external.current = false
    }
    // A submit empties the box from outside: back to INSERT. A user edit never gets here (the doc already matches).
    // Not during a Cmd-G round trip: the large editor may empty the text and the mode must survive it.
    if (inline && viMode && value === "" && !inline.holdMode) {
      const cm = getCM(v)
      if (cm?.state.vim && !cm.state.vim.insertMode) {
        Vim.handleKey(cm, "<Esc>", "user")
        Vim.handleKey(cm, "i", "mapping")
      }
    }
  }, [value])

  if (inline)
    return (
      <Box
        ref={host}
        data-vi-inline=""
        style={{ "--prompt-rows": inline.rows } as CSSProperties}
        overflow="hidden"
        data-focus-ring="none"
        opacity={inline.disabled ? 0.5 : undefined}
        {...fieldLook}
        bg="transparent"
      />
    )
  return (
    <Box
      ref={host}
      flex="1"
      minH="0"
      overflow="hidden"
      borderWidth={bare ? "0" : "1px"}
      borderRadius="10px"
      {...fieldLook}
      {...(bare && { bg: "transparent" })}
    />
  )
}
