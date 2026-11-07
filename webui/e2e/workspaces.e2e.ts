import type { Page } from "@playwright/test";
import { card, expect, nav, test } from "./fixture";

const goto = (page: Page, hash: string) => page.evaluate((h) => (window.location.hash = h), hash);

// One journey over the `workspaces` scenario: the sidebar's rows follow the active workspace, and
// Move, Remove and Hold work from every place an item shows.
test("Workspaces", async ({ page, start }) => {
  await start("workspaces");
  const sidebar = page.getByRole("complementary", nav("Sidebar"));
  const row = (title: string) => sidebar.getByRole("link", { name: new RegExp(`^${title}\\b`) });
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
});
