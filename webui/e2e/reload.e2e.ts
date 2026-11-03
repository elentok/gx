import { expect, test } from "./fixture";

// Smoke check of the fixture's reload allowance, not a catalog flow. The fixture fails a test whose
// load count is above `reloads`, so asserting the count here also covers "a second reload fails"
// and "a reload fails by default" (the counter is the only thing they exercised).
const LOADS = "__gxE2eLoads";
const loads = (page: import("@playwright/test").Page) =>
  page.evaluate((key) => Number(sessionStorage.getItem(key)), LOADS);

test("reload allowance: one allowed reload passes and is counted", async ({ page, start }) => {
  await start("empty", { reloads: 1 });
  const before = await loads(page);
  await page.reload();
  expect(await loads(page), "a reload adds exactly one to the count").toBe(before + 1);
});
