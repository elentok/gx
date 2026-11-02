import { expect, test } from "./fixture";

test.describe("185. Check-lock waiting label", () => {
  test("shows the label and holder, then clears", async ({ page, start }) => {
    const fixture = await start("check-lock");
    await page.evaluate(() => (window.location.hash = "#/t/demo:lock-epic/01"));

    const label = page.locator("[data-waiting-label]");
    await expect(label).toHaveText("waiting for the check lock, held by run-holding-the-lock");
    // A label next to the status, not a pause.
    await expect(page.getByText("claimed", { exact: true })).toBeVisible();
    await expect(page.getByText(/Paused/)).toHaveCount(0);

    await fixture.releaseCheckLock();
    await expect(label).toHaveCount(0);
    await expect(page.getByText("claimed", { exact: true })).toBeVisible();
  });
});
