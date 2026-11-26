import { setViMode } from "../../vimode"
import { EditorView } from "@codemirror/view"
import { getCM, Vim } from "@replit/codemirror-vim"
import { act, cleanup, fireEvent, screen, waitFor } from "@testing-library/react"
import { useState, type ReactNode } from "react"
import { afterEach, expect, test, vi } from "vitest"
import { ConnectionContext, type Connection, type ConnectionState } from "../../connection"
import { CoarsePointerContext, WideContext } from "../../useWide"
import { renderThemed } from "../render"
import { setBigPrompt } from "./bigPromptStore"
import { loadCodeEditor } from "./LargeEditor"
import { PromptBox } from "./PromptBox"
import { findLargeEditor, largeEditorText, typeInLargeEditor, viewOf } from "./testing"

// Wraps the real loader so a test can count calls.
vi.mock("./LargeEditor", async (importOriginal) => {
  const original = await importOriginal<typeof import("./LargeEditor")>()
  return { ...original, loadCodeEditor: vi.fn(original.loadCodeEditor) }
})

afterEach(() => {
  cleanup()
  vi.mocked(loadCodeEditor).mockClear()
  setBigPrompt(false)
  localStorage.clear()
})

const TITLE = "Note on Q2 - Pick"
const expand = () => screen.getByLabelText("Open the note in a large editor")
const field = () => screen.getByLabelText("Note") as HTMLTextAreaElement

function Harness({
  onChange = () => {},
  disabled,
}: {
  onChange?: (t: string) => void
  disabled?: boolean
}) {
  const [text, setText] = useState("")
  return (
    <PromptBox
      label="Note"
      name="test-note"
      title={TITLE}
      rows={3}
      placeholder="Type"
      disabled={disabled}
      value={text}
      onChange={(t) => {
        setText(t)
        onChange(t)
      }}
    />
  )
}

test("the textarea turns AutoFill off and has a name", () => {
  renderThemed(<Harness />)
  const area = field()
  expect(area.getAttribute("autocomplete")).toBe("off")
  expect(area.getAttribute("name")).toBe("test-note")
  for (const attr of ["autocorrect", "autocapitalize", "spellcheck", "inputmode"]) {
    expect(area.hasAttribute(attr)).toBe(false)
  }
})

test("the textarea is named by a linked label and keeps the prompt box marks", () => {
  renderThemed(<Harness />)
  const area = field()
  expect(area.hasAttribute("aria-label")).toBe(false)
  expect(area.labels?.[0].textContent).toBe("Note")
  expect(area.labels?.[0].hasAttribute("aria-hidden")).toBe(false)
  expect(area.hasAttribute("data-prompt-box")).toBe(true)
  expect(area.getAttribute("name")).toBe("test-note")
})

test("a label with the word name is refused", () => {
  vi.spyOn(console, "error").mockImplementation(() => {})
  expect(() =>
    renderThemed(<PromptBox label="Name" name="n" title={TITLE} value="" onChange={() => {}} />),
  ).toThrow("text-box-name")
})

test.each(["sm", "md"] as const)("a %s box keeps its spacing", (size) => {
  renderThemed(
    <PromptBox label="Note" name="n" title={TITLE} size={size} value="" onChange={() => {}} />,
  )
  // jsdom cannot compute the theme's spacing tokens, so read the rules written for the textarea.
  const rules = [...document.querySelectorAll("style")]
    .flatMap((s) => (s.textContent ?? "").split("\n"))
    .filter((line) => [...field().classList].some((c) => line.includes(`.${c}{`)))
    .join("\n")
  for (const rule of [
    /padding-inline:var\(--chakra-spacing-3\)/,
    /padding-block:var\(--chakra-spacing-2\\?\.5\)/,
    /padding-inline-end:46px/,
    /line-height:1\.5/,
    /resize:none/,
  ])
    expect(rules).toMatch(rule)
})

test("the expand button also opens the large editor from the inline vi editor", async () => {
  setViMode(true)
  renderThemed(<Harness />)
  await waitFor(() => expect(document.querySelector("[data-vi-inline] .cm-editor")).toBeTruthy())
  expect(document.querySelector("textarea")).toBeNull()
  expect(screen.getAllByLabelText("Open the note in a large editor")).toHaveLength(1)
  fireEvent.click(expand())
  await findLargeEditor(TITLE)
})

test("the textarea gets value, rows, label, placeholder and disabled", () => {
  renderThemed(
    <PromptBox
      label="Note"
      name="test-note"
      title={TITLE}
      rows={3}
      placeholder="Type"
      disabled
      value="hi"
      onChange={() => {}}
    />,
  )
  const area = field()
  expect(area.value).toBe("hi")
  expect(area.rows).toBe(3)
  expect(area.placeholder).toBe("Type")
  expect(area.disabled).toBe(true)
})

