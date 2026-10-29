import { expect, nav, test } from "./fixture";

test("83. Tab titles", async ({ page, start }) => {
  await start("mixed");
  await expect(page).toHaveTitle("Epics · gx");

  await page.getByRole("navigation").getByRole("link", nav("Stats")).click();
  await expect(page).toHaveTitle("Stats · gx");

  await page.getByRole("navigation").getByRole("link", nav("Epics")).click();
  await page.getByRole("link", { name: "mixed-epic", exact: true }).first().click();
  await expect(page).toHaveTitle("mixed-epic · Epic · gx");

  await page.locator('a[href$="#/t/demo:mixed-epic/02"]').first().click();
  await expect(page.getByRole("region", { name: "Description" })).toBeVisible();
  const heading = await page.getByRole("heading", { level: 1 }).first().innerText();
  await expect(page).toHaveTitle(`${heading} · Ticket · gx`);

  await page.evaluate(() => (window.location.hash = "#/d/demo:mixed-epic"));
  await expect(page).toHaveTitle("mixed-epic · Design · gx");
});
