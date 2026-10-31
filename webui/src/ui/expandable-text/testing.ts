import { EditorView } from "@codemirror/view";
import { act, screen } from "@testing-library/react";

/** The open large editor's content element of the field titled `title`. Async: the editor loads on demand. */
export async function findLargeEditor(title: string): Promise<HTMLElement> {
  return screen.findByRole("textbox", { name: `${title} text` });
}

/** The open large editor of the field titled `title`. */
export function largeEditorOf(title: string): HTMLElement {
  return screen.getByRole("textbox", { name: `${title} text` });
}

export function viewOf(title: string): EditorView {
  const view = EditorView.findFromDOM(largeEditorOf(title));
  if (!view) throw new Error(`no editor view for ${title}`);
  return view;
}

/** The text of the open large editor. */
export function largeEditorText(title: string): string {
  return viewOf(title).state.doc.toString();
}

/** Sets the text of the open large editor, as typing would. */
export async function typeInLargeEditor(title: string, text: string) {
  await findLargeEditor(title);
  const view = viewOf(title);
  act(() => {
    view.dispatch({
      changes: { from: 0, to: view.state.doc.length, insert: text },
      selection: { anchor: text.length },
      userEvent: "input.type",
    });
  });
}
