import { fireEvent, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { renderThemed } from "../render";
import { TopBar } from "./TopBar";

describe("TopBar", () => {
  it("has the Epics, Agents and Stats tabs in that order and marks the active one", () => {
    const { getAllByRole, getByRole } = renderThemed(<TopBar active="stats" connected />);
    expect(getAllByRole("link").map((l) => l.textContent)).toEqual(["Epics", "Agents", "Stats"]);
    expect(getByRole("link", { name: "Agents" }).getAttribute("href")).toBe("#/agents");
    expect(getByRole("link", { name: "Epics" }).getAttribute("href")).toBe("#/epics");
    expect(getByRole("link", { name: "Stats" }).getAttribute("href")).toBe("#/stats");
    expect(getByRole("link", { name: "Stats" }).getAttribute("aria-current")).toBe("page");
    expect(getByRole("link", { name: "Epics" }).hasAttribute("aria-current")).toBe(false);
  });

  it("counts the waiting epics on the Epics tab and leaves out a zero", () => {
    const { getByRole, unmount } = renderThemed(<TopBar connected waiting={3} />);
    expect(getByRole("link", { name: "Epics · 3 need you" }).textContent).toBe("Epics3");
    unmount();
    expect(renderThemed(<TopBar connected waiting={0} />).getByRole("link", { name: "Epics" }).textContent).toBe("Epics");
  });

  it("counts the Actionable sessions on the Agents tab, leaves out a zero and marks it current", () => {
    const { getByRole, unmount } = renderThemed(<TopBar active="agents" connected actionable={2} />);
    expect(getByRole("link", { name: "Agents · 2 need you" }).textContent).toBe("Agents2");
    expect(getByRole("link", { name: "Agents · 2 need you" }).getAttribute("aria-current")).toBe("page");
    unmount();
    expect(renderThemed(<TopBar connected actionable={0} />).getByRole("link", { name: "Agents" }).textContent).toBe("Agents");
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

  it("shows the Paused chip, the Offline dot only while offline, and no AFK", () => {
    const first = renderThemed(<TopBar active="epics" build="v1.2.3" mode="paused" connected afk onToggleAfk={() => {}} />);
    expect(first.queryByText("AFK", { ignore: "[hidden] *" })).toBeNull();
    expect(first.getByText("Paused")).toBeTruthy();
    expect(first.queryByRole("status", { name: "Live" })).toBeNull();
    first.unmount();
    const second = renderThemed(<TopBar active="epics" mode="running" connected={false} />);
    expect(second.queryByText("Paused")).toBeNull();
    expect(second.getByRole("status", { name: "Offline" })).toBeTruthy();
  });
});
