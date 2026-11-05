import { fireEvent, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { WideContext } from "../../useWide";
import { renderThemed } from "../render";
import { MoreActionsMenu } from "./MoreActionsMenu";

const sections = (a = vi.fn(), b = vi.fn()) => [
  [{ value: "copy", label: "Copy ID", hint: "gx:x", onSelect: a }],
  [{ value: "delete", label: "Delete", danger: true, onSelect: b }],
];

const renderMenu = (wide: boolean, s = sections()) =>
  renderThemed(
    <WideContext.Provider value={wide}>
      <MoreActionsMenu sections={s} />
    </WideContext.Provider>,
  );

describe("MoreActionsMenu", () => {
  it("renders the button closed, then both sections with a rule between them", async () => {
    const { getByRole, findByRole, queryByRole, getAllByRole } = renderMenu(true);
    expect(queryByRole("menu")).toBeNull();
    fireEvent.click(getByRole("button", { name: "More actions" }));
    await findByRole("menuitem", { name: /Copy ID/ });
    expect(getAllByRole("menuitem").map((i) => i.textContent)).toEqual(["Copy IDgx:x", "Delete"]);
    expect(getAllByRole("separator")).toHaveLength(1);
  });

  it("lays an inline hint on the label's row and a plain hint under it", async () => {
    const s = [
      [
        { value: "a", label: "Inline", hint: "h", inlineHint: true, onSelect: vi.fn() },
        { value: "b", label: "Below", hint: "h", onSelect: vi.fn() },
      ],
    ];
    const { getByRole, findByRole } = renderMenu(true, s);
    fireEvent.click(getByRole("button", { name: "More actions" }));
    const inline = await findByRole("menuitem", { name: /Inline/ });
    expect(getComputedStyle(inline).flexDirection).toBe("row");
    expect(getComputedStyle(getByRole("menuitem", { name: /Below/ })).flexDirection).toBe("column");
  });

  it("closes on pick and runs the item", async () => {
    const copy = vi.fn();
    const { getByRole, findByRole, queryByRole } = renderMenu(true, sections(copy));
    fireEvent.click(getByRole("button", { name: "More actions" }));
    fireEvent.click(await findByRole("menuitem", { name: /Copy ID/ }));
    await waitFor(() => expect(copy).toHaveBeenCalled());
    await waitFor(() => expect(queryByRole("menu")).toBeNull());
  });

  it("closes on Escape", async () => {
    const { getByRole, findByRole, queryByRole } = renderMenu(true);
    fireEvent.click(getByRole("button", { name: "More actions" }));
    const menu = await findByRole("menu");
    await waitFor(() => expect(document.activeElement).toBe(menu));
    fireEvent.keyDown(menu, { key: "Escape" });
    await waitFor(() => expect(queryByRole("menu")).toBeNull());
  });

  it("shows a grab handle on a phone and none on desktop", async () => {
    const phone = renderMenu(false);
    fireEvent.click(phone.getByRole("button", { name: "More actions" }));
    await phone.findByRole("menu");
    expect(phone.getByTestId("grab-handle")).toBeTruthy();
    phone.unmount();

    const desktop = renderMenu(true);
    fireEvent.click(desktop.getByRole("button", { name: "More actions" }));
    await desktop.findByRole("menu");
    expect(desktop.queryByTestId("grab-handle")).toBeNull();
  });
});
