import { fireEvent, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type {
  LimitWindow,
  UsageLimitWindow,
  UsageSummary,
} from "../generated/server";
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
const show = (limits: UsageSummary["limits"]) =>
  renderThemed(<LimitsBlock summary={summary(limits)} now={now} />);

describe("LimitsBlock", () => {
  it("fresh: two rows, weekly amber", () => {
    const v = show({
      five_hour: win(34),
      seven_day: win(81, { resets_at: sec + 2 * 86400 + 4 * 3600 }),
    });
    const five = v.getByRole("group", { name: "5h limit" });
    const week = v.getByRole("group", { name: "Weekly limit" });
    expect(within(five).getByText("34%")).toBeTruthy();
    expect(within(five).getByText("resets in 2h 10m")).toBeTruthy();
    expect(within(week).getByText("81%").getAttribute("data-tone")).toBe(
      "waiting",
    );
    expect(
      within(week).getByRole("progressbar").getAttribute("data-tone"),
    ).toBe("waiting");
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
    expect(v.getByText(`${Math.round(pct)}%`).getAttribute("data-tone")).toBe(
      tone,
    );
  });

  it("shows seen only past 10 minutes", () => {
    expect(
      show({ five_hour: win(10, { received_at: sec - 600 }) }).queryByText(
        /seen/,
      ),
    ).toBeNull();
    expect(
      show({ five_hour: win(10, { received_at: sec - 601 }) }).getByText(
        "seen 10m ago",
      ),
    ).toBeTruthy();
  });

  it("past reset: waiting, no bar, no seen, no tooltip", () => {
    const v = show({
      five_hour: win(50, { resets_at: sec - 5, received_at: sec - 6 * 3600 }),
      seven_day: win(52),
    });
    const five = v.getByRole("group", { name: "5h limit" });
    expect(within(five).getByText("waiting")).toBeTruthy();
    expect(
      within(five).getByText("reset passed · waiting for a Claude session"),
    ).toBeTruthy();
    expect(within(five).queryByRole("progressbar")).toBeNull();
    expect(within(five).queryByText(/seen/)).toBeNull();
    expect(within(five).queryByRole("button")).toBeNull();
    expect(
      within(v.getByRole("group", { name: "Weekly limit" })).getByRole(
        "progressbar",
      ),
    ).toBeTruthy();
  });

  it("one window shows one row", () => {
    expect(
      show({ seven_day: win(12) })
        .getAllByRole("group")
        .map((g) => g.getAttribute("aria-label")),
    ).toEqual(["Weekly limit"]);
  });

  it("holds the exact reset time in a tooltip that opens on focus", async () => {
    const w = win(34);
    const v = show({ five_hour: w });
    const tip = v.getByRole("button");
    fireEvent.focus(tip);
    expect(await v.findByText(exactTime(w.resets_at))).toBeTruthy();
  });

  it("opens the reset-time tooltip on click, as a tap does", async () => {
    const w = win(34);
    const v = show({ five_hour: w });
    fireEvent.click(v.getByRole("button"));
    expect(await v.findByText(exactTime(w.resets_at))).toBeTruthy();
  });

  describe("weekly burn", () => {
    const weekly = (over: Partial<UsageLimitWindow>) => ({
      seven_day: { ...win(50, { resets_at: sec + 3 * 86400 }), ...over },
    });

    it.each([
      ["up", 7, 5, "▲ 7 vs 5 pts"],
      ["down", 3, 4, "▼ 3 vs 4 pts"],
      ["equal", 4, 4, "= 4 vs 4 pts"],
    ])("chip %s", (direction, today, yesterday, text) => {
      const v = show(weekly({ chip: { today, yesterday, direction } }));
      expect(
        within(v.getByRole("group", { name: "Weekly limit" })).getByText(text),
      ).toBeTruthy();
      expect(v.getByText("today so far")).toBeTruthy();
    });

    it("no chip when the server sends none", () => {
      const v = show(weekly({}));
      expect(v.queryByText(/ vs /)).toBeNull();
      expect(v.queryByText("today so far")).toBeNull();
    });

    it("chip tooltip opens on focus", async () => {
      const v = show(
        weekly({ chip: { today: 7, yesterday: 5, direction: "up" } }),
      );
      fireEvent.focus(v.getByRole("button", { name: "Weekly limit burn" }));
      expect(
        await v.findByText("Burn today 7 pts so far, yesterday 5"),
      ).toBeTruthy();
    });

    it("chip tooltip opens on click, as a tap does", async () => {
      const v = show(
        weekly({ chip: { today: 7, yesterday: 5, direction: "up" } }),
      );
      fireEvent.click(v.getByRole("button", { name: "Weekly limit burn" }));
      expect(
        await v.findByText("Burn today 7 pts so far, yesterday 5"),
      ).toBeTruthy();
    });

    it.each([
      [{ status: "projected", pace: 10, at_reset: 72 }, "~72% at reset"],
      [
        { status: "needs_data", pace: 0, at_reset: 0 },
        "Forecast needs 3 days of data",
      ],
    ])("forecast %j", (forecast, text) => {
      expect(show(weekly({ forecast })).getByText(text)).toBeTruthy();
    });

    it("forecast says when the limit runs out", () => {
      const runs_out_at = Math.floor(
        new Date(2026, 10, 5, 14, 0).getTime() / 1000,
      );
      const v = show(
        weekly({
          forecast: {
            status: "projected",
            pace: 30,
            at_reset: 130,
            runs_out_at,
          },
        }),
      );
      expect(v.getByText("Runs out ~Thu 14:00, before the reset")).toBeTruthy();
    });

    it("none status and past-reset rows show no forecast", () => {
      const f = { status: "none", pace: 0, at_reset: 0 };
      expect(show(weekly({ forecast: f })).container.textContent).not.toMatch(
        /forecast|at reset/i,
      );
      const p = { status: "projected", pace: 1, at_reset: 60 };
      const past = show(
        weekly({
          forecast: p,
          resets_at: sec - 5,
          chip: { today: 1, yesterday: 1, direction: "equal" },
        }),
      );
      expect(past.container.textContent).not.toMatch(/at reset/);
      expect(past.container.textContent).toMatch(/= 1 vs 1 pts/);
    });

    it("a row at 100% keeps its chip", () => {
      const v = show({
        seven_day: {
          ...win(100, { resets_at: sec + 3 * 86400 }),
          chip: { today: 7, yesterday: 5, direction: "up" },
        },
      });
      expect(v.getByText("▲ 7 vs 5 pts")).toBeTruthy();
    });

    it("stale row keeps the seen marker and the forecast", () => {
      const v = show(
        weekly({
          received_at: sec - 700,
          forecast: { status: "projected", pace: 10, at_reset: 72 },
        }),
      );
      expect(v.getByText("today so far · seen 11m ago")).toBeTruthy();
      expect(v.getByText("~72% at reset")).toBeTruthy();
    });
  });

  describe("Codex block", () => {
    it("absent without Codex data", () => {
      const v = show({ five_hour: win(34) });
      expect(v.queryByText("Codex limits")).toBeNull();
    });

    it("present with 5-hour and weekly rows", () => {
      const v = renderThemed(
        <LimitsBlock
          summary={{
            ...summary({ five_hour: win(34) }),
            codex: { five_hour: win(12), seven_day: win(40) },
          }}
          now={now}
        />,
      );
      expect(v.getByText("Codex limits")).toBeTruthy();
      expect(
        within(v.getByRole("group", { name: "Codex 5h limit" })).getByText(
          "12%",
        ),
      ).toBeTruthy();
      expect(
        within(v.getByRole("group", { name: "Codex Weekly limit" })).getByText(
          "40%",
        ),
      ).toBeTruthy();
      expect(
        within(v.getByRole("group", { name: "5h limit" })).getByText("34%"),
      ).toBeTruthy();
    });
  });

  it("carries no budget text", () => {
    const v = show({ five_hour: win(34), seven_day: win(81) });
    expect(v.container.textContent).not.toMatch(/budget|\$/i);
  });
});
