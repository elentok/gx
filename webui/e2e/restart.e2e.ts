import { expect, nav, test } from "./fixture";
import { dialog,expectReloaded, menu, menuButton, openDialog, passing, pidLine, restartButton, restartItem } from "./restart-helpers";

// Also covers 128 (the item sits between AFK and the build line, and opening it closes the menu).
test("129. Restart server keyboard", async ({ page, start }) => {
  await start("restart-projects");
  await menuButton(page).focus();
  await page.keyboard.press("Enter");
  await expect(menu(page)).toBeVisible();
  // The usage block (with its InfoTip) can mount before the menu opens: focus must still land on the menu.
  await expect(menu(page).getByRole("button", nav("About gx spend"))).toBeVisible();
  await expect(menu(page).getByRole("menuitem", nav("Settings"))).toHaveAttribute("data-highlighted", "");
  const afk = await menu(page).getByRole("menuitemcheckbox", nav("AFK")).boundingBox();
  const restart = await restartItem(page).boundingBox();
  const build = await menu(page).getByTestId("server-build").boundingBox();
  expect(restart!.y).toBeGreaterThan(afk!.y);
  expect(build!.y).toBeGreaterThan(restart!.y);
  // Settings, AFK, Vi mode, Restart: Restart is three presses from the highlighted Settings.
  for (let i = 0; i < 3; i++) await page.keyboard.press("ArrowDown");
  await expect(restartItem(page)).toHaveAttribute("data-highlighted", "");
  await page.keyboard.press("Enter");
  await expect(menu(page)).toHaveCount(0);
  await expect(dialog(page)).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(dialog(page)).toHaveCount(0);
});

test("130. Restart dialog confirms", async ({ page, start }) => {
  const fixture = await start("restart-projects", { reloads: 1 });
  await fixture.setRestartControl("check.json", passing("fixture-next"));
  await fixture.setRestartControl("start", "up");
  const before = await pidLine(fixture);

  await openDialog(page);
  await expect(restartButton(page)).toBeEnabled();
  await page.keyboard.press("Escape");
  await expect(dialog(page)).toHaveCount(0);
  expect(await pidLine(fixture)).toBe(before);

  await openDialog(page);
  await restartButton(page).click();
  await expectReloaded(page);
  await expect.poll(() => pidLine(fixture)).not.toBe(before);
});

// 131 (Checking / disabled / versions) and 132 (Versions match) live in RestartDialog.test.tsx; the
// restart and pid change of 132 is flow 130.

// Also covers 135 (bad config) and 136 (old binary) on the same restart-projects server.
test("134. Restart dialog, unreadable version", async ({ page, start }) => {
  // A binary that fails to run: the fake cannot read its check file.
  const failing = await start("restart-projects");
  await failing.clearRestartControl("check.json");
  const before = await pidLine(failing);
  await openDialog(page);
  await expect(dialog(page).getByRole("alert")).toContainText(/check\.json/);
  await expect(restartButton(page)).toBeDisabled();
  expect(await pidLine(failing)).toBe(before);

  // 135: a config the new binary rejects.
  await page.keyboard.press("Escape");
  await expect(dialog(page)).toHaveCount(0);
  await failing.setRestartControl("check.json", passing("fixture-next", { ok: false, error: "fixture config: port is not a number" }));
  await openDialog(page);
  await expect(dialog(page).getByRole("alert")).toContainText("fixture config: port is not a number");
  await expect(restartButton(page)).toBeDisabled();
  expect(await pidLine(failing)).toBe(before);

  // 136: a binary too old to have the check.
  await page.keyboard.press("Escape");
  await expect(dialog(page)).toHaveCount(0);
  await failing.setRestartControl("old", "1");
  await openDialog(page);
  await expect(dialog(page).getByRole("alert")).toContainText("gx server restart");
  await expect(restartButton(page)).toBeDisabled();

  // A binary that is missing.
  const missing = await start("restart-missing");
  const beforeMissing = await pidLine(missing);
  await openDialog(page);
  await expect(dialog(page).getByRole("alert")).toContainText(/no such file/i);
  await expect(restartButton(page)).toBeDisabled();
  expect(await pidLine(missing)).toBe(beforeMissing);
});

test("137. Restart dialog, running agents", async ({ page, start }) => {
  const fixture = await start("restart-agents", { hold: true, reloads: 1 });
  await fixture.setRestartControl("start", "up");
  const before = await pidLine(fixture);

  // The queue starts the agent after the page loads: reopen the dialog until its check counts it.
  // The dialog checks once on open, and its backdrop blocks the menu, so each try closes it first.
  const closeDialog = () =>
    expect(async () => {
      await page.keyboard.press("Escape");
      await expect(dialog(page)).toHaveCount(0, { timeout: 1_000 });
    }).toPass({ timeout: 10_000 });
  const counted = async () => {
    await expect(async () => {
      await closeDialog();
      await openDialog(page);
      await expect(dialog(page)).toContainText("1 agent is running", { timeout: 2_000 });
    }).toPass();
    await expect(dialog(page)).toContainText("Agents keep running through the restart");
  };

  await counted();
  await expect(dialog(page)).not.toContainText("A migration runs while the agents work.");
  await closeDialog();

  await fixture.setRestartControl("check.json", passing("fixture-next", { ok: true }, { demo: 1 }));
  await counted();
  await expect(dialog(page)).toContainText("A migration runs while the agents work.");

  await restartButton(page).click();
  await expectReloaded(page);
  await expect.poll(() => pidLine(fixture)).not.toBe(before);
});

// Also covers 178 (with nothing pending the check is ok at once and the button is plain).
test("138. Restart dialog, dry run", async ({ page, start }) => {
  const fixture = await start("restart-projects", { reloads: 1 });
  await fixture.setRestartControl("start", "up");
  const before = await pidLine(fixture);

  await openDialog(page);
  await expect(dialog(page).getByRole("alert")).toContainText("broken: fixture migration failed");
  await expect(dialog(page).getByRole("alert")).toContainText("Migration block");
  await expect(dialog(page).getByRole("alert")).toContainText("retry the migration from the strip");
  await expect(dialog(page).getByRole("button", nav("Restart"))).toHaveCount(0);
  await expect(restartButton(page)).toHaveText("Restart anyway");
  await restartButton(page).click();
  await expectReloaded(page);
  await expect.poll(() => pidLine(fixture)).not.toBe(before);

  await fixture.setRestartControl("check.json", passing("fixture-next"));
  const began = Date.now();
  await openDialog(page);
  await expect(restartButton(page)).toHaveText("Restart");
  await expect(restartButton(page)).toBeEnabled();
  expect(Date.now() - began).toBeLessThan(10_000);
  await expect(dialog(page).getByRole("alert")).toHaveCount(0);
  await expect(dialog(page)).not.toContainText("A migration runs while the agents work.");
});