test.each([
  [2, 4],
  [3, 6],
  [5, 10],
  [8, 16],
  [undefined, 4],
])("big mode doubles rows %s to %s", (rows, expected) => {
  setBigPrompt(true)
  renderThemed(
    <PromptBox label="Note" name="n" title={TITLE} rows={rows} value="" onChange={() => {}} />,
  )
  expect(field().rows).toBe(expected)
})

test("a box with no rows keeps the browser default with big mode off", () => {
  renderThemed(<PromptBox label="Note" name="n" title={TITLE} value="" onChange={() => {}} />)
  expect(field().rows).toBe(2)
})

test("flipping big mode changes rows live and keeps text and focus", () => {
  renderThemed(<Harness />)
  const area = field()
  fireEvent.change(area, { target: { value: "hello" } })
  area.focus()
  act(() => setBigPrompt(true))
  expect(field()).toBe(area)
  expect(area.rows).toBe(6)
  expect(area.value).toBe("hello")
  expect(document.activeElement).toBe(area)
})

test("typing calls onChange", () => {
  const onChange = vi.fn()
  renderThemed(<Harness onChange={onChange} />)
  fireEvent.change(field(), { target: { value: "abc" } })
  expect(onChange).toHaveBeenCalledWith("abc")
})

test("the expand button opens a dialog named by the title; the editor is a multiline textbox named '<title> text'", async () => {
  renderThemed(<Harness />)
  fireEvent.click(expand())
  expect(screen.getByRole("dialog").getAttribute("aria-label")).toBe(TITLE)
  const editor = await findLargeEditor(TITLE)
  expect(editor.getAttribute("aria-label")).toBe(`${TITLE} text`)
  expect(editor.getAttribute("aria-multiline")).toBe("true")
})

test("the editor edits the same text and Done closes it", async () => {
  const onChange = vi.fn()
  renderThemed(<Harness onChange={onChange} />)
  fireEvent.click(expand())
  await typeInLargeEditor(TITLE, "typed")
  expect(onChange).toHaveBeenCalledWith("typed")
  fireEvent.click(screen.getByText("Done"))
  expect(screen.queryByRole("dialog")).toBeNull()
  expect(field().value).toBe("typed")
})

function SetNewParent({ onChange }: { onChange: () => void }) {
  const [text, setText] = useState("hello world")
  return (
    <>
      <button type="button" onClick={() => setText("hello world!")}>
        Set new
      </button>
      <PromptBox label="Note" name="test-note" title={TITLE} value={text} onChange={onChange} />
    </>
  )
}

test("the editor shows a new value prop and keeps the caret: no copy", async () => {
  const onChange = vi.fn()
  renderThemed(<SetNewParent onChange={onChange} />)
  field().setSelectionRange(2, 2)
  fireEvent.click(expand())
  await findLargeEditor(TITLE)
  expect(largeEditorText(TITLE)).toBe("hello world")
  fireEvent.click(screen.getByText("Set new"))
  expect(largeEditorText(TITLE)).toBe("hello world!")
  expect(viewOf(TITLE).state.selection.main.head).toBe(2)
  expect(onChange).not.toHaveBeenCalled()
})

test("Tab does not insert a tab character", async () => {
  renderThemed(<Harness />)
  fireEvent.click(expand())
  const editor = await findLargeEditor(TITLE)
  await typeInLargeEditor(TITLE, "abc")
  fireEvent.keyDown(editor, { key: "Tab" })
  fireEvent.keyDown(editor, { key: "Tab", shiftKey: true })
  expect(largeEditorText(TITLE)).toBe("abc")
})

test("the placeholder shows while the editor is empty", async () => {
  renderThemed(<Harness />)
  fireEvent.click(expand())
  await findLargeEditor(TITLE)
  expect(screen.getByRole("dialog").querySelector(".cm-placeholder")?.textContent).toBe("Type")
})

test("a disabled field has a disabled expand button", () => {
  renderThemed(<Harness disabled />)
  expect((expand() as HTMLButtonElement).disabled).toBe(true)
})

test("the selection goes to the editor on open and back to the field on close", async () => {
  renderThemed(
    <PromptBox
      label="Note"
      name="test-note"
      title={TITLE}
      value="hello world"
      onChange={() => {}}
    />,
  )
  field().setSelectionRange(2, 7)
  fireEvent.click(expand())
  await findLargeEditor(TITLE)
  const sel = () => {
    const { from, to } = viewOf(TITLE).state.selection.main
    return [from, to]
  }
  expect(sel()).toEqual([2, 7])
  viewOf(TITLE).dispatch({ selection: { anchor: 1, head: 4 } })
  fireEvent.click(screen.getByText("Done"))
  expect([field().selectionStart, field().selectionEnd]).toEqual([1, 4])
})

test("Escape does not close the editor", () => {
  renderThemed(<Harness />)
  fireEvent.click(expand())
  fireEvent.keyDown(window, { key: "Escape" })
  expect(screen.getByRole("dialog")).toBeTruthy()
})

