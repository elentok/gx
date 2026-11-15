import { Box, chakra, Text, Textarea } from "@chakra-ui/react"
import {
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type KeyboardEvent,
  type ReactNode,
} from "react"
import { useCompletion, type CompletionConfig, type MenuKey } from "../../commands/useCompletion"
import type CodeEditor from "./CodeEditor"
import type { CodeEditorHandle, ViMode } from "./CodeEditor"
import { useBigPrompt } from "./bigPromptStore"
import { LargeEditor, preloadCodeEditor, type EditorMenu, type Selection } from "./LargeEditor"
import { useCoarsePointer } from "../../useWide"
import { useViMode } from "../../vimode"
import { fieldLook, PROMPT_BOX_ATTR } from "./look"
import { isMac } from "../../platform"
import { ModeText } from "./ModeText"

/** Runs `submit` on Cmd/Ctrl+Enter; plain Enter stays a newline. */
export function submitOnCmdEnter(e: KeyboardEvent<HTMLTextAreaElement>, submit: () => void) {
  if (e.key !== "Enter" || !(e.metaKey || e.ctrlKey)) return
  e.preventDefault()
  submit()
}

interface FieldProps {
  value: string
  onChange: (text: string) => void
  rows?: number
  /** The field's accessible name. */
  label: string
  /** The textarea's name: stable and not card-like or contact-like, so the browser offers no AutoFill. */
  name: string
  /** The large editor's title. Its accessible name is "<title> text". */
  title: string
  placeholder?: string
  disabled?: boolean
  autoFocus?: boolean
  /** Cmd/Ctrl+Enter in the field. */
  onSubmit?: () => void
  size?: "sm" | "md"
  radius?: string
  /** Overrides the box's background (default: the raised surface). */
  bg?: string
  /** Opt in to the command completion menu in the inline textarea. */
  completion?: CompletionConfig
  /** A status line inside the box, under the text. `null` keeps its room with no text; leave it out for no line. */
  note?: { text: string; red?: boolean } | null
  /** A line inside the box, under the text and the note. */
  footer?: ReactNode
}

/** The one prompt box: a textarea with an expand button that opens a large editor on the same text. The parent owns the text; there is no copy and no save step. */
export function PromptBox(props: FieldProps) {
  const { completion, ...rest } = props
  return completion ? <Completing {...rest} config={completion} /> : <Field {...rest} />
}

interface Menu extends EditorMenu {
  onKeyDown: (e: MenuKey) => boolean
}

function Completing({
  config,
  ...rest
}: Omit<FieldProps, "completion"> & { config: CompletionConfig }) {
  const [cursor, setCursor] = useState(0)
  // The large editor's vi mode; null while it is closed or vi is off. The menu stays shut outside INSERT.
  const [editorMode, setEditorMode] = useState<ViMode | null>(null)
  const pending = useRef<number | null>(null)
  const off = config.off || (editorMode !== null && editorMode !== "INSERT")
  const c = useCompletion({ ...config, off }, rest.value, cursor, rest.disabled, (text, at) => {
    pending.current = at
    rest.onChange(text)
    setCursor(at)
  })
  return (
    <Field
      {...rest}
      menu={{
        node: c.menu,
        onKeyDown: c.onKeyDown,
        onCursor: setCursor,
        onMode: setEditorMode,
        pending,
      }}
    />
  )
}

