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
  // The first Escape can land before a menu takes focus, and a closing menu keeps covering the sidebar until it is gone.
  // A click on a submenu item that lands before the menu has taken focus does not open it.
  const menuFocused = () => expect(page.locator('[role="menu"]:focus-within')).toHaveCount(1);
  const closeMenus = () =>
    expect(async () => {
      await page.keyboard.press("Escape");
      await expect(page.getByRole("menu")).toHaveCount(0, { timeout: 1_000 });
    }).toPass({ timeout: 10_000 });
  const pick = async (name: string) => {
    await switcher.click();
    await page.getByRole("menuitem", { name: new RegExp(`^${name}`) }).click();
    await expect(page.getByRole("button", nav(`Workspace: ${name}`))).toBeVisible();
    // The closing menu keeps covering the sidebar, and its items answer later queries, until it is gone.
    await expect(page.getByRole("menu")).toHaveCount(0);
  };

  // The sidebar shows the Default workspace's rows on the Epics page.
  await expect(page.getByRole("button", nav("Workspace: Default workspace"))).toBeVisible();
  await expect(row("ready-epic")).toBeVisible();
  await expect(row("ws-design")).toHaveCount(0);

  // Clicking an epic row only navigates; the chevron expands and collapses its agents, and a page visit leaves the row as left.
  // Counted inside the epic's own item, so a row still loading elsewhere cannot skew the count.
  const links = row("inv-epic").locator("xpath=ancestor::li[1]").getByRole("link");
  const expand = sidebar.getByRole("button", nav("Expand inv-epic"));
  await expect(expand).toHaveAttribute("aria-expanded", "false");
  await expect(links).toHaveCount(1);
  await row("inv-epic").click();
  await expect(page).toHaveURL(/#\/e\/demo:inv-epic$/);
  await expect(expand).toHaveAttribute("aria-expanded", "false");
  await expect(links).toHaveCount(1);
  await expand.click();
  await expect(sidebar.getByRole("button", nav("Collapse inv-epic"))).toHaveAttribute("aria-expanded", "true");
  await expect(links).toHaveCount(2);
  await goto(page, "#/epics");
  await expect(links).toHaveCount(2);
  await sidebar.getByRole("button", nav("Collapse inv-epic")).click();
  await expect(links).toHaveCount(1);
  await goto(page, "#/e/demo:inv-epic");
  await expect(links).toHaveCount(1);
  await goto(page, "#/epics");

  // Rows show their numbers while Alt is held, and Alt+1 opens the first one.
  // Numbers take the menu-button slot; nothing else changes: no /v1 request, no DOM change outside the sidebar.
  const numbers = sidebar.getByTestId("row-number");
  const status = row("ready-epic").locator("span").last();
  await expect(numbers).toHaveCount(0);
  const wordsBefore = await status.textContent();
  // The Epics page's own loads (epics, checks) land after the goto above; count requests only once its cards are drawn.
  await expect(card(page, "ready-epic")).toBeVisible();
  await page.waitForResponse((r) => r.url().includes("/v1/checks/status")).catch(() => {});
  const requests: string[] = [];
  page.on("request", (r) => void (new URL(r.url()).pathname.startsWith("/v1/") && requests.push(r.url())));
  await page.evaluate(() => {
    const w = window as unknown as { __outside: MutationRecord[] };
    w.__outside = [];
    const side = document.querySelector('[aria-label="Sidebar"]');
    new MutationObserver((rs) => w.__outside.push(...rs.filter((r) => !side?.contains(r.target)))).observe(document.body, { subtree: true, childList: true, attributes: true, characterData: true });
  });
  await page.keyboard.down("Alt");
  await expect(numbers.first()).toHaveText("1");
  await expect(status).toHaveText(wordsBefore!);
  await page.keyboard.up("Alt");
  await expect(numbers).toHaveCount(0);
  expect(requests).toEqual([]);
  expect(await page.evaluate(() => (window as unknown as { __outside: MutationRecord[] }).__outside.length)).toBe(0);
  // A focused text field makes Alt do nothing.
  await page.evaluate(() => {
    const i = document.createElement("input");
    i.id = "alt-probe";
    document.body.appendChild(i);
    i.focus();
  });
  await page.keyboard.down("Alt");
  await expect(numbers).toHaveCount(0);
  await page.keyboard.up("Alt");
  await page.evaluate(() => document.getElementById("alt-probe")?.remove());
  const firstHref = (await sidebar.getByRole("link").first().getAttribute("href"))!.slice(1);
  await page.keyboard.press("Alt+1");
  await expect.poll(() => new URL(page.url()).hash.slice(1)).toBe(firstHref);
  await goto(page, "#/epics");

  // Cmd/Ctrl+B hides the sidebar and brings it back with focus on a row; j/k, l/h and Enter move, expand and open.
  // The roving stop is the last row focused: the earlier row click left it on inv-epic, so put it back on the first.
  const navs = sidebar.locator("[data-nav]");
  await navs.first().focus();
  await page.locator("body").click({ position: { x: 600, y: 600 } });
  await page.keyboard.press("ControlOrMeta+b");
  await expect(sidebar).toBeHidden();
  await page.keyboard.press("ControlOrMeta+b");
  await expect(sidebar).toBeVisible();
  await expect(navs.first()).toBeFocused();
  await page.keyboard.press("j");
  await expect(navs.nth(1)).toBeFocused();
  await page.keyboard.press("ArrowUp");
  await expect(navs.first()).toBeFocused();
  await page.keyboard.press("k");
  await expect(navs.first()).toBeFocused();
  await row("inv-epic").focus();
  await page.keyboard.press("l");
  await expect(sidebar.getByRole("button", nav("Collapse inv-epic"))).toHaveAttribute("aria-expanded", "true");
  await page.keyboard.press("h");
  await expect(sidebar.getByRole("button", nav("Expand inv-epic"))).toHaveAttribute("aria-expanded", "false");
  await page.keyboard.press("Enter");
  await expect(page).toHaveURL(/#\/e\/demo:inv-epic$/);
  await goto(page, "#/epics");

  // Enter on a chat row focuses its prompt; a click opens the chat and leaves the prompt alone.
  const chatRow = sidebar.locator("a[data-chat]");
  const prompt = page.getByRole("form", nav("Composer")).getByRole("textbox");
  await chatRow.focus();
  await page.keyboard.press("Enter");
  await expect(page).toHaveURL(/#\/agents\//);
  await expect(prompt).toBeFocused();
  await goto(page, "#/epics");
  await chatRow.click();
  await expect(prompt).toBeVisible();
  await expect(prompt).not.toBeFocused();
  await goto(page, "#/epics");

  // Alt+Shift+2 picks the second workspace.
  await page.keyboard.press("Alt+Shift+2");
  await expect(page.getByRole("button", nav("Workspace: gx bugs"))).toBeVisible();
  await expect(row("ws-design")).toBeVisible();
  await page.keyboard.press("Alt+Shift+1");
  await expect(page.getByRole("button", nav("Workspace: Default workspace"))).toBeVisible();

  // Switching workspace changes the rows, on the Epics page and after in-app navigation to a Design.
  await pick("gx bugs");
  await expect(row("ws-design")).toBeVisible();
  await expect(row("ready-epic")).toHaveCount(0);
  await row("ws-design").click();
  // A Design row opens the Design page with its composer, not the epic page.
  await expect(page).toHaveURL(/#\/d\/demo:ws-design$/);
  await expect(page.getByRole("form", { name: "Composer" })).toBeVisible();
  await expect(row("ws-design")).toHaveAttribute("aria-current", "page");
  // Picking another workspace on the open item's page switches to it and the pick sticks.
  await pick("research");
  await expect(page.getByRole("button", nav("Workspace: research"))).toBeVisible();
  await expect(row("ws-impl-epic")).toBeVisible();
  await expect(row("ws-design")).toHaveCount(0);
  await expect(page).toHaveURL(/#\/d\/demo:ws-design$/);


  // Move from a sidebar row: the row leaves this workspace and shows in the target.
  const rowMenu = (title: string) => row(title).locator("xpath=..").getByRole("button", nav("More actions"));
  const moveTo = async (menu: ReturnType<typeof rowMenu>, item: string, to: string) => {
    await menu.click();
    await menuFocused();
    await page.getByRole("menuitem", nav("Move to workspace")).click();
    await page.getByRole("menuitem", nav(to)).click();
    await expect(page.getByRole("menu")).toHaveCount(0);
  };
  // The submenu opens to the side of its item, lists every workspace (the Default workspace too) and marks the current one.
  await rowMenu("ws-impl-epic").click();
  const moveItem = page.getByRole("menuitem", nav("Move to workspace"));
  const current = page.getByRole("menu").last().getByRole("menuitem", { name: /^research/ });
  await menuFocused();
  await moveItem.click();
  await expect(current).toHaveAttribute("aria-current", "true");
  await expect(page.getByRole("menuitem", nav("Default workspace"))).toHaveAttribute("aria-current", "false");
  await expect(page.getByRole("menuitem", nav("gx bugs"))).toBeVisible();
  // Both menus animate in; measure once they hold still.
  await expect(async () => {
    const [itemBox, subBox] = [await moveItem.boundingBox(), await current.boundingBox()];
    expect(subBox!.x).toBeGreaterThanOrEqual(itemBox!.x + itemBox!.width - 1);
  }).toPass({ timeout: 5_000 });
  await expect(page.getByRole("menuitem", nav("Remove from workspace"))).toHaveCount(0);
  await page.keyboard.press("Escape");
  await page.keyboard.press("Escape");

  await moveTo(rowMenu("ws-impl-epic"), "ws-impl-epic", "gx bugs");
  await expect(row("ws-impl-epic")).toHaveCount(0);
  await pick("gx bugs");
  await expect(row("ws-impl-epic")).toBeVisible();
  await moveTo(rowMenu("ws-impl-epic"), "ws-impl-epic", "research");
  await expect(row("ws-impl-epic")).toHaveCount(0);

  // Move from an Epics page card.
  await goto(page, "#/epics");
  await expect(card(page, "ws-impl-epic").getByTestId("workspace")).toHaveText("research");
  await card(page, "ws-impl-epic").getByRole("button", nav("More actions")).click();
  await menuFocused();
  // The card menu sits in a page that re-renders as cards load; retry the open until the submenu stays.
  await expect(async () => {
    if (!(await page.getByRole("menuitem", nav("gx bugs")).isVisible())) await page.getByRole("menuitem", nav("Move to workspace")).click({ timeout: 2_000 });
    await page.getByRole("menuitem", nav("gx bugs")).click({ timeout: 2_000 });
  }).toPass({ timeout: 15_000 });
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
  await menuFocused();
  await page.getByRole("menuitem", nav("Move to workspace")).click();
  await page.getByRole("menuitem", nav("research")).click();
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
  await expect(page.getByRole("heading", { level: 1, name: "design-new" })).toBeVisible();
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
  await closeMenus();
  await fx.cli("server", "workspaces", "delete", "studies");
  await expect(page.getByRole("button", nav("Workspace: Default workspace"))).toBeVisible();

  // The gear menu holds AFK, Vi mode, Sort, Restart server and More settings; Sort is per workspace and follows the CLI.
  const gear = page.getByRole("button", nav("Menu"));
  const gearItems = page.getByRole("menu").locator('[role^="menuitem"]');
  await gear.click();
  await expect(gearItems).toHaveText([/^AFK$/, /^Vi mode$/, /^Sort · Default workspace/, /^Restart server$/, /^More settings\.\.\.$/]);
  const sortOptions = page.getByRole("menuitemradio");
  const openSort = async (workspace: string) => {
    await closeMenus();
    await gear.click();
    await menuFocused();
    await page.getByRole("menuitem", { name: new RegExp(`^Sort · ${workspace}`) }).click();
  };
  await openSort("Default workspace");
  await expect(sortOptions).toHaveText(["Needs you first, then by name", "By last activity, newest first", "Alphabetically by name"]);
  await expect(sortOptions.nth(0)).toHaveAttribute("aria-checked", "true");
  await sortOptions.nth(1).click();
  await expect.poll(() => fx.cli("server", "workspaces", "sort", "Default workspace")).toContain("activity");
  await fx.cli("server", "workspaces", "sort", "Default workspace", "name");
  await openSort("Default workspace");
  await expect(sortOptions.nth(2)).toHaveAttribute("aria-checked", "true");
  await closeMenus();
  await fx.cli("server", "workspaces", "sort", "Default workspace", "needs-you");

  // Rows reorder live and keyboard focus stays on the same row: the last row, which moves furthest when the order flips.
  const navRows = sidebar.locator("a[data-nav]");
  const last = navRows.last();
  await last.focus();
  const lastName = (await last.getAttribute("href"))!;
  const focusedRow = sidebar.locator("a[data-nav]:focus");
  for (const order of ["name", "activity", "needs-you"]) {
    await fx.cli("server", "workspaces", "sort", "Default workspace", order);
    await expect(focusedRow).toHaveAttribute("href", lastName);
  }
  await pick("scratch");
  await openSort("scratch");
  await expect(sortOptions.nth(0)).toHaveAttribute("aria-checked", "true");
  await closeMenus();
  await pick("Default workspace");
  await gear.click();
  await page.getByRole("menuitem", nav("More settings...")).click();
  await expect(page).toHaveURL(/#\/settings$/);
  await goto(page, "#/epics");

  // At 744px the sidebar slides over: picking a row closes it, the jump menu is a popover, no page scrolls sideways.
  await page.setViewportSize({ width: 744, height: 900 });
  await goto(page, "#/epics");
  await expect(sidebar).toBeHidden();
  await page.getByRole("button", nav("Show sidebar")).click();
  await expect(sidebar).toBeVisible();
  await gear.click();
  await menuFocused();
  await page.getByRole("menuitem", nav("More settings...")).click();
  await expect(page).toHaveURL(/#\/settings$/);
  await expect(sidebar).toBeHidden();
  await goto(page, "#/epics");
  await page.getByRole("button", nav("Show sidebar")).click();
  await row("paused-epic").click();
  await expect(page).toHaveURL(/#\/e\/demo:paused-epic$/);
  await expect(sidebar).toBeHidden();
  await page.keyboard.press("ControlOrMeta+k");
  const popover = (await jump.boundingBox())!;
  expect(popover.width).toBeLessThan(744);
  expect(popover.height).toBeLessThan(900);
  await page.keyboard.press("Escape");
  const sideways = () => page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth);
  const ticket = page.locator('a[href^="#/t/"]').first();
  await expect(ticket).toBeVisible();
  const ticketHash = (await ticket.getAttribute("href"))!;
  for (const hash of ["#/epics", "#/e/demo:paused-epic", ticketHash, "#/e/demo:ws-design", "#/agents", "#/stats", "#/settings"]) {
    await goto(page, hash);
    await expect(page.getByRole("heading").first()).toBeVisible();
    expect(await sideways(), hash).toBe(false);
  }

  // At 390px an item outside the active workspace opens through the switcher: its row is not in the sidebar.
  await page.setViewportSize({ width: 390, height: 844 });
  await goto(page, "#/epics");
  await page.getByRole("button", nav("Show sidebar")).click();
  // The header's search button opens the jump menu above the sliding sidebar; its tooltip names the key.
  const search = sidebar.getByRole("button", nav("Search"));
  await expect(search).toHaveAttribute("title", /^Search \((⌘K|Ctrl\+K)\)$/);
  await search.click();
  await expect(jump).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(jump).toHaveCount(0);
  await expect(sidebar).toBeVisible();
  // A tap outside an open menu closes it; Move swaps in place with a Back row, Back returns.
  await rowMenu("paused-epic").click();
  await expect(page.getByRole("menuitem", nav("Move to workspace"))).toBeVisible();
  await page.mouse.click(380, 600);
  await expect(page.getByRole("menuitem", nav("Move to workspace"))).toHaveCount(0);
  await rowMenu("paused-epic").click();
  await page.getByRole("menuitem", nav("Move to workspace")).click();
  await expect(page.getByRole("menuitem", nav("Hold"))).toHaveCount(0);
  await page.getByRole("menuitem", nav("‹ Back")).click();
  await expect(page.getByRole("menuitem", nav("Hold"))).toBeVisible();
  await page.mouse.click(380, 600);
  await expect(page.getByRole("menuitem", nav("Hold"))).toHaveCount(0);
  await moveTo(rowMenu("paused-epic"), "paused-epic", "scratch");
  await expect(row("paused-epic")).toHaveCount(0);
  await pick("scratch");
  await row("paused-epic").click();
  await expect(page).toHaveURL(/#\/e\/demo:paused-epic$/);
  await expect(sidebar).toBeHidden();

  // A tap on a chat row opens it without the keyboard: the prompt is not the active element. The fixture page has no touch context, so a click stands in.
  await goto(page, "#/epics");
  await page.getByRole("button", nav("Show sidebar")).click();
  await pick("Default workspace");
  await chatRow.click();
  await expect(page).toHaveURL(/#\/agents\//);
  await expect(prompt).toBeVisible();
  await expect(prompt).not.toBeFocused();

  // The chat row's "..." menu: Stop ends the live session, Remove confirms and deletes it, and removing the open chat leaves its page.
  const chatId = page.url().split("/agents/")[1];
  const chatMenu = () => chatRow.locator("xpath=ancestor::li").getByRole("button", nav("More actions"));
  await goto(page, "#/epics");
  await page.getByRole("button", nav("Show sidebar")).click();
  await chatMenu().click();
  await page.getByRole("menuitem", nav("Stop")).click();
  await expect(chatRow).toContainText("Exited");
  await chatMenu().click();
  await expect(page.getByRole("menuitem", nav("Remove"))).toBeVisible();
  await expect(page.getByRole("menuitem", nav("Stop"))).toHaveCount(0);
  await closeMenus();
  await goto(page, `#/agents/${chatId}`);
  await chatMenu().click();
  await page.getByRole("menuitem", nav("Remove")).click();
  await expect(page.getByRole("dialog")).toBeVisible();
  expect(await fx.cli("agents", "list")).toContain(chatId);
  await page.getByRole("dialog").getByRole("button", nav("Remove")).click();
  await expect(page).not.toHaveURL(new RegExp(`#/agents/${chatId}$`));
  await expect.poll(() => fx.cli("agents", "list")).not.toContain(chatId);
});
