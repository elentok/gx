import { card, expect, nav, section, test } from "./fixture";

test("25. New Design from an empty page", async ({ page, start }) => {
  await start("empty");
  await expect(page.getByText("No epics yet.")).toBeVisible();
  await expect(page.getByRole("button", nav("New Design"))).toBeVisible();
});

test("26. New Design form", async ({ page, start }) => {
  const fixture = await start("ready");

  // The button sits right of the filter and opens the form in a dialog.
  const filter = await page.getByRole("searchbox", nav("Filter epics")).boundingBox();
  const button = page.getByRole("button", nav("New Design"));
  const at = await button.boundingBox();
  expect(at!.x).toBeGreaterThanOrEqual(filter!.x + filter!.width);
  await button.click();
  const dialog = page.getByRole("dialog", nav("New Design"));
  await dialog.getByRole("textbox", nav("Epic name")).fill("fresh-design");
  await dialog.getByRole("textbox", nav("Rough idea")).fill("A rough idea.");
  await dialog.getByRole("button", nav("Start design")).click();

  await expect(page).toHaveURL(/#\/d\/demo:fresh-design$/);
  await expect.poll(() => fixture.cli("server", "epics")).toContain("fresh-design");

  // Its card, with the session running, opens the same page.
  await page.getByRole("link", nav("← Epics")).click();
  await card(page, "fresh-design").getByRole("link", nav("fresh-design")).click();
  await expect(page).toHaveURL(/#\/d\/demo:fresh-design$/);
});

test("27. Design a not-started Design", async ({ page, start }) => {
  const fixture = await start("united");
  const ready = await section(page, "Ready to design");
  await ready.locator("[data-kind]").filter({ has: page.getByRole("link", nav("design-new")) }).getByRole("button", nav("Design")).click();

  await expect(page).toHaveURL(/#\/d\/demo:design-new$/);
  await expect.poll(() => fixture.cli("agents", "list")).toContain("design-new");
});
