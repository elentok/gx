import type { Page } from "@playwright/test";
import { card, expect, nav, test } from "./fixture";

const goto = (page: Page, hash: string) => page.evaluate((h) => (window.location.hash = h), hash);

// One journey over the `workspaces` scenario: the sidebar's rows follow the active workspace, and
// Move, Remove and Hold work from every place an item shows.
test("Workspaces", async ({ page, start }) => {
  const fx = await start("workspaces");
  const sidebar = page.getByRole("complementary", nav("Sidebar"));
  const row = (title: string) => sidebar.getByRole("link", { name: new RegExp(`^${title}(?![\\w-])`) });
  const switcher = page.getByRole("button", { name: /^Workspace:/ });
  const pick = async (name: string) => {
    await switcher.click();
    await page.getByRole("menuitem", { name: new RegExp(`^${name}`) }).click();
    await expect(page.getByRole("button", nav(`Workspace: ${name}`))).toBeVisible();
  };

  // The sidebar shows the Default workspace's rows on the Epics page.
  await expect(page.getByRole("button", nav("Workspace: Default workspace"))).toBeVisible();
  await expect(row("ready-epic")).toBeVisible();
  await expect(row("ws-design")).toHaveCount(0);

  // The chevron expands an epic's agents; the open epic starts expanded.
  const links = sidebar.getByRole("link");
  const expand = sidebar.getByRole("button", nav("Expand inv-epic"));
  await expect(expand).toHaveAttribute("aria-expanded", "false");
  const collapsed = await links.count();
  await expand.click();
  await expect(sidebar.getByRole("button", nav("Collapse inv-epic"))).toHaveAttribute("aria-expanded", "true");
  await expect(links).toHaveCount(collapsed + 1);
  await sidebar.getByRole("button", nav("Collapse inv-epic")).click();
  await expect(links).toHaveCount(collapsed);

  // Switching workspace changes the rows, on the Epics page and after in-app navigation to a Design.
  await pick("gx bugs");
  await expect(row("ws-design")).toBeVisible();
  await expect(row("ready-epic")).toHaveCount(0);
  await row("ws-design").click();
  await expect(page).toHaveURL(/#\/e\/demo:ws-design$/);
  await expect(row("ws-design")).toHaveAttribute("aria-current", "page");
  // An open item keeps its own workspace active, so leave the page before picking another.
  await goto(page, "#/epics");
  await pick("research");
  await expect(row("ws-impl-epic")).toBeVisible();
  await expect(row("ws-design")).toHaveCount(0);


  // Move from a sidebar row: the row leaves this workspace and shows in the target.
  const rowMenu = (title: string) => row(title).locator("xpath=..").getByRole("button", nav("More actions"));
  const moveTo = async (menu: ReturnType<typeof rowMenu>, item: string, to: string) => {
    await menu.click();
    await page.getByRole("menuitem", nav("Move to workspace")).click();
    await page.getByRole("dialog", nav(`Move ${item}`)).getByRole("button", nav(to)).click();
  };
  await moveTo(rowMenu("ws-impl-epic"), "ws-impl-epic", "gx bugs");
  await expect(row("ws-impl-epic")).toHaveCount(0);
  await pick("gx bugs");
  await expect(row("ws-impl-epic")).toBeVisible();

  // Remove asks first, and sends the item back to Default; Default's rows have no Remove.
  await rowMenu("ws-impl-epic").click();
  await page.getByRole("menuitem", nav("Remove from workspace")).click();
  const removing = page.getByRole("dialog", nav("Remove ws-impl-epic"));
  await removing.getByRole("button", nav("Keep")).click();
  await expect(row("ws-impl-epic")).toBeVisible();
  await rowMenu("ws-impl-epic").click();
  await page.getByRole("menuitem", nav("Remove from workspace")).click();
  await removing.getByRole("button", nav("Remove")).click();
  await expect(row("ws-impl-epic")).toHaveCount(0);
  await pick("Default workspace");
  await expect(row("ws-impl-epic")).toBeVisible();
  await rowMenu("ws-impl-epic").click();
  await expect(page.getByRole("menuitem", nav("Move to workspace"))).toBeVisible();
  await expect(page.getByRole("menuitem", nav("Remove from workspace"))).toHaveCount(0);
  await page.getByRole("menuitem", nav("Move to workspace")).click();
  await page.getByRole("dialog", nav("Move ws-impl-epic")).getByRole("button", nav("research")).click();
  await expect(row("ws-impl-epic")).toHaveCount(0);

  // Move from an Epics page card.
  await goto(page, "#/epics");
  await expect(card(page, "ws-impl-epic").getByTestId("workspace")).toHaveText("research");
  await card(page, "ws-impl-epic").getByRole("button", nav("More actions")).click();
  await page.getByRole("menuitem", nav("Move to workspace")).click();
  await page.getByRole("dialog", nav("Move ws-impl-epic")).getByRole("button", nav("gx bugs")).click();
  await expect(card(page, "ws-impl-epic").getByTestId("workspace")).toHaveText("gx bugs");

  // Jump dialog: Move from a row, then opening an item in another workspace switches the sidebar.
  const jump = page.getByRole("dialog", nav("Jump to"));
  const jumpRow = (title: string) => jump.getByRole("option").filter({ hasText: title }).first();
  const openJump = async (filter: string) => {
    await page.keyboard.press("ControlOrMeta+k");
    await jump.getByRole("combobox").fill(filter);
  };
  await pick("Default workspace");
  await openJump("held-epic");
  await jumpRow("held-epic").click();
  await expect(page).toHaveURL(/#\/e\/demo:held-epic$/);
  // A held item does not switch the workspace.
  await expect(page.getByRole("button", nav("Workspace: Default workspace"))).toBeVisible();
  await goto(page, "#/epics");
  await openJump("ws-impl-epic");
  await jumpRow("ws-impl-epic").getByRole("button", nav("More actions")).click();
  await page.getByRole("menuitem", nav("Move to workspace")).click();
  await page.getByRole("dialog", nav("Move ws-impl-epic")).getByRole("button", nav("research")).click();
  await expect(jumpRow("ws-impl-epic")).toContainText("research");
  await jumpRow("ws-impl-epic").click();
  await expect(page).toHaveURL(/#\/e\/demo:ws-impl-epic$/);
  await expect(page.getByRole("button", nav("Workspace: research"))).toBeVisible();
  await expect(row("ws-impl-epic")).toBeVisible();
  await goto(page, "#/epics");

  // A chat row shows in its workspace only.
  await pick("Default workspace");
  await expect(sidebar.getByRole("img", nav("Chat"))).toHaveCount(1);
  await pick("research");
  await expect(sidebar.getByRole("img", nav("Chat"))).toHaveCount(0);

  // Hold from an Epics card: the row moves to the collapsed On hold section.
  await pick("Default workspace");
  const onHold = sidebar.getByRole("button", { name: /^On hold \(\d+\)$/ });
  await expect(onHold).toHaveCount(0);
  await card(page, "ready-epic").getByRole("button", nav("More actions")).click();
  await page.getByRole("menuitem", nav("Hold")).click();
  await expect(onHold).toHaveText("On hold (1)");
  await expect(onHold).toHaveAttribute("aria-expanded", "false");
  await expect(row("ready-epic")).toHaveCount(0);
  await onHold.click();
  await expect(row("ready-epic")).toBeVisible();

  // Take it off hold from the sidebar row, then hold it again from the same row.
  await rowMenu("ready-epic").click();
  await page.getByRole("menuitem", nav("Take off hold")).click();
  await expect(onHold).toHaveCount(0);
  await expect(row("ready-epic")).toBeVisible();
  await rowMenu("ready-epic").click();
  await page.getByRole("menuitem", nav("Hold")).click();
  await expect(onHold).toHaveText("On hold (1)");

  // Hold from the page header, and from a jump row.
  await goto(page, "#/e/demo:design-new");
  await page.getByRole("button", nav("More actions")).last().click();
  await page.getByRole("menuitem", nav("Hold")).click();
  await expect(onHold).toHaveText("On hold (2)");
  await expect(row("design-new")).toHaveCount(0);
  await goto(page, "#/epics");
  await openJump("finished-a");
  await jumpRow("finished-a").getByRole("button", nav("More actions")).click();
  await page.getByRole("menuitem", nav("Hold")).click();
  await expect(jumpRow("finished-a")).toContainText("On hold");
  await page.keyboard.press("Escape");

  // A Design with live sessions asks first.
  await pick("gx bugs");
  await rowMenu("ws-design").click();
  await page.getByRole("menuitem", nav("Hold")).click();
  const holding = page.getByRole("dialog", nav("Hold ws-design"));
  await holding.getByRole("button", nav("Cancel")).click();
  await expect(row("ws-design")).toBeVisible();
  await rowMenu("ws-design").click();
  await page.getByRole("menuitem", nav("Hold")).click();
  await holding.getByRole("button", nav("Hold and end sessions")).click();
  await expect(row("ws-design")).toHaveCount(0);
  await expect(onHold).toBeVisible();

  // Other sits below On hold, is the same in every workspace, and has no Move or Hold.
  const other = sidebar.getByRole("button", nav("Other"));
  const otherRows = ["fix-demo_cone-offs_s04", "fix-demo", "one-offs-iter-01"];
  const showOther = async () => {
    if ((await other.getAttribute("aria-expanded")) === "false") await other.click();
    for (const name of otherRows) await expect(row(name)).toBeVisible();
  };
  await showOther();
  for (const name of otherRows) await expect(row(name).locator("xpath=..").getByRole("button", nav("More actions"))).toHaveCount(0);
  await pick("Default workspace");
  await showOther();
  // Opening an Other row leaves the workspace alone.
  await row("fix-demo").click();
  await expect(page).toHaveURL(/#\/agents\//);
  await expect(page.getByRole("button", nav("Workspace: Default workspace"))).toBeVisible();
  await goto(page, "#/epics");
  // A ticket's Fix agent nests under its epic, not under Other.
  await sidebar.getByRole("button", nav("Expand paused-epic")).click();
  await expect(row("fix-demo_cpaused-epic_s01")).toBeVisible();

  // Manage workspaces: order, rename, reorder, delete naming its members; Default has no controls.
  const manage = page.getByRole("dialog", nav("Manage workspaces"));
  const names = manage.getByRole("textbox");
  const openManage = async () => {
    await switcher.click();
    await page.getByRole("menuitem", nav("Manage workspaces")).click();
  };
  await openManage();
  await expect(names).toHaveCount(2);
  await expect(names.nth(0)).toHaveValue("gx bugs");
  await expect(names.nth(1)).toHaveValue("research");
  await expect(manage.getByRole("button", { name: /^(Rename|Delete) Default/ })).toHaveCount(0);
  await expect(manage.getByRole("button", nav("Move Default workspace up"))).toBeDisabled();
  await names.nth(1).fill("labs");
  await names.nth(1).press("Enter");
  await expect(names.nth(1)).toHaveValue("labs");
  await manage.getByRole("button", nav("Move labs up")).click();
  await expect(names.nth(0)).toHaveValue("labs");
  await expect(names.nth(1)).toHaveValue("gx bugs");
  await manage.getByRole("button", nav("Delete gx bugs")).click();
  const deleting = page.getByRole("dialog", nav("Delete gx bugs"));
  await expect(deleting).toContainText(/Its \d+ items? go(es)? back to the Default workspace/);
  await deleting.getByRole("button", nav("Keep")).click();
  await expect(names).toHaveCount(2);
  await manage.getByRole("button", nav("Delete gx bugs")).click();
  await deleting.getByRole("button", nav("Delete")).click();
  await expect(names).toHaveCount(1);
  await manage.getByRole("button", nav("Close")).click();

  // CLI changes show in the open page without a reload; deleting the active workspace drops to Default.
  await pick("labs");
  await fx.cli("server", "workspaces", "rename", "labs", "studies");
  // The renamed workspace is a new name to the page: the old one vanished, so the active one is Default.
  await expect(page.getByRole("button", nav("Workspace: Default workspace"))).toBeVisible();
  await pick("studies");
  await fx.cli("server", "workspaces", "create", "scratch");
  await fx.cli("server", "workspaces", "reorder", "scratch", "Default workspace", "studies");
  await switcher.click();
  const items = page.getByRole("menuitem").filter({ hasText: /^(scratch|Default workspace|studies)/ });
  await expect(items.nth(0)).toHaveText(/^scratch/);
  await expect(items.nth(2)).toHaveText(/^studies/);
  await page.keyboard.press("Escape");
  await fx.cli("server", "workspaces", "delete", "studies");
  await expect(page.getByRole("button", nav("Workspace: Default workspace"))).toBeVisible();
});