// The existing tests mean the textarea, so the pointer is coarse unless a test says otherwise.
function withVi(on: boolean, ui: ReactNode, coarse = true, viEscape = "") {
  setViMode(on)
  const state = {
    snapshot: { sessions: [], vi_escape: viEscape },
    connected: true,
    loadedOnce: true,
    tick: 0,
  } as unknown as ConnectionState
  const conn = {
    getState: () => state,
    subscribe: () => () => {},
    onEvent: () => () => {},
  } as unknown as Connection
  return renderThemed(
    <WideContext.Provider value>
      <CoarsePointerContext.Provider value={coarse}>
        <ConnectionContext.Provider value={conn}>{ui}</ConnectionContext.Provider>
      </CoarsePointerContext.Provider>
    </WideContext.Provider>,
  )
}

function Keys({
  disabled,
  onSubmit,
  onKeyDown,
}: {
  disabled?: boolean
  onSubmit?: () => void
  onKeyDown?: () => void
}) {
  const [text, setText] = useState("hello world")
  return (
    <form onKeyDown={onKeyDown}>
      <PromptBox
        label="Note"
        name="test-note"
        title={TITLE}
        value={text}
        onChange={setText}
        disabled={disabled}
        onSubmit={onSubmit}
      />
    </form>
  )
}

function stubPlatform(platform: string) {
  vi.spyOn(navigator, "platform", "get").mockReturnValue(platform)
}

afterEach(() => vi.restoreAllMocks())

test("Cmd-G on a Mac and Ctrl-G elsewhere open the editor and cancel the browser default", async () => {
  for (const [platform, mod, other] of [
    ["MacIntel", "metaKey", "ctrlKey"],
    ["Win32", "ctrlKey", "metaKey"],
  ] as const) {
    stubPlatform(platform)
    withVi(true, <Keys />)
    field().focus()
    expect(fireEvent.keyDown(field(), { key: "g", [other]: true })).toBe(true)
    expect(screen.queryByRole("dialog")).toBeNull()
    expect(fireEvent.keyDown(field(), { key: "g", [mod]: true })).toBe(false)
    await findLargeEditor(TITLE)
    cleanup()
  }
})

test("with vi mode off the key still opens the editor", async () => {
  stubPlatform("MacIntel")
  withVi(false, <Keys />)
  expect(fireEvent.keyDown(field(), { key: "g", metaKey: true })).toBe(false)
  await findLargeEditor(TITLE)
})

test("with vi mode off the open key inside the editor closes it", async () => {
  withVi(false, <Keys />)
  fireEvent.click(expand())
  const editor = await findLargeEditor(TITLE)
  expect(fireEvent.keyDown(editor, { key: "g", ctrlKey: true })).toBe(false)
  expect(screen.queryByRole("dialog")).toBeNull()
})

test("with vi mode on, the key with focus on the page body opens nothing", () => {
  stubPlatform("MacIntel")
  withVi(true, <Keys />)
  document.body.focus()
  fireEvent.keyDown(document.body, { key: "g", metaKey: true })
  expect(screen.queryByRole("dialog")).toBeNull()
})

test("a disabled field opens nothing", () => {
  stubPlatform("MacIntel")
  withVi(true, <Keys disabled />)
  fireEvent.keyDown(field(), { key: "g", metaKey: true })
  expect(screen.queryByRole("dialog")).toBeNull()
  expect((expand() as HTMLButtonElement).disabled).toBe(true)
})

test("the open key inside the editor closes it and cancels the default", async () => {
  withVi(true, <Keys />)
  fireEvent.click(expand())
  const editor = await findLargeEditor(TITLE)
  // CodeMirror reads Mod as Ctrl in jsdom.
  expect(fireEvent.keyDown(editor, { key: "g", ctrlKey: true })).toBe(false)
  expect(screen.queryByRole("dialog")).toBeNull()
  expect(document.activeElement).toBe(field())
})

test("Cmd-Enter in the editor closes first, then submits once, without refocus or the field's handler", async () => {
  const onKeyDown = vi.fn()
  const onSubmit = vi.fn()
  withVi(true, <Keys onSubmit={onSubmit} onKeyDown={onKeyDown} />)
  const focus = vi.spyOn(field(), "focus")
  fireEvent.click(expand())
  const editor = await findLargeEditor(TITLE)
  fireEvent.keyDown(editor, { key: "Enter", ctrlKey: true })
  expect(onSubmit).toHaveBeenCalledTimes(1)
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull())
  expect(focus).not.toHaveBeenCalled()
  expect(onKeyDown).not.toHaveBeenCalled()
})

