import { cleanup, fireEvent, screen } from "@testing-library/react";
import { afterEach, describe, expect, test, vi } from "vitest";
import { CoarsePointerContext, WideContext } from "../../useWide";
import { renderThemed } from "../render";
import { LargeEditor } from "./LargeEditor";

afterEach(cleanup);

const TITLE = "Note on Q2 - Pick";

const cases = [
  { name: "wide, mouse", wide: true, coarse: false, header: true, footer: true, bar: false },
  { name: "wide, touch", wide: true, coarse: true, header: true, footer: false, bar: true },
  { name: "narrow, mouse", wide: false, coarse: false, header: true, footer: true, bar: false },
  { name: "narrow, touch", wide: false, coarse: true, header: false, footer: false, bar: true },
];

const mount = ({ wide, coarse }: { wide: boolean; coarse: boolean }, onClose = vi.fn()) => {
  renderThemed(
    <WideContext.Provider value={wide}>
      <CoarsePointerContext.Provider value={coarse}>
        <LargeEditor title={TITLE} value="" onChange={() => {}} selection={{ start: 0, end: 0 }} onClose={onClose} />
      </CoarsePointerContext.Provider>
    </WideContext.Provider>,
  );
  const dialog = screen.getByRole("dialog", { name: TITLE });
  return { dialog, backdrop: dialog.parentElement!, onClose };
};

describe.each(cases)("$name", (c) => {
  test("has the parts this layout needs, one Done and no x button", () => {
    const { dialog } = mount(c);
    expect(screen.queryByText(TITLE) !== null).toBe(c.header);
    expect(screen.queryByText("Saved as you type") !== null).toBe(c.footer);
    expect(screen.queryByTestId("editor-bar") !== null).toBe(c.bar);
    expect(screen.queryByLabelText("Close")).toBeNull();
    expect(screen.getAllByText("Done")).toHaveLength(1);
    expect(dialog.getAttribute("aria-label")).toBe(TITLE);
  });

  test("Done closes and Escape does not", () => {
    const { onClose } = mount(c);
    fireEvent.keyDown(window, { key: "Escape" });
    expect(onClose).not.toHaveBeenCalled();
    fireEvent.click(screen.getByText("Done"));
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  test(`a backdrop press ${c.wide ? "closes" : "does not close"}`, () => {
    const { backdrop, onClose } = mount(c);
    fireEvent.mouseDown(backdrop);
    fireEvent.click(backdrop);
    expect(onClose).toHaveBeenCalledTimes(c.wide ? 1 : 0);
  });
});

test("the bar's Done is 44px high", () => {
  mount({ wide: false, coarse: true });
  expect(getComputedStyle(screen.getByText("Done")).height).toBe("44px");
});
