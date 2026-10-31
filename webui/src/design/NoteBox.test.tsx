import { cleanup, screen } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { renderThemed } from "../ui/render";
import { NoteBox } from "./NoteBox";

afterEach(cleanup);

test("is five rows tall and scrolls into view when it opens", () => {
  const scrollIntoView = vi.fn();
  Element.prototype.scrollIntoView = scrollIntoView;
  renderThemed(
    <NoteBox
      name="question 2"
      title="Note on Q2 - Pick"
      placeholder="Note"
      value=""
      onChange={() => {}}
    />,
  );
  expect(screen.getByLabelText("Note text for question 2")).toHaveProperty(
    "rows",
    5,
  );
  expect(scrollIntoView).toHaveBeenCalled();
});