test("Cmd-Enter without a submit action leaves the editor open", async () => {
  const onKeyDown = vi.fn()
  withVi(true, <Keys onKeyDown={onKeyDown} />)
  fireEvent.click(expand())
  const editor = await findLargeEditor(TITLE)
  fireEvent.keyDown(editor, { key: "Enter", ctrlKey: true })
  expect(screen.getByRole("dialog")).toBeTruthy()
  expect(onKeyDown).not.toHaveBeenCalled()
})

test("Done, the open key, a backdrop press and ZZ each refocus the field with the editor's selection, before the dialog goes", async () => {
  const closers: [string, () => void][] = [
    ["Done", () => fireEvent.click(screen.getByText("Done"))],
    // CodeMirror reads Mod as Ctrl in jsdom.
    ["Cmd-G", () => void fireEvent.keyDown(viewOf(TITLE).contentDOM, { key: "g", ctrlKey: true })],
    [
      "backdrop",
      () => {
        const backdrop = screen.getByRole("dialog").parentElement!
        fireEvent.mouseDown(backdrop)
        fireEvent.click(backdrop)
      },
    ],
    [
      "ZZ",
      () => {
        const cm = getCM(viewOf(TITLE))!
        act(() => void Vim.handleKey(cm, "ZZ", "user"))
      },
    ],
  ]
  for (const [name, close] of closers) {
    withVi(true, <Keys />)
    field().setSelectionRange(1, 3)
    fireEvent.click(expand())
    await findLargeEditor(TITLE)
    // Vim's Escape moves the caret, so the expectation is read after it.
    viewOf(TITLE).dispatch({ selection: { anchor: 2, head: 5 } })
    act(() => void Vim.handleKey(getCM(viewOf(TITLE))!, "<Esc>", "user"))
    const { from, to } = viewOf(TITLE).state.selection.main
    let dialogAtFocus: boolean | undefined
    vi.spyOn(field(), "focus").mockImplementation(() => {
      dialogAtFocus = !!screen.queryByRole("dialog")
    })
    close()
    await waitFor(() => expect(screen.queryByRole("dialog"), name).toBeNull())
    expect(dialogAtFocus, name).toBe(true)
    expect([field().selectionStart, field().selectionEnd], name).toEqual([from, to])
    cleanup()
    vi.restoreAllMocks()
  }
})

const entries = ["compact", "context", "help"].map((name) => ({
  name,
  aliases: [],
  arg_hint: "",
  description: name,
  source: "built-in",
}))
const fetcher = async () => ({ degraded: false, entries }) as never

function Completing({ harness }: { harness: string }) {
  const [text, setText] = useState("")
  return (
    <PromptBox
      label="Note"
      name="test-note"
      title={TITLE}
      value={text}
      onChange={setText}
      completion={{ target: { sessionId: `s-${harness}` }, harness, fetcher }}
    />
  )
}

const options = () => screen.queryAllByRole("option").map((o) => o.textContent)

async function openEditor(harness = "claude") {
  withVi(true, <Completing harness={harness} />)
  fireEvent.click(expand())
  return findLargeEditor(TITLE)
}

test("the menu opens in the large editor on /, Down then Enter fills it", async () => {
  const editor = await openEditor()
  await typeInLargeEditor(TITLE, "/co")
  await waitFor(() => expect(options()).toHaveLength(2))
  fireEvent.keyDown(editor, { key: "ArrowDown" })
  fireEvent.keyDown(editor, { key: "Enter" })
  expect(largeEditorText(TITLE)).toBe("/context ")
  expect(viewOf(TITLE).state.selection.main.head).toBe("/context ".length)
  expect(screen.queryByRole("listbox")).toBeNull()
})

test("Escape closes the menu first and a second Escape leaves INSERT", async () => {
  const editor = await openEditor()
  await typeInLargeEditor(TITLE, "/co")
  await waitFor(() => expect(options()).toHaveLength(2))
  fireEvent.keyDown(editor, { key: "Escape" })
  expect(screen.queryByRole("listbox")).toBeNull()
  expect(screen.getByText("INSERT")).toBeTruthy()
  act(() => void Vim.handleKey(getCM(viewOf(TITLE))!, "<Esc>", "user"))
  expect(screen.getByText("NORMAL")).toBeTruthy()
})

test("the menu stays shut in NORMAL and returns in INSERT", async () => {
  await openEditor()
  await typeInLargeEditor(TITLE, "/co")
  await waitFor(() => expect(options()).toHaveLength(2))
  const cm = getCM(viewOf(TITLE))!
  act(() => void Vim.handleKey(cm, "<Esc>", "user"))
  await waitFor(() => expect(screen.queryByRole("listbox")).toBeNull())
  act(() => void Vim.handleKey(cm, "j", "user"))
  expect(screen.queryByRole("listbox")).toBeNull()
  act(() => void Vim.handleKey(cm, "i", "user"))
  await waitFor(() => expect(options()).toHaveLength(2))
})

