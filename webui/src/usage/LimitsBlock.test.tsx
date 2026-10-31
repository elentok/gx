import { fireEvent, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { LimitWindow, UsageSummary } from "../generated/server";
import { UsageModeLimits } from "../generated/server";
import { renderThemed } from "../ui/render";
import { exactTime } from "./format";
import { LimitsBlock } from "./LimitsBlock";

const now = Date.UTC(2026, 9, 31, 12, 0, 0);
const sec = now / 1000;
const win = (pct: number, over: Partial<LimitWindow> = {}): LimitWindow => ({
  used_percentage: pct,
  resets_at: sec + 2 * 3600 + 600,
  received_at: sec - 60,
  ...over,
});
const summary = (limits: UsageSummary["limits"]): UsageSummary => ({
  mode: UsageModeLimits,
  limits,
  spend: {} as UsageSummary["spend"],
});
const show = (limits: UsageSummary["limits"]) => renderThemed(<LimitsBlock summary={summary(limits)} now={now} />);

describe("LimitsBlock", () => {
  it("fresh: two rows, weekly amber", () => {
    const v = show({ five_hour: win(34), seven_day: win(81, { resets_at: sec + 2 * 86400 + 4 * 3600 }) });
    const five = v.getByRole("group", { name: "5h limit" });
    const week = v.getByRole("group", { name: "Weekly limit" });
    expect(within(five).getByText("34%")).toBeTruthy();
    expect(within(five).getByText("resets in 2h 10m")).toBeTruthy();
    expect(within(week).getByText("81%").getAttribute("data-tone")).toBe("waiting");
    expect(within(week).getByRole("progressbar").getAttribute("data-tone")).toBe("waiting");
    expect(v.queryByText(/seen/)).toBeNull();
  });

  it.each([
    [79, "running"],
    [80, "waiting"],
    [94, "waiting"],
    [95, "deadlocked"],
    [79.5, "waiting"],
  ])("tones %d as %s on bar and percentage", (pct, tone) => {
    const v = show({ five_hour: win(pct) });
    expect(v.getByRole("progressbar").getAttribute("data-tone")).toBe(tone);
    expect(v.getByText(`${Math.round(pct)}%`).getAttribute("data-tone")).toBe(tone);
  });

  it("shows seen only past 10 minutes", () => {
    expect(show({ five_hour: win(10, { received_at: sec - 600 }) }).queryByText(/seen/)).toBeNull();
    expect(show({ five_hour: win(10, { received_at: sec - 601 }) }).getByText("seen 10m ago")).toBeTruthy();
  });

  it("past reset: waiting, no bar, no seen, no tooltip", () => {
    const v = show({ five_hour: win(50, { resets_at: sec - 5, received_at: sec - 6 * 3600 }), seven_day: win(52) });
    const five = v.getByRole("group", { name: "5h limit" });
    expect(within(five).getByText("waiting")).toBeTruthy();
    expect(within(five).getByText("reset passed · waiting for a Claude session")).toBeTruthy();
    expect(within(five).queryByRole("progressbar")).toBeNull();
    expect(within(five).queryByText(/seen/)).toBeNull();
    expect(within(five).queryByRole("button")).toBeNull();
    expect(within(v.getByRole("group", { name: "Weekly limit" })).getByRole("progressbar")).toBeTruthy();
  });

  it("one window shows one row", () => {
    expect(show({ seven_day: win(12) }).getAllByRole("group").map((g) => g.getAttribute("aria-label"))).toEqual(["Weekly limit"]);
  });

  it("holds the exact reset time in a tooltip that opens on focus", async () => {
    const w = win(34);
    const v = show({ five_hour: w });
    const tip = v.getByRole("button");
    fireEvent.focus(tip);
    expect(await v.findByText(exactTime(w.resets_at))).toBeTruthy();
  });

  it("carries no budget text", () => {
    const v = show({ five_hour: win(34), seven_day: win(81) });
    expect(v.container.textContent).not.toMatch(/budget|\$/i);
  });
});
