// Sample for the fixed-sleep rule. Not part of the web build.
test("sleeps", async ({ page }) => {
  await page.waitForTimeout(500);
  await page
    .locator("x")
    .click(); await page.waitForTimeout(1_000);
});