test("$ opens the menu in the editor for Codex only", async () => {
  await openEditor("codex")
  await typeInLargeEditor(TITLE, "$co")
  await waitFor(() => expect(options()).toHaveLength(2))
  cleanup()
  await openEditor("claude")
  await typeInLargeEditor(TITLE, "$co")
  expect(screen.queryByRole("listbox")).toBeNull()
})

// The inline vi editor.
const isEditor = () => screen.getByRole("textbox", { name: "Note" }).tagName === "DIV"
const inlineView = async () => {
  await waitFor(() => expect(isEditor()).toBe(true))
  return EditorView.findFromDOM(screen.getByRole("textbox", { name: "Note" }))!
}

function Live({
  rows,
  fine,
  onSubmit,
  disabled,
}: {
  rows?: number
  fine?: boolean
  onSubmit?: () => void
  disabled?: boolean
}) {
  const [coarse, setCoarse] = useState(!fine)
  const [text, setText] = useState("hello world")
  return (
    <CoarsePointerContext.Provider value={coarse}>
      <button type="button" onClick={() => setCoarse((c) => !c)}>
        flip
      </button>
      <PromptBox
        label="Note"
        name="test-note"
        title={TITLE}
        rows={rows}
        value={text}
        onChange={setText}
        onSubmit={onSubmit}
        disabled={disabled}
      />
    </CoarsePointerContext.Provider>
  )
}

test("vi off or a coarse pointer keeps the textarea; vi on with a fine pointer is an editor", async () => {
  withVi(false, <Live />, false)
  expect(field().tagName).toBe("TEXTAREA")
  cleanup()
  withVi(true, <Live />, true)
  expect(field().tagName).toBe("TEXTAREA")
  cleanup()
  withVi(true, <Harness />, false)
  await inlineView()
  expect(document.querySelector("textarea")).toBeNull()
})

test("the chunk is preloaded only with vi on and a fine pointer", async () => {
  withVi(false, <Harness />, false)
  cleanup()
  withVi(true, <Harness />, true)
  expect(loadCodeEditor).not.toHaveBeenCalled()
  cleanup()
  withVi(true, <Harness />, false)
  expect(loadCodeEditor).toHaveBeenCalled()
  await inlineView()
})

test("the inline vi host carries data-vi-inline", async () => {
  withVi(true, <Harness />, false)
  const view = await inlineView()
  expect(view.dom.parentElement?.hasAttribute("data-vi-inline")).toBe(true)
})

test("a live swap keeps text, caret, selection and focus in both directions", async () => {
  withVi(true, <Live />)
  field().focus()
  field().setSelectionRange(2, 7)
  fireEvent.click(screen.getByText("flip"))
  const view = await inlineView()
  expect(view.state.doc.toString()).toBe("hello world")
  const { from, to } = view.state.selection.main
  expect([from, to]).toEqual([2, 7])
  expect(view.hasFocus).toBe(true)
  view.dispatch({ selection: { anchor: 1, head: 4 } })
  fireEvent.click(screen.getByText("flip"))
  expect(field().value).toBe("hello world")
  expect([field().selectionStart, field().selectionEnd]).toEqual([1, 4])
  expect(document.activeElement).toBe(field())
})

test("an unfocused box is still unfocused after the swap", async () => {
  withVi(true, <Live />)
  field().setSelectionRange(3, 3)
  fireEvent.click(screen.getByText("flip"))
  const view = await inlineView()
  expect(view.hasFocus).toBe(false)
  expect(view.state.selection.main.head).toBe(3)
  fireEvent.click(screen.getByText("flip"))
  expect(document.activeElement).not.toBe(field())
})

