import { fireEvent, waitFor } from "@testing-library/react";
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

  it("has no menu button without a toggle or a build", () => {
    const { queryByRole } = renderThemed(<TopBar connected />);
    expect(queryByRole("button", { name: "Menu" })).toBeNull();
  });

  it("holds the AFK item in the menu, checked while AFK is on, and keeps the menu open on toggle", async () => {
    const onToggleAfk = vi.fn();
    const off = renderThemed(<TopBar active="epics" connected onToggleAfk={onToggleAfk} />);
    expect(off.queryByRole("button", { name: "AFK" })).toBeNull();
    fireEvent.click(off.getByRole("button", { name: "Menu" }));
    const item = await off.findByRole("menuitemcheckbox", { name: "AFK" });
    expect(item.getAttribute("aria-checked")).toBe("false");
    fireEvent.click(item);
    await waitFor(() => expect(onToggleAfk).toHaveBeenCalledOnce());
    expect(off.getByRole("menuitemcheckbox", { name: "AFK" })).toBeTruthy();
    off.unmount();
    const on = renderThemed(<TopBar active="epics" connected afk onToggleAfk={onToggleAfk} />);
    fireEvent.click(on.getByRole("button", { name: "Menu" }));
    expect((await on.findByRole("menuitemcheckbox", { name: "AFK" })).getAttribute("aria-checked")).toBe("true");
  });

  it("shows the build as plain text in the menu, not in the bar", async () => {
    const { getByRole, findByTestId, queryByTestId } = renderThemed(<TopBar active="epics" build="v1.2.3" connected />);
    expect(queryByTestId("server-build")?.closest("[role=menu]") ?? null).toBeNull();
    fireEvent.click(getByRole("button", { name: "Menu" }));
    const build = await findByTestId("server-build");
    expect(build.textContent).toBe("v1.2.3");
    expect(build.closest("[role=menu]")).not.toBeNull();
    expect(build.getAttribute("role")).toBeNull();
  });

  it("marks no tab without an active one", () => {
    const { container } = renderThemed(<TopBar connected />);
    expect(container.querySelector("[aria-current]")).toBeNull();
  });

  it("shows the Paused chip and the Live or Offline dot, and no AFK", () => {
    const first = renderThemed(<TopBar active="epics" build="v1.2.3" mode="paused" connected afk onToggleAfk={() => {}} />);
    expect(first.queryByText("AFK", { ignore: "[hidden] *" })).toBeNull();
    expect(first.getByText("Paused")).toBeTruthy();
    expect(first.getByRole("status", { name: "Live" })).toBeTruthy();
    first.unmount();
    const second = renderThemed(<TopBar active="epics" mode="running" connected={false} />);
    expect(second.queryByText("Paused")).toBeNull();
    expect(second.getByRole("status", { name: "Offline" })).toBeTruthy();
  });
});
