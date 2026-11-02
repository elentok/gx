import { cleanup, fireEvent, screen } from "@testing-library/react";
import { afterEach, expect, test } from "vitest";
import type {
  LimitBurnDay,
  StatsLimitBurn,
  StatsLimitBurns,
} from "../generated/server";
import { renderThemed } from "../ui/render";
import { LimitBurnCards } from "./LimitBurnCards";

afterEach(cleanup);

const dates = Array.from(
  { length: 8 },
  (_, i) => `2026-10-${String(10 + i).padStart(2, "0")}`,
);

const day = (
  date: string,
  burn: number,
  over: Partial<LimitBurnDay> = {},
): LimitBurnDay => ({ date, burn, no_data: false, reset_day: false, ...over });
const burn = (
  days: LimitBurnDay[],
  over: Partial<StatsLimitBurn> = {},
): StatsLimitBurn => {
  const withData = days.filter((d) => !d.no_data);
  const sum = withData.reduce((a, d) => a + d.burn, 0);
  return {
    days,
    sum,
    average: withData.length ? sum / withData.length : 0,
    ...over,
  };
};
const noHistory = burn(dates.map((d) => day(d, 0, { no_data: true })));
const full = burn(dates.map((d, i) => day(d, 9, { reset_day: i === 3 })));

const burns = (over: Partial<StatsLimitBurns> = {}): StatsLimitBurns => ({
  claude_weekly: burn(
    dates.map((d, i) => day(d, 10, { no_data: i < 2 })),
    {
      start_day: "2026-10-12",
      forecast: { status: "projected", pace: 9, at_reset: 72 },
    },
  ),
  codex_weekly: full,
  claude_5h: full,
  codex_5h: noHistory,
  ...over,
});

const card = (title: string) => screen.getByText(title).parentElement!;

const render = (b: StatsLimitBurns, project = "") =>
  renderThemed(<LimitBurnCards days={dates} burns={b} project={project} />);

test("renders four cards with sum headline and daily average", () => {
  render(burns());
  for (const t of [
    "Claude weekly burn",
    "Codex weekly burn",
    "Claude 5-hour burn",
    "Codex 5-hour burn",
  ])
    expect(screen.getByText(t)).toBeTruthy();
  const text = card("Codex weekly burn").textContent;
  expect(text).toContain("72 pts");
  expect(text).toContain("9 pts/day");
});

test("the burn cards have no legend row", () => {
  render(burns());
  expect(card("Claude weekly burn").querySelector("[aria-label=Legend]")).toBeNull();
});

test("weekly cards show the 7-day pace line and the forecast", () => {
  render(burns());
  const text = card("Claude weekly burn").textContent;
  expect(text).toContain("7-day average: 9 points a day");
  expect(text).toContain("~72% at reset");
});

test("a weekly card whose forecast needs data shows no pace line", () => {
  const needs = burn(
    dates.map((d) => day(d, 10)),
    { forecast: { status: "needs_data", pace: 0, at_reset: 0 } },
  );
  render(burns({ claude_weekly: needs }));
  const text = card("Claude weekly burn").textContent;
  expect(text).not.toContain("7-day average");
  expect(text).toContain("Forecast needs 3 days of data");
});

test("5-hour cards show the reset note and no pace or forecast", () => {
  render(burns());
  const text = card("Claude 5-hour burn").textContent;
  expect(text).toContain("Reset days are marked ↺ in the tooltip.");
  expect(text).not.toContain("7-day average");
  expect(text).not.toContain("at reset");
});

test("a range reaching before history is partial and names the start day", () => {
  render(burns());
  const text = card("Claude weekly burn").textContent;
  expect(text).toContain("partial");
  expect(text).toContain("starts Oct 12");
  expect(card("Codex weekly burn").textContent).not.toContain("partial");
});

test("no-data days are hatched; a card with no history shows only hatched days", () => {
  const { container } = render(burns());
  const cardEl = card("Codex 5-hour burn");
  expect(cardEl.querySelectorAll("[data-nodata]").length).toBe(dates.length);
  expect(cardEl.querySelectorAll("[data-bar]").length).toBe(0);
  expect(container.querySelectorAll("[data-nodata]").length).toBe(
    dates.length + 2,
  );
});

test("the tooltip shows date, burn and ↺ on a reset day", () => {
  render(burns());
  const hit = card("Codex weekly burn").querySelector("[data-hit='3']")!;
  fireEvent.mouseEnter(hit);
  const tip = screen.getByRole("tooltip").textContent;
  expect(tip).toContain("2026-10-13");
  expect(tip).toContain("9");
  expect(tip).toContain("↺");
});

test("the All projects tag and account line show only under a project filter", () => {
  const { unmount } = render(burns());
  expect(screen.queryByText("All projects")).toBeNull();
  expect(
    screen.queryByText("The limit counts all use on your account"),
  ).toBeNull();
  unmount();
  render(burns(), "gx");
  expect(screen.getAllByText("All projects").length).toBe(4);
  expect(
    screen.getAllByText("The limit counts all use on your account").length,
  ).toBe(4);
});
