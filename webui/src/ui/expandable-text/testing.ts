import { EditorView } from "@codemirror/view"
import { act, screen, waitFor } from "@testing-library/react"
import { expect } from "vitest"
import type { Connection, ConnectionState } from "../../connection"

/** A connection whose snapshot has vi mode on, for a test that renders a PromptBox call site. */
export function viConnection(extra: Record<string, unknown> = {}): Connection {
  const state = {
    snapshot: { sessions: [], tickets: [], epics: [], vi_mode: true, vi_escape: "", ...extra },
    connected: true,
    loadedOnce: true,
    tick: 0,
  } as unknown as ConnectionState
  return {
    getState: () => state,
    subscribe: () => () => {},
    onEvent: () => () => {},
    refresh: () => {},
  } as unknown as Connection
}

/** Waits for the inline box named `name` to be a CodeMirror editor; a textarea fails. */
export async function expectViEditor(name: string) {
  await waitFor(() => {
    const box = screen.getByRole("textbox", { name })
    expect(box.tagName).not.toBe("TEXTAREA")
    expect(EditorView.findFromDOM(box)).not.toBeNull()
  })
}

/** The open large editor's content element of the field titled `title`. Async: the editor loads on demand. */
export async function findLargeEditor(title: string): Promise<HTMLElement> {
  return screen.findByRole("textbox", { name: `${title} text` })
}

/** The open large editor of the field titled `title`. */
export function largeEditorOf(title: string): HTMLElement {
  return screen.getByRole("textbox", { name: `${title} text` })
}

export function viewOf(title: string): EditorView {
  const view = EditorView.findFromDOM(largeEditorOf(title))
  if (!view) throw new Error(`no editor view for ${title}`)
  return view
}

/** The text of the open large editor. */
export function largeEditorText(title: string): string {
  return viewOf(title).state.doc.toString()
}

/** Sets the text of the open large editor, as typing would. */
export async function typeInLargeEditor(title: string, text: string) {
  await findLargeEditor(title)
  const view = viewOf(title)
  act(() => {
    view.dispatch({
      changes: { from: 0, to: view.state.doc.length, insert: text },
      selection: { anchor: text.length },
      userEvent: "input.type",
    })
  })
}