function Field({
  value,
  onChange,
  rows,
  label,
  name,
  title,
  placeholder,
  disabled,
  autoFocus,
  onSubmit,
  size,
  radius = "12px",
  bg,
  menu,
  note,
  footer,
}: Omit<FieldProps, "completion"> & { menu?: Menu }) {
  const big = useBigPrompt()
  const effectiveRows = big ? (rows ?? 2) * 2 : (rows ?? 2)
  const area = useRef<HTMLTextAreaElement>(null)
  const [selection, setSelection] = useState<Selection | null>(null)

  // With vi on and a fine pointer the box is a vi editor. Until its chunk has loaded the textarea stays.
  const wantEditor = useViMode() && !useCoarsePointer()
  const [Editor, setEditor] = useState<typeof CodeEditor | null>(null)
  useEffect(() => {
    if (!wantEditor) return
    let live = true
    Promise.resolve(preloadCodeEditor()).then((m) => live && m && setEditor(() => m.default))
    return () => {
      live = false
    }
  }, [wantEditor])
  const showEditor = wantEditor && Editor !== null
  // Kept across blur and focus. The editor resets it itself when the parent empties the text.
  const [mode, setMode] = useState<ViMode>("INSERT")
  const onMode = menu?.onMode
  useEffect(() => {
    // While the large editor is open it reports its own mode.
    if (!showEditor || selection) return
    onMode?.(mode)
    return () => onMode?.(null)
  }, [onMode, showEditor, selection, mode])
  // The swap carries the selection and focus. They are read during the render that swaps, while the old element is still in the DOM.
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
  if (shown.current !== showEditor) {
    shown.current = showEditor
    carry.current = read()
    editorSel.current = carry.current.sel
  }
  useLayoutEffect(() => {
    const c = carry.current
    if (showEditor || !c || !area.current) return
    carry.current = null
    area.current.setSelectionRange(c.sel.start, c.sel.end)
    if (c.focus) area.current.focus()
  }, [showEditor])
  useLayoutEffect(() => {
    // While the editor is open it places the caret itself.
    // The inline editor takes `pending` itself, as its `caret`.
    if (selection || showEditor) return
    const at = menu?.pending.current
    if (at == null) return
    menu!.pending.current = null
    area.current?.setSelectionRange(at, at)
  }, [value, menu, selection, showEditor])
  const open = () => setSelection(read().sel)
  const editorHandle = useRef<CodeEditorHandle>(null)
  // Focus and selection are set synchronously, inside the user's gesture and before the layer unmounts, so a phone keeps its keyboard.
  const close = (sel: Selection, refocus: boolean) => {
    if (refocus) {
      area.current?.focus()
      area.current?.setSelectionRange(sel.start, sel.end)
      // The inline editor keeps its own mode; VISUAL comes back as NORMAL with a caret.
      if (editorHandle.current) {
        const collapse = mode === "VISUAL"
        editorSel.current = collapse ? { start: sel.end, end: sel.end } : sel
        editorHandle.current.restore(sel, collapse)
      }
    }
    setSelection(null)
  }
  const onKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (
      !disabled &&
      e.key.toLowerCase() === "g" &&
      !e.altKey &&
      !e.shiftKey &&
      (isMac() ? e.metaKey && !e.ctrlKey : e.ctrlKey && !e.metaKey)
    ) {
      e.preventDefault()
      open()
      return
    }
    if (menu?.onKeyDown(e)) return
    if (onSubmit) submitOnCmdEnter(e, onSubmit)
  }
  const raised = bg ?? "surface.raised"
  // The box draws the border, background and focus ring; the textarea inside has none, and the note and footer sit under it in the flow.
  // Both outlines sit inside the box, so nothing spills into (or is clipped by) the sheet or footer around it.
  const ringShadow = "inset 0 0 0 2px {colors.tone.running.fg}"
  return (
    <Box
      position="relative"
      display="flex"
      flexDirection="column"
      bg={raised}
      borderRadius={radius}
      boxShadow="inset 0 0 0 1px {colors.hairline}"
      css={{
        "&:has(> textarea:focus-visible), &:has(.cm-editor.cm-focused)": {
          boxShadow: `${ringShadow} !important`,
        },
      }}
    >
      {showEditor && Editor ? (
        <Box ref={wrap}>
          <Editor
            inline={{ rows: effectiveRows, name, disabled }}
            onSubmit={onSubmit}
            label={label}
            value={value}
            onChange={onChange}
            placeholder={placeholder}
            selection={carry.current?.sel ?? editorSel.current}
            onSelectionChange={(s) => {
              // The large editor owns the selection and the menu cursor while it is open.
              if (selection) return
              editorSel.current = s
              menu?.onCursor(s.end)
            }}
            onMenuKey={menu?.onKeyDown}
            caret={selection ? undefined : menu?.pending}
            onModeChange={setMode}
            onToggle={disabled ? undefined : open}
            handle={editorHandle}
            holdMode={!!selection}
            autoFocus={carry.current?.focus ?? false}
            viMode
          />
        </Box>
      ) : (
        <Textarea
          ref={area}
          {...fieldLook}
          data-focus-ring="none"
          {...{ [PROMPT_BOX_ATTR]: "" }}
          size={size}
          bg="transparent"
          borderWidth="0"
          borderRadius="0"
          autoFocus={autoFocus}
          rows={big ? (rows ?? 2) * 2 : rows}
          lineHeight="1.5"
          display="block"
          pt="2.5"
          pb="2.5"
          ps="3"
          pe="46px"
          resize="none"
          aria-label={label}
          name={name}
          autoComplete="off"
          placeholder={placeholder}
          disabled={disabled}
          value={value}
          onChange={(e) => {
            menu?.onCursor(e.target.selectionStart)
            onChange(e.target.value)
          }}
          onSelect={menu ? (e) => menu.onCursor(e.currentTarget.selectionStart) : undefined}
          onKeyDown={onKeyDown}
        />
      )}
      {(note !== undefined || footer || wantEditor) && (
        <Box px="3" pt="1" pb="1.5">
          {note !== undefined && (
            <Text
              fontSize="xs"
              lineHeight="1.5"
              pointerEvents="none"
              truncate
              pe="34px"
              color={note?.red ? "tone.deadlocked.fg" : "text.dim"}
            >
              {note?.text}
            </Text>
          )}
          {wantEditor ? (
            <Box display="flex" alignItems="center" gap="2" minW="0">
              <Box opacity={disabled ? 0.5 : undefined} data-disabled={disabled ? "" : undefined}>
                <ModeText mode={mode} />
              </Box>
              {footer && (
                <>
                  <Box aria-hidden color="text.dim" flex="none">
                    ·
                  </Box>
                  <Box flex="1" minW="0">
                    {footer}
                  </Box>
                </>
              )}
            </Box>
          ) : (
            footer
          )}
        </Box>
      )}
      {!selection && menu?.node}
      <chakra.button
        type="button"
        position="absolute"
        top="1.5"
        right="1.5"
        w="36px"
        h="36px"
        borderRadius="8px"
        bg={raised}
        color="text.muted"
        display="flex"
        alignItems="center"
        justifyContent="center"
        cursor="pointer"
        _disabled={{ opacity: 0.5, cursor: "not-allowed" }}
        disabled={disabled}
        aria-label="Open the note in a large editor"
        title="Open the note in a large editor"
        onClick={open}
      >
        <svg
          width="16"
          height="16"
          viewBox="0 0 16 16"
          fill="none"
          stroke="currentColor"
          strokeWidth="1.6"
          strokeLinecap="round"
          strokeLinejoin="round"
          aria-hidden="true"
        >
          <path d="M9.5 2.5h4v4M13.5 2.5L9 7M6.5 13.5h-4v-4M2.5 13.5L7 9" />
        </svg>
      </chakra.button>
      {selection && (
        <LargeEditor
          title={title}
          value={value}
          onChange={onChange}
          placeholder={placeholder}
          selection={selection}
          onClose={close}
          onSubmit={onSubmit}
          menu={menu}
        />
      )}
    </Box>
  )
}
