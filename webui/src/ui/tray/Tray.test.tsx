import { describe, expect, it } from "vitest";
import type { Health, HealthState, MeterHealth } from "../../generated/server";
import { renderThemed } from "../render";
import { TopBar } from "../top-bar/TopBar";
import { Tray } from "./Tray";

const meter = (state: HealthState, level = 1): MeterHealth => ({ level, state, read_at: "" });
const healthOf = (cpu: MeterHealth, memory = meter("normal"), disk = meter("normal")): Health => ({ cpu, memory, disk });

describe("Tray", () => {
  it("draws the meters in the order CPU, memory, disk with C, M, D under them", () => {
    const { container } = renderThemed(<Tray health={healthOf(meter("normal"))} />);
    const meters = [...container.querySelectorAll("[data-meter]")];
    expect(meters.map((m) => m.getAttribute("data-meter"))).toEqual(["C", "M", "D"]);
    expect(meters.map((m) => m.textContent)).toEqual(["C", "M", "D"]);
  });

  it("gives each state its own state and fill", () => {
    const { container } = renderThemed(<Tray health={healthOf(meter("normal"), meter("high"), meter("critical"))} />);
    const [cpu, memory, disk] = [...container.querySelectorAll("[data-meter]")];
    expect([cpu, memory, disk].map((m) => m!.getAttribute("data-state"))).toEqual(["normal", "high", "critical"]);
    // jsdom does not resolve Chakra's color tokens, so compare the generated color classes.
    const colorClass = (m: Element) => m.querySelector("[data-part=fill]")!.className;
    expect(colorClass(memory!)).not.toBe(colorClass(cpu!));
    expect(colorClass(disk!)).not.toBe(colorClass(memory!));
    expect(colorClass(disk!)).not.toBe(colorClass(cpu!));
  });

  it("fills the bar at the top level and still shows a fill at level 0", () => {
    const { container } = renderThemed(<Tray health={healthOf(meter("normal", 0), meter("normal", 4))} />);
    const [cpu, memory] = [...container.querySelectorAll("[data-part=fill]")] as HTMLElement[];
    expect(getComputedStyle(memory!).height).toBe("100%");
    expect(cpu).toBeTruthy();
    expect(getComputedStyle(cpu!).height).not.toBe("0%");
  });

  // Replaces e2e flow 79 (Tray states): the states the fixture walked through, as meter data.
  it.each([
    { name: "normal", cpu: meter("normal", 0), mem: meter("normal", 0), cpuFill: "20%", memFill: "20%" },
    { name: "high", cpu: meter("high", 3), mem: meter("high", 4), cpuFill: "80%", memFill: "100%" },
    { name: "critical", cpu: meter("critical", 4), mem: meter("critical", 4), cpuFill: "100%", memFill: "100%" },
    { name: "unknown", cpu: meter("unknown", 0), mem: meter("unknown", 0), cpuFill: null, memFill: null },
  ])("shows the $name meter state and fill height", ({ name, cpu, mem, cpuFill, memFill }) => {
    const { container } = renderThemed(<Tray health={healthOf(cpu, mem)} />);
    for (const [letter, fill] of [["C", cpuFill], ["M", memFill]] as const) {
      const m = container.querySelector(`[data-meter=${letter}]`)!;
      expect(m.getAttribute("data-state")).toBe(name);
      const bar = m.querySelector("[data-part=fill]");
      if (fill === null) expect(bar).toBeNull();
      else expect(getComputedStyle(bar!).height).toBe(fill);
    }
    expect(container.querySelector("[data-meter=D]")!.getAttribute("data-state")).toBe("normal");
  });

  it("draws unknown as an outline with no fill", () => {
    const { container } = renderThemed(<Tray health={healthOf(meter("unknown", 0))} />);
    const [cpu, memory] = [...container.querySelectorAll("[data-meter]")];
    expect(cpu!.querySelector("[data-part=fill]")).toBeNull();
    expect(memory!.querySelector("[data-part=fill]")).not.toBeNull();
  });

  it("names each meter's state in the aria-label, with no percent", () => {
    const { getByRole } = renderThemed(<Tray health={healthOf(meter("high"), meter("normal"), meter("unknown"))} />);
    expect(getByRole("button", { name: "System health: CPU high; memory normal; disk unknown" })).toBeTruthy();
  });
});

describe("TopBar Tray", () => {
  it("sits after the Paused chip and the Offline dot and before the menu", () => {
    const { container } = renderThemed(<TopBar mode="paused" connected={false} build="v1" health={healthOf(meter("normal"))} />);
    const right = container.querySelector("nav > div:last-child")!;
    const labels = [...right.children].map((c) => c.getAttribute("aria-label") ?? c.textContent);
    expect(labels).toEqual(["Paused", "Offline", "System health: CPU normal; memory normal; disk normal", "Menu"]);
  });

  it("renders without an AFK toggle or build", () => {
    const { getByRole } = renderThemed(<TopBar connected health={healthOf(meter("normal"))} />);
    expect(getByRole("button", { name: /^System health/ })).toBeTruthy();
  });
});
