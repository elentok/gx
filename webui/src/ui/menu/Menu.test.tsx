import { fireEvent, waitFor, within } from "@testing-library/react";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { WideContext } from "../../useWide";
import { renderThemed } from "../render";
import { GxMenu, menuGroups, type MenuEntry } from "./Menu";

const trigger = <button type="button" aria-label="Menu">open</button>;

const open = async (r: ReturnType<typeof renderThemed>) => {
  fireEvent.click(r.getByRole("button", { name: "Menu" }));
  return r.findByRole("menu", { name: "Menu" });
};

describe("GxMenu on the phone", () => {
  const PANEL_HEIGHT = 300;
  const rect = (r: Partial<DOMRect>) => ({ x: 0, y: 0, width: 0, height: 0, top: 0, left: 0, right: 0, bottom: 0, ...r }) as DOMRect;

  // jsdom has no layout: the trigger sits where the test says, the panel is PANEL_HEIGHT tall.
  const layout = (button: Partial<DOMRect>) =>
    vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockImplementation(function (this: HTMLElement) {
      return this.getAttribute("role") === "menu" ? rect({ height: PANEL_HEIGHT, width: 280 }) : rect(button);
    });
  const phone = (items: MenuEntry[], onClosed?: () => void) =>
    renderThemed(
      <WideContext.Provider value={false}>
        <GxMenu trigger={trigger} items={items} onClosed={onClosed} />
      </WideContext.Provider>,
    );
  const sub: MenuEntry[] = [
    { value: "a", label: "A", onSelect: vi.fn() },
    { kind: "submenu", value: "s", label: "Sub", items: [{ value: "inner", label: "Inner", onSelect: vi.fn() }] },
  ];

  afterEach(() => vi.restoreAllMocks());

  it("opens below the button with its right edge on the button's, 280px wide with 48px rows", async () => {
    Object.defineProperty(window, "innerHeight", { value: 800, configurable: true });
    Object.defineProperty(window, "innerWidth", { value: 390, configurable: true });
    layout({ top: 100, bottom: 136, left: 300, right: 336 });
    const menu = await open(phone(sub));
    await waitFor(() => expect(menu.style.visibility === "hidden").toBe(false));
    expect(menu.style.left).toBe("56px");
    expect(menu.style.top).toBe("140px");
    expect(getComputedStyle(menu).width).toBe("280px");
    expect(getComputedStyle(within(menu).getByRole("menuitem", { name: "A" })).minHeight).toBe("48px");
  });

  it("flips above when there is no room below", async () => {
    Object.defineProperty(window, "innerHeight", { value: 800, configurable: true });
    layout({ top: 700, bottom: 736, left: 300, right: 336 });
    const menu = await open(phone(sub));
    await waitFor(() => expect(menu.style.bottom).toBe("104px"));
    expect(menu.style.top).toBe("");
  });

  it("keeps 8px inside the screen", async () => {
    Object.defineProperty(window, "innerWidth", { value: 390, configurable: true });
    layout({ top: 100, bottom: 136, left: 0, right: 36 });
    const menu = await open(phone(sub));
    await waitFor(() => expect(menu.style.left).toBe("8px"));
  });

  it("dims behind the menu and closes on a tap outside", async () => {
    layout({ top: 100, bottom: 136, left: 300, right: 336 });
    const onClosed = vi.fn();
    const r = phone(sub, onClosed);
    const menu = await open(r);
    fireEvent.click(menu);
    expect(r.queryByRole("menu")).toBeTruthy();
    fireEvent.click(r.getByTestId("menu-dim"));
    await waitFor(() => expect(r.queryByRole("menu")).toBeNull());
    expect(onClosed).toHaveBeenCalledTimes(1);
  });

  it("swaps a submenu into the same panel with a Back row and divider, and Back returns", async () => {
    layout({ top: 100, bottom: 136, left: 300, right: 336 });
    const r = phone(sub);
    const menu = await open(r);
    await waitFor(() => expect(menu.style.top).toBe("140px"));
    fireEvent.click(r.getByRole("menuitem", { name: /Sub/ }));
    expect(r.getAllByRole("menu").length).toBe(1);
    expect(r.queryByRole("menuitem", { name: "A" })).toBeNull();
    const rows = r.getAllByRole("menuitem").map((e) => e.textContent);
    expect(rows).toEqual(["‹ Back", "Inner"]);
    expect(r.getByRole("separator")).toBeTruthy();
    expect(menu.style.top).toBe("140px");
    fireEvent.click(r.getByRole("menuitem", { name: /Back/ }));
    expect(r.getByRole("menuitem", { name: "A" })).toBeTruthy();
  });

  it("closes after choosing a submenu option", async () => {
    layout({ top: 100, bottom: 136, left: 300, right: 336 });
    const pick = vi.fn();
    const r = phone([{ kind: "submenu", value: "s", label: "Sub", items: [{ value: "i", label: "Inner", onSelect: pick }] }]);
    await open(r);
    fireEvent.click(r.getByRole("menuitem", { name: /Sub/ }));
    fireEvent.click(r.getByRole("menuitem", { name: "Inner" }));
    expect(pick).toHaveBeenCalled();
    await waitFor(() => expect(r.queryByRole("menu")).toBeNull());
  });

  it("works from the keyboard: arrows, Enter, Left for Back, Escape", async () => {
    layout({ top: 100, bottom: 136, left: 300, right: 336 });
    const r = phone(sub);
    const menu = await open(r);
    await waitFor(() => expect(document.activeElement).toBe(menu));
    const highlighted = () => r.getAllByRole("menuitem").find((e) => e.hasAttribute("data-highlighted"))?.textContent;
    fireEvent.keyDown(menu, { key: "ArrowDown" });
    expect(highlighted()).toBe("A");
    fireEvent.keyDown(menu, { key: "End" });
    expect(highlighted()).toBe("Sub›");
    fireEvent.keyDown(menu, { key: "Enter" });
    expect(r.getByRole("menuitem", { name: "Inner" })).toBeTruthy();
    fireEvent.keyDown(menu, { key: "ArrowLeft" });
    expect(r.getByRole("menuitem", { name: "A" })).toBeTruthy();
    fireEvent.keyDown(menu, { key: "Escape" });
    await waitFor(() => expect(r.queryByRole("menu")).toBeNull());
    expect(document.activeElement).toBe(r.getByRole("button", { name: "Menu" }));
  });
});

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

  it("puts a rule between non-empty groups, and an inline hint on the label's row", async () => {
    const items = menuGroups([
      [
        { value: "a", label: "Inline", hint: "h", inlineHint: true, onSelect: vi.fn() },
        { value: "b", label: "Below", hint: "h", onSelect: vi.fn() },
      ],
      [],
      [{ value: "c", label: "Last", onSelect: vi.fn() }],
    ]);
    const r = renderThemed(<GxMenu trigger={trigger} items={items} />);
    await open(r);
    expect(r.getAllByRole("separator")).toHaveLength(1);
    expect(r.getAllByRole("menuitem").map((i) => i.textContent)).toEqual(["Inlineh", "Belowh", "Last"]);
    expect(getComputedStyle(r.getByRole("menuitem", { name: /Inline/ })).flexDirection).toBe("row");
    expect(getComputedStyle(r.getByRole("menuitem", { name: /Below/ })).flexDirection).toBe("column");
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
