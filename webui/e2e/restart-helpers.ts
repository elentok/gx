import type { Page } from "@playwright/test";
import { expect, nav, type Fixture } from "./fixture";

// Shared by the restart flows (restart.e2e.ts, restart-run.e2e.ts). They run on the restart-*
// scenarios, whose server path is a fake gx binary that a flow steers through its control files
// (check.json, slow, old, start, gx).

export const menuButton = (page: Page) => page.getByRole("button", nav("Menu"));
export const menu = (page: Page) => page.getByRole("menu", nav("Menu"));
export const restartItem = (page: Page) => menu(page).getByRole("menuitem", nav("Restart server"));
export const dialog = (page: Page) => page.getByRole("dialog", nav("Restart server"));
export const restartButton = (page: Page) => dialog(page).getByRole("button", { name: /^Restart( anyway)?$/ });

export const openDialog = async (page: Page) => {
  await menuButton(page).click();
  await restartItem(page).click();
  await expect(dialog(page)).toBeVisible();
};

/** The build text in the menu, e.g. what the dialog calls the running version. */
export const buildText = async (page: Page) => {
  await menuButton(page).click();
  const build = menu(page).getByTestId("server-build");
  await expect(build).toBeVisible();
  const text = (await build.textContent())!.trim();
  await page.keyboard.press("Escape");
  await expect(menu(page)).toHaveCount(0);
  return text;
};

/** What `restart-check` prints when every project passes, on restart-projects (projects demo and broken). */
export const passing = (version: string, config: { ok: boolean; error?: string } = { ok: true }) =>
  JSON.stringify({
    version,
    config,
    projects: [
      { name: "demo", ok: true, files: [] },
      { name: "broken", ok: true, files: [] },
    ],
  });

/** The pid line of `gx server status` (it has no start time): it changes on every restart. */
export const pidLine = async (fixture: Fixture) => {
  const status = await fixture.cli("server", "status");
  const line = status.split("\n").find((l) => l.startsWith("pid:"));
  expect(line, `no pid in:\n${status}`).toBeTruthy();
  return line!;
};

/** Waits for the dialog to go away on its own (the tab reloaded onto the new server). */
export const expectReloaded = async (page: Page) => {
  await expect(page.getByText("Restarting...")).toBeVisible();
  await expect(dialog(page)).toHaveCount(0, { timeout: 45_000 });
  await expect(menuButton(page)).toBeVisible();
};
