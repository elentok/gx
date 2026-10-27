import type { Page } from "@playwright/test";
import { expect, nav, test } from "./fixture";

const banner = (page: Page) => page.getByRole("region", nav("Missing check config"));

test("64. Check config banner", async ({ page, start }) => {
  await start("checks-missing");
  await page.getByRole("combobox", nav("Project")).selectOption("demo");

  const b = banner(page);
  await expect(b).toContainText("demo has no check config.");
  await expect(b).toContainText("Agents run gx's built-in checks until you set it up.");
  await expect(b).toContainText("gx found 1 command to suggest.");
  await expect(b).not.toContainText("auto");
  await expect(b.getByRole("button", nav("Not now"))).toBeVisible();
  await expect(b.getByRole("button", nav("Set up checks"))).toBeVisible();
});

test("65. Check config banner, all projects", async ({ page, start }) => {
  await start("checks-missing");
  await page.getByRole("combobox", nav("Project")).selectOption("");

  const b = banner(page);
  await expect(b).toContainText("2 projects have no check config.");
  await expect(b).toContainText("Agents run gx's built-in checks until you set them up.");
  await expect(b.getByRole("listitem")).toHaveText([/^auto/, /^demo/]);
  await expect(b.getByRole("button", nav("Set up checks for demo"))).toBeVisible();
  await expect(b.getByRole("button", nav("Set up checks for auto"))).toBeVisible();
  await expect(b.getByRole("button", nav("Not now"))).toHaveCount(1);
});

test("66. Not now", async ({ page, start }) => {
  await start("checks-missing");
  const writes: string[] = [];
  page.on("request", (r) => {
    if (r.url().includes("/v1/checks/save")) writes.push(r.url());
  });
  await expect(banner(page)).toBeVisible();

  await banner(page).getByRole("button", nav("Not now")).click();

  await expect(banner(page)).toHaveCount(0);
  await expect(page.getByRole("link", nav("demo-epic"))).toBeVisible();
  expect(writes).toEqual([]);
});
