import { appendFile, mkdir } from "node:fs/promises";
import { dirname } from "node:path";
import type { Locator, Page } from "@playwright/test";
import { expect, nav, test } from "./fixture";

// Seeded totals are in the grilling-stats comment in testutil/gxfixture/scenarios.go.
const grilling = (page: import("@playwright/test").Page) => page.getByRole("region", nav("Grilling"));
const statCard = (page: import("@playwright/test").Page, title: string) =>
  grilling(page).locator("[data-tone]").filter({ hasText: title });

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
  await page.evaluate(() => (location.hash = "#/stats?range=30"));
  await costCard(page, LIMIT_CARDS[0]).waitFor();
}

// Moved to vitest: 57 (Grilling section, StatsPage.test.tsx), 182 (limit burn, LimitBurnCards.test.tsx and
// the Go shaping tests), 183 (tokens and spend by group, StatsPage.test.tsx). Also covers 182 phone.
test("184. Live token ledger (and 182. Stats limit burn, phone)",async ({ page, start }) => {
  test.setTimeout(120_000);
  const fixture = await start("limit-burn", { shape: "live" });
  const groups = { queue: "Queue", design: "Design", one_off: "One-off", other: "Other" };
  const append = async (tokens: number) => {
    for (const path of Object.values(fixture.transcripts)) {
      await mkdir(dirname(path), { recursive: true });
      const line = { type: "assistant", timestamp: new Date().toISOString(), message: { model: "claude-sonnet-4-5", usage: { input_tokens: tokens, output_tokens: 0 } } };
      await appendFile(path, JSON.stringify(line) + "\n");
    }
  };
  // Today's tokens per group from the CLI, the same numbers the page reads.
  const today = async () => {
    const stats = JSON.parse(await fixture.cli("server", "stats", "--json", "--range", "7"));
    return Object.fromEntries(stats.cost.groups.map((g: { group: string; tokens: { value: number }[] }) => [g.group, g.tokens[g.tokens.length - 1].value])) as Record<string, number>;
  };
  const waitFor = async (want: (t: Record<string, number>) => boolean) => {
    await expect.poll(async () => want(await today()), { timeout: 60_000 }).toBe(true);
    return today();
  };
  expect(Object.keys(fixture.transcripts).sort()).toEqual(Object.keys(groups).sort());

  // The first poll that sees a transcript only stores its total, so a baseline line goes in first.
  await append(1000);
  await new Promise((r) => setTimeout(r, 3000));
  const before = await today();

  await append(250_000);
  const afterFirst = await waitFor((t) => Object.values(groups).every((g) => t[g] - before[g] === 250_000));

  // Navigate by hash: a page reload fails the suite.
  await page.evaluate(() => (location.hash = "#/stats?range=30"));
  const tokens = costCard(page, /^Tokens/);
  await tokens.waitFor();
  const check = async (totals: Record<string, number>) => {
    const tooltip = await tip(tokens, 29);
    for (const g of Object.values(groups)) expect(tooltip).toMatch(new RegExp(`${g}: ${totals[g]}(?!\\d)`));
  };
  await check(afterFirst);

  // Only the delta is added.
  await append(100_000);
  const afterSecond = await waitFor((t) => Object.values(groups).every((g) => t[g] - afterFirst[g] === 100_000));
  await page.evaluate(() => (location.hash = "#/"));
  await page.evaluate(() => (location.hash = "#/stats?range=30"));
  await tokens.waitFor();
  // The pointer still rests on the old bar; move it off so the new bar sees a fresh hover.
  await page.mouse.move(0, 0);
  await check(afterSecond);

  // 182 phone: the four limit cards stack in one column with no horizontal overflow.
  await page.setViewportSize({ width: 375, height: 812 });
  const boxes = [];
  for (const t of LIMIT_CARDS) boxes.push((await costCard(page, t).boundingBox())!);
  for (const b of boxes.slice(1)) expect(b.x).toBe(boxes[0].x);
  for (let i = 1; i < boxes.length; i++) expect(boxes[i].y).toBeGreaterThan(boxes[i - 1].y);
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
  expect(overflow).toBeLessThanOrEqual(0);
});

// Kept as its own flow: the Grilling cards only exist in the grilling-stats scenario, not in 184's.
test.describe("phone", () => {
  test.use({ viewport: { width: 375, height: 812 } });

  test("57.Stats, Grilling section, phone", async ({ page, start }) => {
    await start("grilling-stats");
    await page.evaluate(() => (location.hash = "#/stats?range=30"));
    const first = (await statCard(page, "Words read").boundingBox())!;
    const second = (await statCard(page, "Questions asked").boundingBox())!;
    expect(second.x).toBe(first.x);
    expect(second.y).toBeGreaterThan(first.y);
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
    expect(overflow).toBeLessThanOrEqual(0);
  });
});
