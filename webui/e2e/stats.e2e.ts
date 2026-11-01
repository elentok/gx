import type { Locator, Page } from "@playwright/test";
import { expect, nav, test } from "./fixture";

// Seeded totals are in the grilling-stats comment in testutil/gxfixture/scenarios.go.
const grilling = (page: import("@playwright/test").Page) => page.getByRole("region", nav("Grilling"));
const statCard = (page: import("@playwright/test").Page, title: string) =>
  grilling(page).locator("[data-tone]").filter({ hasText: title });

test("57. Stats, Grilling section", async ({ page, start }) => {
  await start("grilling-stats");
  await page.evaluate(() => (location.hash = "#/stats"));
  const words = statCard(page, "Words read");
  const questions = statCard(page, "Questions asked");

  // The section sits after Cost and effort and before Friction.
  const headings = await page.getByRole("region").evaluateAll((els) => els.map((el) => document.getElementById(el.getAttribute("aria-labelledby") ?? "")?.textContent));
  expect(headings.indexOf("Grilling")).toBe(headings.indexOf("Cost and effort") + 1);
  expect(headings.indexOf("Friction")).toBe(headings.indexOf("Grilling") + 1);

  // 30 days: headline is the total; small text is total over the 26 non-hatched days.
  await expect(words).toContainText("550");
  await expect(words).toContainText("21/day");
  await expect(questions).toContainText("12");
  await expect(questions).toContainText("0/day");

  // The 4 days before the earliest count are hatched; the empty day after it is a zero bar.
  const hatched = words.locator("[data-nodata]");
  await expect(hatched).toHaveCount(4);
  await expect(words.locator("[data-bar]")).toHaveCount(3);
  await expect(words.locator('[data-nodata="4"], [data-bar="4"]')).toHaveCount(1);
  await expect(words.locator('[data-nodata="5"], [data-bar="5"]')).toHaveCount(0);

  // 7 days: no hatched days, so the small text is over all 7.
  await page.getByLabel("Range").selectOption("7");
  await expect(words).toContainText("260");
  await expect(words).toContainText("37/day");
  await expect(questions).toContainText("6");
  await expect(words.locator("[data-nodata]")).toHaveCount(0);

  // One project: only its counts; hatched days stay where they were.
  await page.getByLabel("Range").selectOption("30");
  await expect(words).toContainText("550");
  await page.getByLabel("Project").selectOption("demo");
  await expect(words).toContainText("310");
  await expect(words).not.toContainText("550");
  await expect(questions).toContainText("7");
  await expect(hatched).toHaveCount(4);
  await expect(words.locator('[data-nodata="0"]')).toHaveCount(1);
  await expect(words.locator('[data-nodata="3"]')).toHaveCount(1);
});

// The limit-burn `stats` shape is relative to today: day 0 of a 30-day range is 29 days ago, history starts
// at day 7, the weekly windows reset at days 12, 19 and 26, the 5-hour windows at 15 and 29. The ledger has
// stamp-only days 0-8 (day 4 an unstamped ticket), rebuilt days 9-18, live days 19-29 (day 24 quiet).
const costCard = (page: Page, title: RegExp) =>
  page.getByRole("region", nav("Cost and effort")).locator("[data-tone]").filter({ hasText: title });
const LIMIT_CARDS = [/^Claude weekly burn/, /^Codex weekly burn/, /^Claude 5-hour burn/, /^Codex 5-hour burn/];

async function tip(card: Locator, day: number): Promise<string> {
  await card.locator(`[data-hit="${day}"]`).hover({ force: true });
  return card.getByRole("tooltip").innerText();
}

async function openStats(page: Page, start: (scenario: string, opts?: { shape?: string }) => Promise<unknown>) {
  await start("limit-burn", { shape: "stats" });
  await page.evaluate(() => (location.hash = "#/stats"));
  await costCard(page, LIMIT_CARDS[0]).waitFor();
}

test("182. Stats limit burn", async ({ page, start }) => {
  await openStats(page, start);
  const [claudeWeekly, codexWeekly, claude5h, codex5h] = LIMIT_CARDS.map((t) => costCard(page, t));

  // Four cards in order, with the headline, the partial badge and where history starts.
  await expect(claudeWeekly).toContainText("61 pts");
  await expect(claudeWeekly).toContainText("partial");
  await expect(claudeWeekly).toContainText(/starts \w+ \d+/);
  await expect(codexWeekly).toContainText("59 pts");
  await expect(claude5h).toContainText("62 pts");
  await expect(codex5h).toContainText("60 pts");

  // Weekly cards show the pace and the forecast; 5-hour cards show neither.
  await expect(claudeWeekly).toContainText("7-day average: 3 points a day");
  await expect(claudeWeekly).toContainText("~23% at reset");
  await expect(codexWeekly).toContainText("~22% at reset");
  for (const card of [claude5h, codex5h]) {
    await expect(card).toContainText("Reset days are marked ↺ in the tooltip.");
    await expect(card).not.toContainText("at reset");
    await expect(card).not.toContainText("7-day average");
  }

  // The 7 days before history are hatched and the tooltip says so; no project tag without a filter.
  for (const card of [claudeWeekly, codexWeekly, claude5h, codex5h]) {
    await expect(card.locator("[data-nodata]")).toHaveCount(7);
    await expect(card).not.toContainText("All projects");
  }
  expect(await tip(claudeWeekly, 0)).toContain("no data");

  // A reset day carries "↺" in the tooltip; an ordinary day does not.
  expect(await tip(claude5h, 15)).toContain("↺ window reset");
  expect(await tip(claude5h, 9)).not.toContain("↺");
  expect(await tip(claudeWeekly, 12)).toContain("↺ window reset");

  // One project: the same numbers, plus the tag and the line.
  await page.getByLabel("Project").selectOption("demo");
  for (const [card, pts] of [[claudeWeekly, "61 pts"], [codexWeekly, "59 pts"], [claude5h, "62 pts"], [codex5h, "60 pts"]] as const) {
    await expect(card).toContainText(pts);
    await expect(card).toContainText("All projects");
    await expect(card).toContainText("The limit counts all use on your account");
  }
  await expect(claudeWeekly).toContainText("~23% at reset");

  // 7 days: history covers the whole range, so no hatched days and no "partial".
  await page.getByLabel("Project").selectOption("");
  await page.getByLabel("Range").selectOption("7");
  await expect(claudeWeekly).toContainText("19 pts");
  await expect(claudeWeekly.locator("[data-nodata]")).toHaveCount(0);
  await expect(claudeWeekly).not.toContainText("partial");

  // 90 days: the same burn, with the head of the range hatched.
  await page.getByLabel("Range").selectOption("90");
  await expect(claudeWeekly).toContainText("61 pts");
  await expect(claudeWeekly.locator("[data-nodata]")).toHaveCount(67);
});

