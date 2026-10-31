import { fireEvent, screen } from "@testing-library/react";

/** The open large editor of the field titled `title`. */
export function largeEditorOf(title: string): HTMLTextAreaElement {
  return screen.getByLabelText(`${title} text`) as HTMLTextAreaElement;
}

/** Sets the text of the open large editor, as typing would. */
export function typeInLargeEditor(title: string, text: string) {
  fireEvent.change(largeEditorOf(title), { target: { value: text } });
}
