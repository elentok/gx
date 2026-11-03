test("bare evaluate", async ({ page }) => {
  const dialog = page.getByRole("dialog");
  expect(await dialog.evaluate((el) => el.scrollWidth <= el.clientWidth)).toBe(true);
});

test("expect on another locator", async ({ page }) => {
  const a = page.getByText("a");
  const b = page.getByText("b");
  await expect(a).toBeVisible();
  const rows = await b.evaluateAll((els) => els.length);
});

const fontSize = (el: Locator) => el.evaluate((node) => getComputedStyle(node).fontSize);