test("183. Stats tokens and spend by group", async ({ page, start }) => {
  await openStats(page, start);
  const tokens = costCard(page, /^Tokens/);
  const spend = costCard(page, /^Spend \(est\.\)/);
  await expect(tokens).toBeVisible();

  // Four groups stack in the legend order.
  for (const card of [tokens, spend]) {
    const legend = (await card.getByLabel("Legend").innerText()).split(/\s+/).filter(Boolean);
    expect(legend).toEqual(["Queue", "Design", "One-off", "Other"]);
  }
  await expect(tokens).toContainText("partial");
  await expect(tokens).toContainText("older days from ticket stamps (lighter)");
  await expect(spend).toContainText("Codex counts $0");

  // A live day: all four groups and the harness split.
  const live = await tip(tokens, 29);
  for (const g of ["Queue", "Design", "One-off", "Other"]) expect(live).toContain(`${g}:`);
  expect(live).toMatch(/Claude: \S+/);
  expect(live).toMatch(/Codex: \S+/);
  expect(live).not.toContain("rebuilt");
  await expect(tokens.locator('[data-bar="29"]').first()).not.toHaveAttribute("data-style", /.+/);

  // Rebuilt and stamp-only days are lighter and say why.
  expect(await tip(tokens, 12)).toContain("rebuilt from transcripts");
  await expect(tokens.locator('[data-bar="12"]').first()).toHaveAttribute("data-style", "dim");
  expect(await tip(tokens, 3)).toContain("ticket stamps only");
  await expect(tokens.locator('[data-bar="3"]').first()).toHaveAttribute("data-style", "dim");

  // A finished ticket without a stamp is a hatched no-data day; a quiet live day is a zero bar, not hatched.
  await expect(tokens.locator("[data-nodata]")).toHaveCount(1);
  await expect(tokens.locator('[data-nodata="4"]')).toHaveCount(1);
  expect(await tip(tokens, 4)).toContain("no data");
  await expect(tokens.locator('[data-bar="24"], [data-nodata="24"]')).toHaveCount(0);
  const quiet = await tip(tokens, 24);
  expect(quiet).toContain("Queue: 0");
  expect(quiet).not.toContain("no data");

  // Codex adds tokens but $0.
  expect(await tip(tokens, 12)).toMatch(/Codex: [1-9]\S*K/);
  expect(await tip(spend, 12)).toContain("Codex: $0");

  // One project: the "no project" rows drop out and the totals and bars change.
  const allTotal = await tokens.innerText();
  const allBars = await tokens.locator("[data-bar]").count();
  const allSpend = await spend.innerText();
  await page.getByLabel("Project").selectOption("demo");
  await expect(tokens).toContainText("4M");
  await expect(tokens).not.toContainText("6M");
  expect(await tokens.innerText()).not.toBe(allTotal);
  expect(await tokens.locator("[data-bar]").count()).toBeLessThan(allBars);
  expect(await spend.innerText()).not.toBe(allSpend);
});

test.describe("phone", () => {
  test.use({ viewport: { width: 375, height: 812 } });

  test("182. Stats limit burn, phone", async ({ page, start }) => {
    await openStats(page, start);
    const boxes = [];
    for (const t of LIMIT_CARDS) boxes.push((await costCard(page, t).boundingBox())!);
    for (const b of boxes.slice(1)) expect(b.x).toBe(boxes[0].x);
    for (let i = 1; i < boxes.length; i++) expect(boxes[i].y).toBeGreaterThan(boxes[i - 1].y);
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
    expect(overflow).toBeLessThanOrEqual(0);
  });

  test("57. Stats, Grilling section, phone", async ({ page, start }) => {
    await start("grilling-stats");
    await page.evaluate(() => (location.hash = "#/stats"));
    const first = (await statCard(page, "Words read").boundingBox())!;
    const second = (await statCard(page, "Questions asked").boundingBox())!;
    expect(second.x).toBe(first.x);
    expect(second.y).toBeGreaterThan(first.y);
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
    expect(overflow).toBeLessThanOrEqual(0);
  });
});
