import { cleanup, fireEvent, screen } from "@testing-library/react";
import { useState } from "react";
import { afterEach, expect, test, vi } from "vitest";
import { renderThemed } from "../render";
import { ExpandableTextField } from "./ExpandableTextField";
import { findLargeEditor, largeEditorText, typeInLargeEditor, viewOf } from "./testing";

afterEach(cleanup);

const TITLE = "Note on Q2 - Pick";
const expand = () => screen.getByLabelText("Open the note in a large editor");
const field = () => screen.getByLabelText("Note") as HTMLTextAreaElement;

function Harness({
  onChange = () => {},
  disabled,
}: {
  onChange?: (t: string) => void;
  disabled?: boolean;
}) {
  const [text, setText] = useState("");
  return (
    <ExpandableTextField
      label="Note"
      title={TITLE}
      rows={3}
      placeholder="Type"
      disabled={disabled}
      value={text}
      onChange={(t) => (setText(t), onChange(t))}
    />
  );
}

test("the textarea gets value, rows, label, placeholder and disabled", () => {
  renderThemed(
    <ExpandableTextField
      label="Note"
      title={TITLE}
      rows={3}
      placeholder="Type"
      disabled
      value="hi"
      onChange={() => {}}
    />,
  );
  const area = field();
  expect(area.value).toBe("hi");
  expect(area.rows).toBe(3);
  expect(area.placeholder).toBe("Type");
  expect(area.disabled).toBe(true);
});

test("typing calls onChange", () => {
  const onChange = vi.fn();
  renderThemed(<Harness onChange={onChange} />);
  fireEvent.change(field(), { target: { value: "abc" } });
  expect(onChange).toHaveBeenCalledWith("abc");
});

test("the expand button opens a dialog named by the title; the editor is a multiline textbox named '<title> text'", async () => {
  renderThemed(<Harness />);
  fireEvent.click(expand());
  expect(screen.getByRole("dialog").getAttribute("aria-label")).toBe(TITLE);
  const editor = await findLargeEditor(TITLE);
  expect(editor.getAttribute("aria-label")).toBe(`${TITLE} text`);
  expect(editor.getAttribute("aria-multiline")).toBe("true");
});

test("the editor edits the same text and Done closes it", async () => {
  const onChange = vi.fn();
  renderThemed(<Harness onChange={onChange} />);
  fireEvent.click(expand());
  await typeInLargeEditor(TITLE, "typed");
  expect(onChange).toHaveBeenCalledWith("typed");
  fireEvent.click(screen.getByText("Done"));
  expect(screen.queryByRole("dialog")).toBeNull();
  expect(field().value).toBe("typed");
});

test("the editor shows a new value prop and keeps the caret: no copy", async () => {
  const onChange = vi.fn();
  function Parent() {
    const [text, setText] = useState("hello world");
    return (
      <>
        <button onClick={() => setText("hello world!")}>Set new</button>
        <ExpandableTextField
          label="Note"
          title={TITLE}
          value={text}
          onChange={onChange}
        />
      </>
    );
  }
  renderThemed(<Parent />);
  field().setSelectionRange(2, 2);
  fireEvent.click(expand());
  await findLargeEditor(TITLE);
  expect(largeEditorText(TITLE)).toBe("hello world");
  fireEvent.click(screen.getByText("Set new"));
  expect(largeEditorText(TITLE)).toBe("hello world!");
  expect(viewOf(TITLE).state.selection.main.head).toBe(2);
  expect(onChange).not.toHaveBeenCalled();
});

test("Tab does not insert a tab character", async () => {
  renderThemed(<Harness />);
  fireEvent.click(expand());
  const editor = await findLargeEditor(TITLE);
  await typeInLargeEditor(TITLE, "abc");
  fireEvent.keyDown(editor, { key: "Tab" });
  fireEvent.keyDown(editor, { key: "Tab", shiftKey: true });
  expect(largeEditorText(TITLE)).toBe("abc");
});

test("the placeholder shows while the editor is empty", async () => {
  renderThemed(<Harness />);
  fireEvent.click(expand());
  await findLargeEditor(TITLE);
  expect(screen.getByRole("dialog").querySelector(".cm-placeholder")?.textContent).toBe("Type");
});

test("a disabled field has a disabled expand button", () => {
  renderThemed(<Harness disabled />);
  expect((expand() as HTMLButtonElement).disabled).toBe(true);
});

test("the selection goes to the editor on open and back to the field on close", async () => {
  renderThemed(
    <ExpandableTextField
      label="Note"
      title={TITLE}
      value="hello world"
      onChange={() => {}}
    />,
  );
  field().setSelectionRange(2, 7);
  fireEvent.click(expand());
  await findLargeEditor(TITLE);
  const sel = () => {
    const { from, to } = viewOf(TITLE).state.selection.main;
    return [from, to];
  };
  expect(sel()).toEqual([2, 7]);
  viewOf(TITLE).dispatch({ selection: { anchor: 1, head: 4 } });
  fireEvent.click(screen.getByText("Done"));
  expect([field().selectionStart, field().selectionEnd]).toEqual([1, 4]);
});

test("Escape closes the editor", () => {
  renderThemed(<Harness />);
  fireEvent.click(expand());
  fireEvent.keyDown(window, { key: "Escape" });
  expect(screen.queryByRole("dialog")).toBeNull();
});
