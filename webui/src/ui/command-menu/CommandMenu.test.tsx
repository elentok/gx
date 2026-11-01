import { fireEvent } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { renderThemed } from "../render";
import { TONES } from "../tone";
import { CommandMenu, type CommandMenuRow } from "./CommandMenu";

const rows: CommandMenuRow[] = [
  { name: "/clear", description: "Start a new conversation", source: "built-in", segments: [{ text: "/c", match: true }, { text: "lear", match: false }] },
  { name: "/compact", hint: "[instructions]", description: "Summarize the conversation", source: "built-in", segments: [{ text: "/c", match: true }, { text: "ompact", match: false }] },
];

const base = { rows, selected: 0, status: "ready" as const, typed: "/c", onPick: () => {} };

describe("CommandMenu", () => {
  it("lists rows as options and marks the selected one", () => {
    const { getAllByRole, getByRole } = renderThemed(<CommandMenu {...base} selected={1} />);
    expect(getByRole("listbox")).toBeTruthy();
    const opts = getAllByRole("option");
    expect(opts.map((o) => o.getAttribute("aria-selected"))).toEqual(["false", "true"]);
  });

  it("highlights the matched segment and shows the hint", () => {
    const { getAllByRole } = renderThemed(<CommandMenu {...base} />);
    const mark = getAllByRole("option")[0].querySelector("mark");
    expect(mark?.textContent).toBe("/c");
    expect(getAllByRole("option")[1].textContent).toContain("[instructions]");
  });

  it("calls onPick with the clicked row", () => {
    const onPick = vi.fn();
    const { getAllByRole } = renderThemed(<CommandMenu {...base} onPick={onPick} />);
    fireEvent.click(getAllByRole("option")[1]);
    expect(onPick).toHaveBeenCalledWith(1);
  });

  it("does not take focus on mousedown", () => {
    const { getAllByRole } = renderThemed(<CommandMenu {...base} />);
    expect(fireEvent.mouseDown(getAllByRole("option")[0])).toBe(false);
  });

  it("shows the key footer on desktop and hides it when compact", () => {
    const wide = renderThemed(<CommandMenu {...base} />);
    expect(wide.getByText("Enter fills")).toBeTruthy();
    expect(wide.getByText("Cmd/Ctrl+Enter sends")).toBeTruthy();
    wide.unmount();
    const phone = renderThemed(<CommandMenu {...base} compact />);
    expect(phone.queryByText("Enter fills")).toBeNull();
  });

  it("shows the no-match line with the typed token", () => {
    const { getByText, queryByRole } = renderThemed(<CommandMenu {...base} rows={[]} status="no-match" typed="/zz" />);
    expect(getByText("No command matches /zz. Cmd/Ctrl+Enter sends it as typed.")).toBeTruthy();
    expect(queryByRole("listbox")).toBeNull();
  });

  it("shortens the no-match line on a phone", () => {
    const { getByText } = renderThemed(<CommandMenu {...base} rows={[]} status="no-match" typed="/zz" compact />);
    expect(getByText("Send sends it as typed.")).toBeTruthy();
  });

  it("shows one loading line", () => {
    const { getByText, queryByRole } = renderThemed(<CommandMenu {...base} rows={[]} status="loading" />);
    expect(getByText("Loading commands")).toBeTruthy();
    expect(queryByRole("listbox")).toBeNull();
  });

  it("shows built-ins plus a warning when the list failed", () => {
    const { getByText, getAllByRole } = renderThemed(<CommandMenu {...base} status="failed" />);
    expect(getByText("Skills could not load")).toBeTruthy();
    expect(getAllByRole("option")).toHaveLength(2);
  });

  it("places the menu above or below the field", () => {
    const above = renderThemed(<CommandMenu {...base} />);
    expect(above.container.querySelector("[data-placement]")?.getAttribute("data-placement")).toBe("above");
    above.unmount();
    const below = renderThemed(<CommandMenu {...base} placement="below" />);
    expect(below.container.querySelector("[data-placement]")?.getAttribute("data-placement")).toBe("below");
  });

  it.each(TONES)("carries tone %s", (tone) => {
    const { container } = renderThemed(<CommandMenu {...base} tone={tone} />);
    expect(container.querySelector("[data-tone]")?.getAttribute("data-tone")).toBe(tone);
  });
});