test("in vi mode the expand button comes after the editor in DOM order", async () => {
  withVi(true, <Live fine />)
  const view = await inlineView()
  expect(view.dom.compareDocumentPosition(expand()) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
})

test("focus on the expand button is not carried into the textarea on swap", async () => {
  withVi(true, <Live fine />)
  await inlineView()
  expand().focus()
  fireEvent.click(screen.getByText("flip"))
  expect(document.activeElement).not.toBe(field())
})

test.each([
  [3, false, "3"],
  [undefined, false, "2"],
  [3, true, "6"],
  [undefined, true, "4"],
])("the inline editor is rows %s (big %s) lines tall: %s", async (rows, big, expected) => {
  setBigPrompt(big)
  withVi(true, <Live rows={rows} fine />)
  const view = await inlineView()
  expect(view.dom.parentElement!.style.getPropertyValue("--prompt-rows")).toBe(expected)
})

test("the escape sequence takes the inline box from INSERT to NORMAL", async () => {
  withVi(true, <Harness />, false, "jk")
  const view = await inlineView()
  const host = view.dom.parentElement!
  const cm = getCM(view)!
  expect(host.getAttribute("data-vi-mode")).toBe("INSERT")
  act(() => void Vim.handleKey(cm, "j", "user"))
  act(() => void Vim.handleKey(cm, "k", "user"))
  expect(host.getAttribute("data-vi-mode")).toBe("NORMAL")
  expect(view.state.doc.toString()).toBe("")
})

test("flipping big mode resizes the inline editor without remounting it", async () => {
  withVi(true, <Live rows={3} fine />)
  const view = await inlineView()
  act(() => setBigPrompt(true))
  expect(EditorView.findFromDOM(screen.getByRole("textbox", { name: "Note" }))).toBe(view)
  expect(view.dom.parentElement!.style.getPropertyValue("--prompt-rows")).toBe("6")
  expect(view.state.doc.toString()).toBe("hello world")
})

test("ZZ does nothing; :q and :wq say Not an editor command and keep the text", async () => {
  withVi(true, <Live fine />)
  const view = await inlineView()
  const cm = getCM(view)!
  act(() => void Vim.handleKey(cm, "<Esc>", "user"))
  act(() => void Vim.handleKey(cm, "ZZ", "user"))
  expect(view.state.doc.toString()).toBe("hello world")
  for (const cmd of ["q", "wq"]) {
    act(() => void Vim.handleEx(cm as Parameters<typeof Vim.handleEx>[0], cmd))
    expect(view.dom.textContent).toContain("Not an editor command")
    expect(view.state.doc.toString()).toBe("hello world")
  }
  expect(screen.queryByRole("dialog")).toBeNull()
})

test("/, ? and : open the panel inside the box", async () => {
  withVi(true, <Live fine />)
  const view = await inlineView()
  const cm = getCM(view)!
  for (const key of ["/", "?", ":"]) {
    act(() => void Vim.handleKey(cm, "<Esc>", "user"))
    act(() => void Vim.handleKey(cm, key, "user"))
    expect(view.dom.querySelector(".cm-vim-panel input")).not.toBeNull()
    act(() => void Vim.handleKey(cm, "<Esc>", "user"))
  }
})

const modEnter = (el: Element) =>
  fireEvent.keyDown(el, { key: "Enter", code: "Enter", keyCode: 13, ctrlKey: true })

test("Mod-Enter submits in INSERT, NORMAL and VISUAL", async () => {
  const onSubmit = vi.fn()
  withVi(true, <Live fine onSubmit={onSubmit} />)
  const view = await inlineView()
  const cm = getCM(view)!
  modEnter(view.contentDOM)
  act(() => void Vim.handleKey(cm, "<Esc>", "user"))
  modEnter(view.contentDOM)
  act(() => void Vim.handleKey(cm, "v", "user"))
  modEnter(view.contentDOM)
  expect(onSubmit).toHaveBeenCalledTimes(3)
  expect(view.state.doc.toString()).toBe("hello world")
})

test("Mod-Enter without onSubmit is left to the page", async () => {
  const seen = vi.fn()
  withVi(true, <Live fine />)
  const view = await inlineView()
  document.addEventListener("keydown", seen)
  try {
    modEnter(view.contentDOM)
  } finally {
    document.removeEventListener("keydown", seen)
  }
  expect(seen).toHaveBeenCalledTimes(1)
})

test("Tab is not handled by the inline editor", async () => {
  withVi(true, <Live fine />)
  const view = await inlineView()
  const tab = new KeyboardEvent("keydown", { key: "Tab", bubbles: true, cancelable: true })
  view.contentDOM.dispatchEvent(tab)
  expect(tab.defaultPrevented).toBe(false)
  expect(view.state.doc.toString()).toBe("hello world")
})

test("a disabled inline editor is read-only, dimmed and skipped by Tab, and keeps its text", async () => {
  withVi(true, <Live fine disabled />)
  const view = await inlineView()
  expect(view.state.readOnly).toBe(true)
  expect(view.contentDOM.getAttribute("contenteditable")).toBe("false")
  expect(view.contentDOM.getAttribute("tabindex")).toBe("-1")
  expect(getComputedStyle(view.dom.parentElement!).opacity).toBe("0.5")
  expect(view.state.doc.toString()).toBe("hello world")
})

test("the inline editor has the textarea's name, the marker, and the focus ring hooks", async () => {
  withVi(true, <Live fine />)
  const view = await inlineView()
  expect(view.contentDOM.getAttribute("name")).toBe("test-note")
  expect(view.contentDOM.hasAttribute("data-prompt-box")).toBe(true)
  expect(view.dom.parentElement!.getAttribute("data-focus-ring")).toBe("none")
  cleanup()
  withVi(true, <Live />)
  expect(field().hasAttribute("data-prompt-box")).toBe(true)
})

test("the box's css rings the box while the editor has focus", async () => {
  withVi(true, <Live fine />)
  await inlineView()
  const css = Array.from(document.styleSheets)
    .flatMap((s) => Array.from(s.cssRules))
    .map((r) => r.cssText)
    .join("\n")
  expect(css).toContain(":has(.cm-editor.cm-focused)")
})

// The mode pill and mode rules.
function Pill({
  footer,
  note,
  disabled,
  completion,
}: {
  footer?: ReactNode
  note?: { text: string; red?: boolean } | null
  disabled?: boolean
  completion?: boolean
}) {
  const [text, setText] = useState("hello")
  return (
    <>
      <button type="button" onClick={() => setText("")}>
        submit
      </button>
      <PromptBox
        label="Note"
        name="test-note"
        title={TITLE}
        value={text}
        onChange={setText}
        footer={footer}
        note={note}
        disabled={disabled}
        completion={
          completion ? { target: { sessionId: "s-pill" }, harness: "claude", fetcher } : undefined
        }
      />
    </>
  )
}

const pill = () => screen.getByRole("status")
const pressKeys = (view: EditorView, ...keys: string[]) =>
  act(() => keys.forEach((k) => void Vim.handleKey(getCM(view)!, k, "user")))

test("the pill follows the mode and carries its tone", async () => {
  withVi(true, <Pill />, false)
  const view = await inlineView()
  expect([pill().textContent, pill().dataset.tone]).toEqual(["INSERT", "done"])
  pressKeys(view, "<Esc>")
  expect([pill().textContent, pill().dataset.tone]).toEqual(["NORMAL", "running"])
  pressKeys(view, "v")
  expect([pill().textContent, pill().dataset.tone]).toEqual(["VISUAL", "waiting"])
  pressKeys(view, "<Esc>", "i")
  expect(pill().textContent).toBe("INSERT")
  expect(view.contentDOM.closest("[data-vi-mode]")?.getAttribute("data-vi-mode")).toBe("INSERT")
})

test("the pill shows INSERT while the textarea stands in", () => {
  withVi(true, <Pill />, false)
  expect(field().tagName).toBe("TEXTAREA")
  expect(pill().textContent).toBe("INSERT")
})

test("the pill comes before a separator and the footer; alone it has none", () => {
  withVi(true, <Pill footer={<span>the footer</span>} />, false)
  expect(pill().parentElement!.parentElement!.textContent).toBe("INSERT·the footer")
  cleanup()
  withVi(true, <Pill />, false)
  expect(pill().parentElement!.parentElement!.textContent).toBe("INSERT")
})

test("a note and a red refusal stay above the pill line", () => {
  withVi(true, <Pill note={{ text: "no way", red: true }} footer="f" />, false)
  const text = screen.getByText("no way")
  expect(text.compareDocumentPosition(pill()) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
})

test("vi off or a coarse pointer has no pill", () => {
  withVi(false, <Pill footer="f" />, false)
  expect(screen.queryByRole("status")).toBeNull()
  cleanup()
  withVi(true, <Pill />, true)
  expect(screen.queryByRole("status")).toBeNull()
})

test("a disabled box dims the pill and keeps its mode", async () => {
  withVi(true, <Pill disabled />, false)
  await inlineView()
  expect(getComputedStyle(pill().parentElement!).opacity).toBe("0.5")
  expect(pill().textContent).toBe("INSERT")
})

test("the mode survives blur and focus", async () => {
  withVi(true, <Pill />, false)
  const view = await inlineView()
  pressKeys(view, "<Esc>")
  act(() => view.contentDOM.blur())
  act(() => view.focus())
  expect(pill().textContent).toBe("NORMAL")
})

test("an outside clear returns to INSERT; a user dd does not", async () => {
  withVi(true, <Pill />, false)
  const view = await inlineView()
  pressKeys(view, "<Esc>", "d", "d")
  expect(view.state.doc.toString()).toBe("")
  expect(pill().textContent).toBe("NORMAL")
  cleanup()
  withVi(true, <Pill />, false)
  const again = await inlineView()
  pressKeys(again, "<Esc>")
  expect(pill().textContent).toBe("NORMAL")
  fireEvent.click(screen.getByText("submit"))
  expect(pill().textContent).toBe("INSERT")
})

test("the inline menu opens in INSERT only, and Escape closes it first", async () => {
  withVi(true, <Pill completion />, false)
  const view = await inlineView()
  act(() => view.dispatch({ changes: { from: 0, to: 5, insert: "/co" }, selection: { anchor: 3 } }))
  await waitFor(() => expect(options()).toHaveLength(2))
  fireEvent.keyDown(view.contentDOM, { key: "Escape" })
  expect(screen.queryByRole("listbox")).toBeNull()
  expect(pill().textContent).toBe("INSERT")
  pressKeys(view, "<Esc>")
  expect(pill().textContent).toBe("NORMAL")
  expect(screen.queryByRole("listbox")).toBeNull()
  pressKeys(view, "v")
  expect(screen.queryByRole("listbox")).toBeNull()
})

// Cmd-G hands the inline box to the large editor and back.
const openKey = (view: EditorView) =>
  fireEvent.keyDown(view.contentDOM, { key: "g", ctrlKey: true })
const done = () => fireEvent.click(screen.getByText("Done"))
const closed = () => waitFor(() => expect(screen.queryByRole("dialog")).toBeNull())

test("the key opens the large editor in INSERT from every inline mode, with the selection", async () => {
  withVi(true, <Pill />, false)
  const view = await inlineView()
  openKey(view)
  await findLargeEditor(TITLE)
  expect(largeEditorText(TITLE)).toBe("hello")
  expect(viewOf(TITLE).dom.parentElement?.getAttribute("data-vi-mode")).toBe("INSERT")
  done()
  await closed()
  pressKeys(view, "<Esc>", "0", "v", "l", "l")
  openKey(view)
  await findLargeEditor(TITLE)
  const { from, to } = viewOf(TITLE).state.selection.main
  expect(to - from).toBeGreaterThan(0)
  expect(viewOf(TITLE).dom.parentElement?.getAttribute("data-vi-mode")).toBe("INSERT")
})

test("a disabled inline box opens nothing on the key", async () => {
  withVi(true, <Pill disabled />, false)
  const view = await inlineView()
  openKey(view)
  expect(screen.queryByRole("dialog")).toBeNull()
})

test("closing keeps the inline mode: NORMAL stays NORMAL, INSERT stays INSERT", async () => {
  withVi(true, <Pill />, false)
  const view = await inlineView()
  pressKeys(view, "<Esc>")
  openKey(view)
  await findLargeEditor(TITLE)
  pressKeys(viewOf(TITLE), "<Esc>")
  done()
  await closed()
  expect(pill().textContent).toBe("NORMAL")
  expect(view.hasFocus).toBe(true)
  pressKeys(view, "i")
  openKey(view)
  await findLargeEditor(TITLE)
  pressKeys(viewOf(TITLE), "<Esc>")
  done()
  await closed()
  expect(pill().textContent).toBe("INSERT")
})

test("VISUAL comes back as NORMAL with a caret", async () => {
  withVi(true, <Pill />, false)
  const view = await inlineView()
  pressKeys(view, "<Esc>", "0", "v", "l", "l")
  openKey(view)
  await findLargeEditor(TITLE)
  done()
  await closed()
  expect(pill().textContent).toBe("NORMAL")
  const { from, to } = view.state.selection.main
  expect(from).toBe(to)
})

test("the caret is a bar in INSERT and a block in NORMAL", async () => {
  // jsdom has no layout; give every range a box so vim's cursor layer can draw.
  const rect = { left: 10, right: 18, top: 0, bottom: 16, width: 8, height: 16, x: 10, y: 0 }
  const list = Object.assign([rect], { item: () => rect }) as unknown as DOMRectList
  vi.spyOn(Range.prototype, "getClientRects").mockReturnValue(list)
  vi.spyOn(Range.prototype, "getBoundingClientRect").mockReturnValue(rect as DOMRect)
  withVi(true, <Pill />, false)
  const view = await inlineView()
  const block = () => view.scrollDOM.querySelector(".cm-vimCursorLayer .cm-fat-cursor")
  const bar = () =>
    view.scrollDOM.querySelector(".cm-cursorLayer:not(.cm-vimCursorLayer) .cm-cursor")
  await waitFor(() => expect(bar()).not.toBeNull())
  expect(block()).toBeNull()
  pressKeys(view, "<Esc>")
  await waitFor(() => expect(block()).not.toBeNull())
  pressKeys(view, "i")
  await waitFor(() => expect(block()).toBeNull())
})

test("a large editor that empties the text does not reset the box to INSERT", async () => {
  withVi(true, <Pill />, false)
  const view = await inlineView()
  pressKeys(view, "<Esc>")
  openKey(view)
  await typeInLargeEditor(TITLE, "")
  done()
  await closed()
  expect(view.state.doc.toString()).toBe("")
  expect(pill().textContent).toBe("NORMAL")
})

test("after a round trip a real submit that empties the box returns to INSERT", async () => {
  withVi(true, <Pill />, false)
  const view = await inlineView()
  pressKeys(view, "<Esc>")
  openKey(view)
  await findLargeEditor(TITLE)
  done()
  await closed()
  fireEvent.click(screen.getByText("submit"))
  expect(pill().textContent).toBe("INSERT")
})

test("the menu follows the large editor while open, then the inline mode", async () => {
  withVi(true, <Pill completion />, false)
  const view = await inlineView()
  pressKeys(view, "<Esc>")
  openKey(view)
  await typeInLargeEditor(TITLE, "/co")
  await waitFor(() => expect(options()).toHaveLength(2))
  done()
  await closed()
  expect(pill().textContent).toBe("NORMAL")
  expect(screen.queryByRole("listbox")).toBeNull()
})
