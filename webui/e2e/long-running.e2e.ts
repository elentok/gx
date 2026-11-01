import { expect, test } from "./fixture";

test("148. Long-running badge", async ({ page, start }) => {
  await start("long-running");
  await page.evaluate(() => (window.location.hash = "#/e/demo:long-epic"));

  const badge = page.getByText("Long-running", { exact: true });
  await expect(badge).toHaveCount(1);
  await expect(badge).toHaveAttribute("title", /^120 assistant turns \(threshold \d+\)$/);

  await page.locator('a[href$="#/t/demo:long-epic/01"]').first().click();
  await expect(page.getByRole("heading", { level: 1 }).first()).toBeVisible();
  await expect(badge).toHaveCount(1);

  await page.evaluate(() => (window.location.hash = "#/t/demo:long-epic/02"));
  await expect(page.getByRole("heading", { level: 1 }).first()).toContainText("plain");
  await expect(badge).toHaveCount(0);
});
