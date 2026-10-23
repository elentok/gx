import { describe, expect, it } from "vitest";
import { renderThemed } from "../render";
import { TopBar } from "./TopBar";

describe("TopBar", () => {
  it("links the tabs and marks only the active one", () => {
    const { getByRole } = renderThemed(<TopBar active="epics" connected />);
    expect(getByRole("link", { name: "Queue" }).getAttribute("href")).toBe("#/q");
    expect(getByRole("link", { name: "Epics" }).getAttribute("href")).toBe("#/epics");
    expect(getByRole("link", { name: "Epics" }).getAttribute("aria-current")).toBe("page");
    expect(getByRole("link", { name: "Queue" }).hasAttribute("aria-current")).toBe(false);
  });

  it("orders the tabs Queue, Epics, Designs, Stats and marks Stats", () => {
    const { getAllByRole, getByRole } = renderThemed(<TopBar active="stats" connected />);
    expect(getAllByRole("link").map((l) => l.textContent)).toEqual(["Queue", "Epics", "Designs", "Stats"]);
    expect(getByRole("link", { name: "Stats" }).getAttribute("href")).toBe("#/stats");
    expect(getByRole("link", { name: "Stats" }).getAttribute("aria-current")).toBe("page");
  });

  it("marks no tab without an active one", () => {
    const { container } = renderThemed(<TopBar connected />);
    expect(container.querySelector("[aria-current]")).toBeNull();
  });

  it("shows the build, the queue chip and the Live or Offline dot", () => {
    const first = renderThemed(<TopBar build="v1.2.3" mode="paused" connected />);
    expect(first.getByText("v1.2.3")).toBeTruthy();
    expect(first.getByText("Paused")).toBeTruthy();
    expect(first.getByRole("status", { name: "Live" })).toBeTruthy();
    first.unmount();
    const second = renderThemed(<TopBar mode="running" connected={false} />);
    expect(second.queryByText("Paused")).toBeNull();
    expect(second.getByRole("status", { name: "Offline" })).toBeTruthy();
  });
});
