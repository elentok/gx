import { cleanup, fireEvent, screen } from "@testing-library/react";
import { useState } from "react";
import { afterEach, expect, test, vi } from "vitest";
import { renderThemed } from "../render";
import { ExpandableTextField } from "./ExpandableTextField";
import { largeEditorOf, typeInLargeEditor } from "./testing";

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

test("the expand button opens a dialog named by the title; the editor is named '<title> text'", () => {
  renderThemed(<Harness />);
  fireEvent.click(expand());
  expect(screen.getByRole("dialog").getAttribute("aria-label")).toBe(TITLE);
  expect(largeEditorOf(TITLE).getAttribute("aria-label")).toBe(`${TITLE} text`);
});

test("the editor edits the same text and Done closes it", () => {
  const onChange = vi.fn();
  renderThemed(<Harness onChange={onChange} />);
  fireEvent.click(expand());
  typeInLargeEditor(TITLE, "typed");
  expect(onChange).toHaveBeenCalledWith("typed");
  fireEvent.click(screen.getByText("Done"));
  expect(screen.queryByRole("dialog")).toBeNull();
  expect(field().value).toBe("typed");
});

test("the editor shows a new value prop: no copy", () => {
  function Parent() {
    const [text, setText] = useState("one");
    return (
      <>
        <button onClick={() => setText("two")}>Set two</button>
        <ExpandableTextField
          label="Note"
          title={TITLE}
          value={text}
          onChange={() => {}}
        />
      </>
    );
  }
  renderThemed(<Parent />);
  fireEvent.click(expand());
  expect(largeEditorOf(TITLE).value).toBe("one");
  fireEvent.click(screen.getByText("Set two"));
  expect(largeEditorOf(TITLE).value).toBe("two");
});

test("a disabled field has a disabled expand button", () => {
  renderThemed(<Harness disabled />);
  expect((expand() as HTMLButtonElement).disabled).toBe(true);
});

test("the selection goes to the editor on open and back to the field on close", () => {
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
  const editor = largeEditorOf(TITLE);
  expect([editor.selectionStart, editor.selectionEnd]).toEqual([2, 7]);
  editor.setSelectionRange(1, 4);
  fireEvent.click(screen.getByText("Done"));
  expect([field().selectionStart, field().selectionEnd]).toEqual([1, 4]);
});

test("Escape closes the editor", () => {
  renderThemed(<Harness />);
  fireEvent.click(expand());
  fireEvent.keyDown(window, { key: "Escape" });
  expect(screen.queryByRole("dialog")).toBeNull();
});
