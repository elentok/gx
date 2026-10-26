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

test.describe("phone", () => {
  test.use({ viewport: { width: 375, height: 812 } });

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
