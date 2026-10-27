import { cleanup, fireEvent, screen } from "@testing-library/react";
import { useState } from "react";
import { afterEach, expect, test, vi } from "vitest";
import { renderThemed } from "../ui/render";
import { NoteBox } from "./NoteBox";

afterEach(cleanup);

function Harness({ onChange = () => {} }: { onChange?: (t: string) => void }) {
  const [text, setText] = useState("");
  return (
    <NoteBox
      name="question 2"
      title="Note on Q2 - Pick"
      placeholder="Note"
      value={text}
      onChange={(t) => (setText(t), onChange(t))}
    />
  );
}

test("is five rows tall and scrolls into view when it opens", () => {
  const scrollIntoView = vi.fn();
  Element.prototype.scrollIntoView = scrollIntoView;
  renderThemed(<Harness />);
  expect(screen.getByLabelText("Note text for question 2")).toHaveProperty(
    "rows",
    5,
  );
  expect(scrollIntoView).toHaveBeenCalled();
});

test("the large editor edits the same text and Done closes it", () => {
  const onChange = vi.fn();
  renderThemed(<Harness onChange={onChange} />);
  fireEvent.click(screen.getByLabelText("Open the note in a large editor"));
  expect(screen.getByRole("dialog").getAttribute("aria-label")).toBe(
    "Note on Q2 - Pick",
  );
  expect(screen.getByText("Saved as you type")).toBeTruthy();
  fireEvent.change(screen.getByLabelText("Note text in the large editor"), {
    target: { value: "typed" },
  });
  expect(onChange).toHaveBeenCalledWith("typed");
  fireEvent.click(screen.getByText("Done"));
  expect(screen.queryByRole("dialog")).toBeNull();
  expect(screen.getByLabelText("Note text for question 2")).toHaveProperty(
    "value",
    "typed",
  );
});

test("Escape closes the modal", () => {
  renderThemed(<Harness />);
  fireEvent.click(screen.getByLabelText("Open the note in a large editor"));
  fireEvent.keyDown(window, { key: "Escape" });
  expect(screen.queryByRole("dialog")).toBeNull();
});
