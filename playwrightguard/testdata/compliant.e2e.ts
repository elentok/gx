test("evaluate after expect", async ({ page }) => {
  const dialog = page.getByRole("dialog");
  await expect(dialog).toBeVisible();
  expect(await dialog.evaluate((el) => el.scrollWidth <= el.clientWidth)).toBe(true);
});

test("evaluate inside expect.poll", async ({ page }) => {
  const layer = page.getByRole("menu");
  await expect.poll(() => layer.evaluate((el) => el.contains(document.activeElement))).toBe(true);
});

test("page evaluate is a page-level call", async ({ page }) => {
  await page.evaluate(() => (location.hash = "#x"));
});

const fontSize = async (el: Locator) => {
  await expect(el).toBeVisible();
  return el.evaluate((node) => getComputedStyle(node).fontSize);
};
