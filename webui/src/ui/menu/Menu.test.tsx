import { fireEvent, waitFor } from "@testing-library/react";
import { useState } from "react";
import { describe, expect, it, vi } from "vitest";
import { renderThemed } from "../render";
import { GxMenu, type MenuEntry } from "./Menu";

const trigger = <button type="button" aria-label="Menu">open</button>;

const open = async (r: ReturnType<typeof renderThemed>) => {
  fireEvent.click(r.getByRole("button", { name: "Menu" }));
  return r.findByRole("menu", { name: "Menu" });
};

describe("GxMenu", () => {
  it("renders every item kind", async () => {
    const items: MenuEntry[] = [
      { value: "a", label: "Action", hint: "h", onSelect: vi.fn() },
      { kind: "separator" },
      { kind: "checkbox", value: "c", label: "Check", checked: true, onCheckedChange: vi.fn() },
      {
        kind: "radio-group",
        value: "r",
        current: "x",
        onValueChange: vi.fn(),
        options: [
          { value: "x", label: "X" },
          { value: "y", label: "Y" },
        ],
      },
      { kind: "submenu", value: "s", label: "Sub", items: [] },
      { kind: "content", value: "ct", content: <p>custom</p> },
    ];
    const r = renderThemed(<GxMenu trigger={trigger} items={items} />);
    await open(r);
    expect(r.getByRole("menuitem", { name: /Action/ })).toBeTruthy();
    expect(r.getByRole("separator")).toBeTruthy();
    expect(r.getByRole("menuitemcheckbox", { name: /Check/ }).getAttribute("aria-checked")).toBe("true");
    expect(r.getAllByRole("menuitemradio").map((e) => e.getAttribute("aria-checked"))).toEqual(["true", "false"]);
    expect(r.getByRole("menuitem", { name: /Sub/ })).toBeTruthy();
    expect(r.getByText("custom")).toBeTruthy();
  });

  it("opens a submenu beside its item and runs its action", async () => {
    const pick = vi.fn();
    const items: MenuEntry[] = [
      { kind: "submenu", value: "s", label: "Sub", items: [{ value: "inner", label: "Inner", onSelect: pick }] },
    ];
    const r = renderThemed(<GxMenu trigger={trigger} items={items} />);
    await open(r);
    fireEvent.click(r.getByRole("menuitem", { name: /Sub/ }));
    const inner = await r.findByRole("menuitem", { name: "Inner" });
    expect(r.getAllByRole("menu").length).toBe(2);
    fireEvent.click(inner);
    await waitFor(() => expect(pick).toHaveBeenCalled());
  });

  it("shows a disabled reason and does not run the item", async () => {
    const pick = vi.fn();
    const items: MenuEntry[] = [
      { value: "a", label: "Go", disabled: true, disabledReason: "Nothing to do", onSelect: pick },
    ];
    const r = renderThemed(<GxMenu trigger={trigger} items={items} />);
    await open(r);
    const item = r.getByRole("menuitem", { name: /Go/ });
    expect(item.textContent).toContain("Nothing to do");
    expect(item.hasAttribute("data-disabled")).toBe(true);
    fireEvent.click(item);
    await new Promise((res) => setTimeout(res, 20));
    expect(pick).not.toHaveBeenCalled();
  });

  it("closes on pick unless closeOnSelect is false", async () => {
    const items: MenuEntry[] = [
      { value: "stay", label: "Stay", closeOnSelect: false, onSelect: vi.fn() },
      { value: "go", label: "Go", onSelect: vi.fn() },
    ];
    const onClosed = vi.fn();
    const r = renderThemed(<GxMenu trigger={trigger} items={items} onClosed={onClosed} />);
    await open(r);
    fireEvent.click(r.getByRole("menuitem", { name: "Stay" }));
    await new Promise((res) => setTimeout(res, 20));
    expect(r.queryByRole("menu")).toBeTruthy();
    expect(onClosed).not.toHaveBeenCalled();
    fireEvent.click(r.getByRole("menuitem", { name: "Go" }));
    await waitFor(() => expect(r.queryByRole("menu")).toBeNull());
    expect(onClosed).toHaveBeenCalledTimes(1);
  });

  it("runs a checkbox and a radio change", async () => {
    const check = vi.fn();
    const radio = vi.fn();
    const items: MenuEntry[] = [
      { kind: "checkbox", value: "c", label: "Check", checked: false, onCheckedChange: check, closeOnSelect: false },
      {
        kind: "radio-group",
        value: "r",
        current: "x",
        onValueChange: radio,
        options: [
          { value: "x", label: "X" },
          { value: "y", label: "Y" },
        ],
      },
    ];
    const r = renderThemed(<GxMenu trigger={trigger} items={items} />);
    await open(r);
    fireEvent.click(r.getByRole("menuitemcheckbox"));
    await waitFor(() => expect(check).toHaveBeenCalledWith(true));
    fireEvent.click(r.getByRole("menuitemradio", { name: "Y" }));
    await waitFor(() => expect(radio).toHaveBeenCalledWith("y"));
  });

  it("closes when the external signal changes", async () => {
    function Host() {
      const [n, setN] = useState(0);
      return (
        <>
          <button type="button" onClick={() => setN(n + 1)}>
            signal
          </button>
          <GxMenu trigger={trigger} items={[{ value: "a", label: "A", onSelect: vi.fn() }]} closeSignal={n} />
        </>
      );
    }
    const r = renderThemed(<Host />);
    await open(r);
    fireEvent.click(r.getByRole("button", { name: "signal" }));
    await waitFor(() => expect(r.queryByRole("menu")).toBeNull());
  });

  it("moves with arrow keys, Home and End, and marks the active item", async () => {
    const items: MenuEntry[] = [
      { value: "a", label: "A", onSelect: vi.fn() },
      { value: "b", label: "B", onSelect: vi.fn() },
      { value: "c", label: "C", onSelect: vi.fn() },
    ];
    const r = renderThemed(<GxMenu trigger={trigger} items={items} />);
    const menu = await open(r);
    await waitFor(() => expect(document.activeElement).toBe(menu));
    const highlighted = () => r.getAllByRole("menuitem").find((e) => e.hasAttribute("data-highlighted"))?.textContent;
    fireEvent.keyDown(menu, { key: "ArrowDown" });
    await waitFor(() => expect(highlighted()).toBe("A"));
    fireEvent.keyDown(menu, { key: "ArrowDown" });
    await waitFor(() => expect(highlighted()).toBe("B"));
    fireEvent.keyDown(menu, { key: "End" });
    await waitFor(() => expect(highlighted()).toBe("C"));
    fireEvent.keyDown(menu, { key: "Home" });
    await waitFor(() => expect(highlighted()).toBe("A"));
    fireEvent.keyDown(menu, { key: "ArrowUp" });
    await waitFor(() => expect(highlighted()).toBe("C"));
  });

  it("selects with Enter", async () => {
    const pick = vi.fn();
    const r = renderThemed(<GxMenu trigger={trigger} items={[{ value: "a", label: "A", onSelect: pick }]} />);
    const menu = await open(r);
    await waitFor(() => expect(document.activeElement).toBe(menu));
    fireEvent.keyDown(menu, { key: "ArrowDown" });
    await waitFor(() => expect(r.getByRole("menuitem").hasAttribute("data-highlighted")).toBe(true));
    fireEvent.keyDown(menu, { key: "Enter" });
    await waitFor(() => expect(pick).toHaveBeenCalled());
  });

  it("closes on Escape and returns focus to the trigger", async () => {
    const onClosed = vi.fn();
    const r = renderThemed(<GxMenu trigger={trigger} items={[{ value: "a", label: "A", onSelect: vi.fn() }]} onClosed={onClosed} />);
    const menu = await open(r);
    await waitFor(() => expect(document.activeElement).toBe(menu));
    fireEvent.keyDown(menu, { key: "Escape" });
    await waitFor(() => expect(r.queryByRole("menu")).toBeNull());
    await waitFor(() => expect(document.activeElement).toBe(r.getByRole("button", { name: "Menu" })));
    expect(onClosed).toHaveBeenCalledTimes(1);
  });
});
