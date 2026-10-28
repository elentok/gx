import type { Page } from "@playwright/test";
import { card, expect, holdRoute, nav, test } from "./fixture";

const DISK = "fixture disk";

/** The disk strip; other live regions share its role, so it is found by the disk name. */
const strip = (page: Page) => page.getByRole("status").filter({ hasText: DISK });

test("76. Disk banner and Clean now", async ({ page, start }) => {
  const fixture = await start("disk-low");
  await fixture.setDiskFree("15G");
  await expect(strip(page)).toContainText(`${DISK} is low`);
  await expect(strip(page).getByRole("button", nav("Clean now"))).toBeEnabled();

  await page.getByRole("link", { name: "disk-epic", exact: true }).click();
  await expect(strip(page)).toBeVisible();
  await page.locator('a[href$="/t/demo:disk-epic/01"]').click();
  await expect(strip(page)).toBeVisible();
  await page.evaluate(() => (window.location.hash = "#/epics"));
  await expect(strip(page)).toBeVisible();

  const release = await holdRoute(page, "**/v1/disk/clean");
  await strip(page).getByRole("button", nav("Clean now")).click();
  await expect(strip(page).getByRole("button", nav("Clean now"))).toBeDisabled();
  release();

  const before = await strip(page).textContent();
  await fixture.setDiskFree("21G");
  await expect.poll(() => strip(page).textContent()).not.toBe(before);
  await expect(strip(page)).toContainText(`${DISK} is low`);

  await fixture.setDiskFree("23G");
  await expect(strip(page)).toHaveCount(0);
});

test("77. Disk hold", async ({ page, start }) => {
  const fixture = await start("disk-low", { hold: true });
  await fixture.setDiskClean("6G");
  await fixture.setDiskFree("5G");
  await expect(strip(page)).toContainText("new tickets held");
  // The auto-clean frees 1G and sets free space to 6G; a free-space write before it lands would be overwritten.
  await expect(strip(page)).toContainText("6.0 GB free");

  await card(page, "disk-epic").getByRole("button", nav("Queue")).click();
  await card(page, "disk-epic").getByRole("link", { name: "disk-epic", exact: true }).click();
  const row = page.locator('a[href$="/t/demo:disk-epic/01"]');
  await expect(row.locator("[data-verdict]")).toContainText("disk low");
  await expect(row.locator("[data-meta]")).not.toHaveText(/^running · /);
  const explain = await fixture.cli("server", "tickets", "explain", "demo:disk-epic/01");
  expect(explain).toContain("disk low");
  expect(explain).toContain(DISK);

  await fixture.setDiskFree("11G");
  await expect(strip(page)).toContainText(`${DISK} is low`);
  await expect(strip(page)).not.toContainText("new tickets held");
  await expect(row.locator("[data-meta]")).toHaveText(/^running · /);
});

test("78. Clean now fails", async ({ page, start }) => {
  const fixture = await start("disk-low");
  await fixture.setDiskClean("fail");
  await fixture.setDiskFree("15G");
  await expect(strip(page)).toContainText(`${DISK} is low`);
  await strip(page).getByRole("button", nav("Clean now")).click();
  await expect(strip(page).getByRole("alert")).toContainText(/^failed: .*clean\.sh/);
  await expect(strip(page)).toBeVisible();
});
