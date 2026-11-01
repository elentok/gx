import { Menu } from "@chakra-ui/react";
import { fireEvent, waitFor } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { renderThemed } from "../render";
import { InfoTip } from "./InfoTip";

const TEXT = "Resets Friday 09:00";

function setup() {
  const view = renderThemed(<InfoTip label="Reset time" content={TEXT} />);
  const trigger = view.getByRole("button", { name: "Reset time" });
  const shown = () => view.queryByText(TEXT) !== null;
  return { ...view, trigger, shown };
}

describe("InfoTip chip variant", () => {
  it("shows its text, not the 14px circle, and opens on focus", async () => {
    const v = renderThemed(
      <InfoTip variant="chip" label="Burn" content={TEXT}>
        ▲ 7 vs 5 pts
      </InfoTip>,
    );
    const trigger = v.getByRole("button", { name: "Burn" });
    expect(trigger.textContent).toBe("▲ 7 vs 5 pts");
    fireEvent.focus(trigger);
    expect(await v.findByText(TEXT)).toBeTruthy();
  });
});

describe("InfoTip", () => {
  it("is closed at rest and names the trigger from label", () => {
    const { shown, trigger } = setup();
    expect(shown()).toBe(false);
    expect(trigger.getAttribute("aria-label")).toBe("Reset time");
  });

  it("opens on hover and closes on pointer leave", async () => {
    const { trigger, findByText, shown } = setup();
    fireEvent.pointerEnter(trigger);
    fireEvent.pointerMove(trigger);
    await findByText(TEXT);
    fireEvent.pointerLeave(trigger);
    await waitFor(() => expect(shown()).toBe(false));
  });

  it("opens on focus and closes on blur", async () => {
    const { trigger, findByText, shown } = setup();
    fireEvent.focus(trigger);
    await findByText(TEXT);
    fireEvent.blur(trigger);
    await waitFor(() => expect(shown()).toBe(false));
  });

  it("opens on click and closes on a second click", async () => {
    const { trigger, findByText, shown } = setup();
    fireEvent.click(trigger);
    await findByText(TEXT);
    fireEvent.click(trigger);
    await waitFor(() => expect(shown()).toBe(false));
  });

  it("closes on Escape", async () => {
    const { trigger, findByText, shown } = setup();
    fireEvent.click(trigger);
    await findByText(TEXT);
    fireEvent.keyDown(trigger, { key: "Escape" });
    await waitFor(() => expect(shown()).toBe(false));
  });

  it("works inside an open menu: Escape closes the tooltip first, the menu second", async () => {
    const { getByRole, findByRole, findByText, queryByRole } = renderThemed(
      <Menu.Root closeOnSelect={false} lazyMount unmountOnExit>
        <Menu.Trigger asChild>
          <button type="button">Open</button>
        </Menu.Trigger>
        <Menu.Positioner>
          <Menu.Content>
            <Menu.Item value="a">Item</Menu.Item>
            <InfoTip label="Reset time" content={TEXT} />
          </Menu.Content>
        </Menu.Positioner>
      </Menu.Root>,
    );
    fireEvent.click(getByRole("button", { name: "Open" }));
    await findByRole("menuitem", { name: "Item" });
    const marker = getByRole("button", { name: "Reset time" });
    marker.focus();
    await findByText(TEXT);
    expect(queryByRole("menuitem", { name: "Item" }), "menu stays open on focus").not.toBeNull();
    fireEvent.keyDown(marker, { key: "Escape" });
    await waitFor(() => expect(queryByRole("tooltip")).toBeNull());
    expect(queryByRole("menuitem", { name: "Item" })).not.toBeNull();
    fireEvent.keyDown(marker, { key: "Escape" });
    await waitFor(() => expect(queryByRole("menuitem", { name: "Item" })).toBeNull());
  });
});
