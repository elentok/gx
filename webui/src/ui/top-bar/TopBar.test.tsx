import { fireEvent } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { renderThemed } from "../render";
import { TopBar } from "./TopBar";

describe("TopBar", () => {
  it("has only the Epics and Stats tabs and marks the active one", () => {
    const { getAllByRole, getByRole } = renderThemed(<TopBar active="stats" connected />);
    expect(getAllByRole("link").map((l) => l.textContent)).toEqual(["Epics", "Stats"]);
    expect(getByRole("link", { name: "Epics" }).getAttribute("href")).toBe("#/epics");
    expect(getByRole("link", { name: "Stats" }).getAttribute("href")).toBe("#/stats");
    expect(getByRole("link", { name: "Stats" }).getAttribute("aria-current")).toBe("page");
    expect(getByRole("link", { name: "Epics" }).hasAttribute("aria-current")).toBe(false);
  });

  it("shows the AFK toggle only with a handler, pressed while AFK is on, and calls it on click", () => {
    const none = renderThemed(<TopBar connected />);
    expect(none.queryByRole("button", { name: "AFK" })).toBeNull();
    none.unmount();
    const onToggleAfk = vi.fn();
    const off = renderThemed(<TopBar active="epics" connected onToggleAfk={onToggleAfk} />);
    expect(off.getByRole("button", { name: "AFK" }).getAttribute("aria-pressed")).toBe("false");
    fireEvent.click(off.getByRole("button", { name: "AFK" }));
    expect(onToggleAfk).toHaveBeenCalledOnce();
    off.unmount();
    const on = renderThemed(<TopBar active="epics" connected afk onToggleAfk={onToggleAfk} />);
    expect(on.getByRole("button", { name: "AFK" }).getAttribute("aria-pressed")).toBe("true");
  });

  it("marks no tab without an active one", () => {
    const { container } = renderThemed(<TopBar connected />);
    expect(container.querySelector("[aria-current]")).toBeNull();
  });

  it("shows the build, the Paused chip and the Live or Offline dot", () => {
    const first = renderThemed(<TopBar active="epics" build="v1.2.3" mode="paused" connected />);
    expect(first.getByText("v1.2.3")).toBeTruthy();
    expect(first.getByText("Paused")).toBeTruthy();
    expect(first.getByRole("status", { name: "Live" })).toBeTruthy();
    first.unmount();
    const second = renderThemed(<TopBar active="epics" mode="running" connected={false} />);
    expect(second.queryByText("Paused")).toBeNull();
    expect(second.getByRole("status", { name: "Offline" })).toBeTruthy();
  });
});
