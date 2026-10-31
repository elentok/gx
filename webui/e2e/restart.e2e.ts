import { expect, nav, test } from "./fixture";
import { buildText, dialog, expectReloaded, menu, menuButton, openDialog, passing, pidLine, restartButton, restartItem } from "./restart-helpers";

test("128. Restart server in the menu", async ({ page, start }) => {
  await start("restart-projects");
  await menuButton(page).click();
  const item = restartItem(page);
  await expect(item).toBeVisible();
  const afk = await menu(page).getByRole("menuitemcheckbox", nav("AFK")).boundingBox();
  const restart = await item.boundingBox();
  const build = await menu(page).getByTestId("server-build").boundingBox();
  expect(restart!.y).toBeGreaterThan(afk!.y);
  expect(build!.y).toBeGreaterThan(restart!.y);
  await item.click();
  await expect(menu(page)).toHaveCount(0);
  await expect(dialog(page)).toBeVisible();
});

test("129. Restart server keyboard", async ({ page, start }) => {
  await start("restart-projects");
  await menuButton(page).focus();
  await page.keyboard.press("Enter");
  await expect(menu(page)).toBeVisible();
  await page.keyboard.press("ArrowDown");
  await page.keyboard.press("ArrowDown");
  await expect(restartItem(page)).toHaveAttribute("data-highlighted", "");
  await page.keyboard.press("Enter");
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

test("131. Restart dialog checks", async ({ page, start }) => {
  const fixture = await start("restart-projects");
  await fixture.setRestartControl("check.json", passing("fixture-next"));
  await fixture.setRestartControl("slow", "3");
  const build = await buildText(page);

  await openDialog(page);
  await expect(dialog(page).getByText("Checking...")).toBeVisible();
  await expect(restartButton(page)).toBeDisabled();
  await expect(restartButton(page)).toBeEnabled();
  await expect(dialog(page)).toContainText(`Running version: ${build}`);
  await expect(dialog(page)).toContainText("Next version: fixture-next");
});

test("132. Restart dialog, same version", async ({ page, start }) => {
  const fixture = await start("restart-projects", { reloads: 1 });
  const build = await buildText(page);
  await fixture.setRestartControl("check.json", passing(build));
  await fixture.setRestartControl("start", "up");
  const before = await pidLine(fixture);

  await openDialog(page);
  await expect(dialog(page)).toContainText("Versions match");
  await expect(restartButton(page)).toBeEnabled();
  await restartButton(page).click();
  await expectReloaded(page);
  await expect.poll(() => pidLine(fixture)).not.toBe(before);
});

test("133. Restart dialog, other install path", async ({ page, start }) => {
  const fixture = await start("restart-launchd");
  const build = await buildText(page);
  await fixture.setRestartControl(
    "check.json",
    JSON.stringify({ version: build, config: { ok: true }, projects: [{ name: "demo", ok: true, files: [] }] }),
  );

  await openDialog(page);
  await expect(dialog(page)).toContainText("Versions match");
  await expect(restartButton(page)).toBeEnabled();
});

test("134. Restart dialog, unreadable version", async ({ page, start }) => {
  // A binary that fails to run: the fake cannot read its check file.
  const failing = await start("restart-projects");
  await failing.clearRestartControl("check.json");
  const before = await pidLine(failing);
  await openDialog(page);
  await expect(dialog(page).getByRole("alert")).toContainText(/check\.json/);
  await expect(restartButton(page)).toBeDisabled();
  expect(await pidLine(failing)).toBe(before);

  // A binary that is missing.
  const missing = await start("restart-missing");
  const beforeMissing = await pidLine(missing);
  await openDialog(page);
  await expect(dialog(page).getByRole("alert")).toContainText(/no such file/i);
  await expect(restartButton(page)).toBeDisabled();
  expect(await pidLine(missing)).toBe(beforeMissing);
});

test("135. Restart dialog, bad config", async ({ page, start }) => {
  const fixture = await start("restart-projects");
  await fixture.setRestartControl("check.json", passing("fixture-next", { ok: false, error: "fixture config: port is not a number" }));
  const before = await pidLine(fixture);

  await openDialog(page);
  await expect(dialog(page).getByRole("alert")).toContainText("fixture config: port is not a number");
  await expect(restartButton(page)).toBeDisabled();
  expect(await pidLine(fixture)).toBe(before);
});

test("136. Restart dialog, old binary", async ({ page, start }) => {
  const fixture = await start("restart-projects");
  await fixture.setRestartControl("old", "1");

  await openDialog(page);
  await expect(dialog(page).getByRole("alert")).toContainText("gx server restart");
  await expect(restartButton(page)).toBeDisabled();
});

test("137. Restart dialog, running agents", async ({ page, start }) => {
  const fixture = await start("restart-agents", { hold: true, reloads: 1 });
  await fixture.setRestartControl("start", "up");
  const before = await pidLine(fixture);

  // The queue starts the agent after the page loads: reopen the dialog until its check counts it.
  await expect(async () => {
    await openDialog(page);
    await expect(dialog(page)).toContainText("1 agent is running", { timeout: 2_000 });
  }).toPass();
  await expect(dialog(page)).toContainText("Agents keep running through the restart");
  await expect(dialog(page)).toContainText("A migration runs while the agents work.");

  await restartButton(page).click();
  await expectReloaded(page);
  await expect.poll(() => pidLine(fixture)).not.toBe(before);
});

test("138. Restart dialog, dry run", async ({ page, start }) => {
  const fixture = await start("restart-projects", { reloads: 1 });
  await fixture.setRestartControl("start", "up");
  const before = await pidLine(fixture);

  await openDialog(page);
  await expect(dialog(page).getByRole("alert")).toContainText("broken: fixture migration failed");
  await expect(dialog(page).getByRole("button", nav("Restart"))).toHaveCount(0);
  await restartButton(page).click();
  await expectReloaded(page);
  await expect.poll(() => pidLine(fixture)).not.toBe(before);
});
