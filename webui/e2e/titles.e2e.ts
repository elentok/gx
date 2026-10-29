import { expect, nav, test } from "./fixture";

test("83. Tab titles, part A", async ({ page, start }) => {
  await start("mixed");
  await expect(page).toHaveTitle("Epics · gx");

  await page.getByRole("navigation").getByRole("link", nav("Stats")).click();
  await expect(page).toHaveTitle("Stats · gx");
  await page.getByLabel("Range").selectOption("7");
  await expect(page).toHaveTitle("Stats · gx");
  await page.getByLabel("Project").selectOption("demo");
  await expect(page).toHaveTitle("Stats · gx");

  await page.getByRole("navigation").getByRole("link", nav("Epics")).click();
  await page.getByRole("link", { name: "mixed-epic", exact: true }).first().click();
  await expect(page).toHaveTitle("mixed-epic · Epic · gx");

  await page.locator('a[href$="#/t/demo:mixed-epic/02"]').first().click();
  await expect(page).toHaveURL(/#\/t\/demo:mixed-epic\/02$/);
  await expect(page.getByRole("region", { name: "Description" })).toBeVisible();
  const heading = await page.getByRole("heading", { level: 1 }).first().innerText();
  await expect(page).toHaveTitle(`${heading} · Ticket · gx`);
  await page.goBack();
  await expect(page).toHaveTitle("mixed-epic · Epic · gx");

  await page.evaluate(() => (window.location.hash = "#/t/demo:mixed-epic/99"));
  await expect(page.getByText("Ticket not found")).toBeVisible();
  await expect(page).toHaveTitle("demo:mixed-epic/99 · Ticket · gx");

  await page.evaluate(() => (window.location.hash = "#/nowhere/at/all"));
  await expect(page).toHaveURL(/#\/epics$/);
  await expect(page).toHaveTitle("Epics · gx");
});

test("83. Tab titles, part B", async ({ page, start }) => {
  await start("design");
  for (const [hash, title] of [
    ["#/d/demo:design-epic", "design-epic · Design · gx"],
    ["#/e/demo:design-epic-impl", "design-epic-impl · Epic · gx"],
    ["#/e/demo:design-epic", "design-epic · Epic · gx"],
  ]) {
    await page.evaluate((h) => (window.location.hash = h), hash);
    await expect(page).toHaveTitle(title);
  }
});
