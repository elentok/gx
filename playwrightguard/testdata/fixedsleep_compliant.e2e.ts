// Sample for the fixed-sleep rule. Not part of the web build.
// A comment may name page.waitForTimeout(500) without failing.
test("waits on state", async ({ page }) => {
  await expect(page.locator("x")).toBeVisible();
  await expect.poll(async () => page.locator("x").count()).toBe(1);
});
