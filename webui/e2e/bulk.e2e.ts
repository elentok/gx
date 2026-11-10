import { epicsButton, expect, nav, openEpics, test } from "./fixture";

// Select mode's rendering and keys are vitest's (SidebarSelect, EpicsPage and ArchivePage tests); this flow covers
// what needs the server and real layout: Move, Hold, Stop and the Archive confirm through the bar, a CLI archive
// leaving the selection, and the two ways into the Archive page.
test("Bulk actions", async ({ page, start }) => {
  test.setTimeout(120_000);
  const fx = await start("bulk-actions");
  await page.addStyleTag({ content: "*,*::before,*::after{animation-duration:0s!important;animation-delay:0s!important;transition-duration:0s!important}" });
  const sidebar = page.getByRole("complementary", nav("Sidebar"));
  const box = (name: string) => sidebar.getByRole("checkbox", nav(`Select ${name}`));
  const bar = page.locator('[data-part="select-bar"]');
  const count = bar.getByTestId("select-count");
  const barButton = (name: string) => bar.getByRole("button", { name: new RegExp(`^${name}\\b`) });
  const dialog = (name: string) => page.getByRole("dialog", nav(name));

  const enter = async () => {
    await sidebar.getByRole("button", nav("Select")).click();
    await expect(bar).toBeVisible();
  };

  // Entry 1: the header's Select opens the checkbox column and the bar.
  await expect(box("bulk-merged")).toHaveCount(0);
  await enter();
  await expect(count).toHaveText("0 selected");
  await expect(box("bulk-merged")).toBeVisible();

  // Entry 11: Move opens the workspace menu above the bar and moves the fitting rows.
  await box("bulk-done").check();
  await box("bulk-unmerged").check();
  await expect(count).toHaveText("2 selected");
  await barButton("Move").click();
  const menu = page.getByRole("menu");
  await expect(menu).toBeVisible();
  // The menu opens upward from the Move button, so it never covers the bar's own buttons.
  const moveTop = (await barButton("Move").boundingBox())!.y;
  const menuBox = (await menu.boundingBox())!;
  expect(menuBox.y + menuBox.height).toBeLessThanOrEqual(moveTop);
  await menu.getByRole("menuitem", { name: /^bulk/ }).click();
  // The moved rows leave the Default workspace's list, and the mode ends because nothing is selected any more.
  await expect(bar).toHaveCount(0);
  await expect(sidebar.getByRole("link", nav("bulk-done Not started"))).toHaveCount(0);
  // The CLI verb takes many rows too.
  const moved = await fx.cli("server", "workspaces", "move", "demo:bulk-done-impl", "demo:bulk-unmerged", "bulk", "--json");
  expect(JSON.parse(moved).results).toHaveLength(2);

  // Entry 12: Hold on a live Design asks and lists its Sessions; on an epic it asks nothing.
  await enter();
  await box("bulk-early").check();
  await barButton("Hold").click();
  await expect(dialog("Hold selected")).toContainText("Holding ends these live sessions");
  await dialog("Hold selected").getByRole("button", nav("Cancel")).click();
  await expect(dialog("Hold selected")).toHaveCount(0);
  await expect(count).toHaveText("1 selected");
  await box("bulk-early").uncheck();
  await box("bulk-clash").check();
  await barButton("Hold").click();
  await expect(box("bulk-clash")).toHaveCount(0);
  await expect(dialog("Hold selected")).toHaveCount(0);

  // Entry 14: Stop confirms once and lists the rows; the Exited agent is skipped as not live.
  await sidebar.getByRole("button", nav("Expand bulk-paused")).click();
  await expect(sidebar.getByRole("link", { name: /^Fix agent / })).toBeVisible();
  await enter();
  await sidebar.getByRole("checkbox", { name: /^Select fix-/ }).check();
  await box("Claude Code").check();
  await barButton("Stop").click();
  await expect(dialog("Stop selected")).toContainText("Stop ");
  await expect(dialog("Stop selected")).not.toContainText("Claude Code");
  await dialog("Stop selected").getByRole("button", nav("Stop")).click();
  await expect(dialog("Stop selected")).toHaveCount(0);
  await expect(page.getByText("Not run on some rows")).toBeVisible();
  await expect(page.getByText(/Claude Code.*not live/)).toBeVisible();
  // The stopped row leaves the selection; the skipped one stays.
  await expect(count).toHaveText("1 selected");
  await bar.getByRole("button", nav("Done")).click();
  await expect(bar).toHaveCount(0);

  // Entry 17: the Archive confirm lists the rows, the live Sessions and one warning per epic not on trunk.
  // The scenario has no live Session on an epic that fits, so the Sessions line is covered by the rig and vitest.
  await enter();
  await box("bulk-merged").check();
  await box("bulk-done-solo").check();
  await box("bulk-early").check();
  await barButton("Archive").click();
  const archive = dialog("Archive selected");
  // The live Design is not finished, so the bar leaves it out of the dialog.
  await expect(archive).toContainText("Archive 2 epics?");
  for (const name of ["bulk-merged", "bulk-done-solo"]) await expect(archive.getByRole("listitem").filter({ hasText: name })).toHaveCount(1);
  await expect(archive.getByRole("listitem")).toHaveCount(2);
  await expect(archive.getByText(/is not merged to trunk/)).toHaveCount(1);
  await expect(archive.getByText("⚠ bulk-done-solo is not merged to trunk")).toBeVisible();
  await archive.getByRole("button", nav("Cancel")).click();
  await expect(archive).toHaveCount(0);

  // Entry 22: an epic archived through the CLI leaves the selection.
  await expect(count).toHaveText("3 selected");
  const archived = await fx.cli("server", "epics", "archive", "demo:bulk-merged", "--json");
  expect(JSON.parse(archived).results).toHaveLength(1);
  await expect(count).toHaveText("2 selected");
  await expect(box("bulk-merged")).toHaveCount(0);
  await bar.getByRole("button", nav("Done")).click();
  await expect(bar).toHaveCount(0);

  // Entry 26: the footer keeps its width while the Epics menu is open, and Archive opens the Archive page.
  const footer = page.locator('[data-part="sidebar-footer"]');
  const width = async () => (await footer.boundingBox())!.width;
  const before = await width();
  await epicsButton(page).click();
  const epicsMenu = page.getByRole("menuitem", { name: /^Archive/ });
  await expect(epicsMenu).toBeVisible();
  expect(await width()).toBe(before);
  await epicsMenu.click();
  await expect(page).toHaveURL(/#\/archive$/);
  await expect(page.getByRole("heading", nav("Archive"))).toBeVisible();

  // Entry 25: the Epics page header's Archive link opens the Archive page.
  await openEpics(page);
  await expect(page).toHaveURL(/#\/epics$/);
  await page.getByRole("link", nav("Archive")).click();
  await expect(page).toHaveURL(/#\/archive$/);
  await expect(page.getByRole("heading", nav("Archive"))).toBeVisible();
});
