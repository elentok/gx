import type { Locator, Page } from "@playwright/test";
import { sessions } from "./agentsKit";
import { blockEvents, card, expect, nav, sessionRows, swipe, test, touch } from "./fixture";

const goto = (page: Page, hash: string) => page.evaluate((h) => (window.location.hash = h), hash);

// One journey over the `workspaces` scenario: the sidebar's rows follow the active workspace, and
// Move, Remove and Hold work from every place an item shows.
test("Workspaces", async ({ page, start }) => {
  test.setTimeout(120_000);
  const fx = await start("workspaces");
  // Every menu close and sidebar slide waits out its animation (a third of this flow's run time); the flow checks end states, not motion.
  await page.addStyleTag({ content: "*,*::before,*::after{animation-duration:0s!important;animation-delay:0s!important;transition-duration:0s!important}" });
  const sidebar = page.getByRole("complementary", nav("Sidebar"));
  // A Fix agent's link is named "Fix agent <name>": its wrench icon carries the label.
  const row = (title: string) => sidebar.getByRole("link", { name: new RegExp(`^(Fix agent )?${title}(?![\\w-])`) });
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

  // The footer's inbox button sums the project's follow-ups (2 bugs, 2 ideas); the popover line opens the Follow-ups card, section open and in view.
  const inbox = page.getByRole("button", { name: /^Follow-ups/ });
  const followUpsLine = page.getByRole("menuitem", { name: /^demo/ });
  await expect(inbox).toHaveAttribute("aria-label", "Follow-ups · 4 open");
  await expect(inbox).toContainText("4");
  await inbox.click();
  await expect(followUpsLine).toContainText("2 bugs · 2 ideas");
  await expect(followUpsLine).toContainText("4");
  await followUpsLine.click();
  await expect(page).toHaveURL(/#\/epics$/);
  const followUpsCard = page.locator('[data-focus="follow-ups:demo"]');
  await expect(followUpsCard).toBeVisible();
  await expect(followUpsCard).toBeInViewport();
  await expect(page.getByRole("menu")).toHaveCount(0);

  // One global ring for keyboard focus: inset on a sidebar row, gap plus blue ring elsewhere, never after a click.
  const blue = "rgb(108, 180, 255)";
  const ring = new RegExp(`${blue.replace(/[()]/g, "\\$&")} 0px 0px 0px 4px`);
  await row("ready-epic").focus();
  await expect(row("ready-epic")).toHaveCSS("box-shadow", new RegExp(`${blue.replace(/[()]/g, "\\$&")} 0px 0px 0px 2px inset`));
  const iconButton = card(page, "ready-epic").getByRole("button", nav("More actions"));
  await iconButton.focus();
  await expect(iconButton).toHaveCSS("box-shadow", ring);
  const epicCard = card(page, "ready-epic");
  await epicCard.getByRole("link", nav("ready-epic")).focus();
  await expect(epicCard.getByRole("link", nav("ready-epic"))).toHaveCSS("box-shadow", ring);
  await switcher.focus();
  await expect(switcher).toHaveCSS("box-shadow", new RegExp(`${blue.replace(/[()]/g, "\\$&")} 0px 0px 0px 2px inset`));
  await page.keyboard.press("Enter");
  await menuFocused();
  await expect(page.getByRole("menuitem").first()).toHaveCSS("box-shadow", ring);
  await closeMenus();
  await pick("Default workspace");
  await expect(switcher).toHaveCSS("box-shadow", "none");
  await page.keyboard.press("Tab");
  await expect(page.locator(":focus")).toHaveCSS("box-shadow", /rgb\(108, 180, 255\) 0px 0px 0px (2px inset|4px)/);

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
  const chatRow = sidebar.locator("a[data-chat]").first();
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

  // Tree rows: a nested agent row sits on its parent's columns, has its own "..." menu (none for a Queue agent), and the button follows its own row's hover and focus.
  const itemOf = (title: string) => row(title).locator("xpath=ancestor::li[1]");
  const x = async (l: Locator) => {
    await expect(l).toBeVisible();
    return (await l.boundingBox())!.x;
  };
  const right = async (l: Locator) => {
    await expect(l).toBeVisible();
    const box = (await l.boundingBox())!;
    return box.x + box.width;
  };
  const titleOf =(link: Locator) => link.locator(":scope > span").first();
  const wordsOf = (link: Locator) => link.locator("span").last();
  const frameButton = (link: Locator) => link.locator("xpath=..").getByRole("button", nav("More actions"));
  // The button hides through an ancestor's opacity (shown on hover, focus or without hover), so multiply up to the row.
  const shown = (button: Locator) => button.evaluate((el) => { let o = 1; for (let n: Element | null = el; n && n.tagName !== "LI"; n = n.parentElement) o *= Number(getComputedStyle(n).opacity); return o; });
  const nestedLinks = (title: string) => itemOf(title).locator("ul").getByRole("link");
  expect(await x(titleOf(chatRow))).toBe(await x(titleOf(row("ready-epic"))));
  await sidebar.getByRole("button", nav("Expand paused-epic")).click();
  await sidebar.getByRole("button", nav("Expand inv-epic")).click();
  const working = nestedLinks("paused-epic").filter({ hasText: "Working" });
  const queueAgent = nestedLinks("inv-epic");
  await expect(working).toHaveCount(1);
  await expect(queueAgent).toHaveCount(1);
  await expect(queueAgent.locator("[data-testid=icon-slot] svg")).toHaveCount(0);
  await expect(working.locator("[data-testid=icon-slot] svg")).toHaveCount(1);
  const parentMenu = frameButton(row("paused-epic"));
  // The guide line runs in the parent's icon column, 3px left of its centre so it clears a Fix agent's wrench.
  const centre = async (l: Locator) => {
    await expect(l).toBeVisible();
    const box = (await l.boundingBox())!;
    return box.x + box.width / 2;
  };
  const guide = itemOf("paused-epic").locator("[data-testid=nested-guide]").first();
  expect(Math.abs((await centre(guide)) + 3 - (await centre(row("paused-epic").locator("[data-testid=icon-slot]"))))).toBeLessThanOrEqual(0.5);
  expect(await x(titleOf(working))).toBe(await x(titleOf(row("paused-epic"))));
  // The words differ in length, so their right edges are the shared column.
  expect(await right(wordsOf(working))).toBe(await right(wordsOf(row("paused-epic"))));
  expect(await x(frameButton(working))).toBe(await x(parentMenu));
  await expect(frameButton(queueAgent)).toHaveCount(0);
  // At 1280px the button shows on its own row only; Tab onto it shows it; without hover every row shows its button.
  const size = page.viewportSize()!;
  await page.setViewportSize({ width: 1280, height: size.height });
  await working.hover();
  await expect.poll(() => shown(frameButton(working))).toBe(1);
  await expect.poll(() => shown(parentMenu)).toBe(0);
  await page.mouse.move(700, 5);
  await expect.poll(() => shown(frameButton(working))).toBe(0);
  await working.focus();
  await page.keyboard.press("Tab");
  await expect(frameButton(working)).toBeFocused();
  await expect.poll(() => shown(frameButton(working))).toBe(1);
  await expect.poll(() => shown(parentMenu)).toBe(0);
  await page.setViewportSize(size);
  // A touch context has no hover: the same sidebar shows every row's button, nested included.
  const phone = await page.context().browser()!.newContext({ ...touch, viewport: { width: 1280, height: size.height }, storageState: await page.context().storageState() });
  const tablet = await phone.newPage();
  await tablet.goto(new URL(page.url()).origin + "/#/epics");
  const tabletSidebar = tablet.getByRole("complementary", nav("Sidebar"));
  await tabletSidebar.getByRole("button", nav("Expand paused-epic")).click();
  const tabletWorking = tabletSidebar.getByRole("listitem").filter({ has: tablet.getByRole("button", nav("Collapse paused-epic")) }).locator("ul").getByRole("link", { name: /Working/ });
  await expect(tabletWorking).toHaveCount(1);
  for (const button of [frameButton(tabletWorking), tabletSidebar.getByRole("button", nav("More actions")).first()]) await expect.poll(() => shown(button)).toBe(1);
  await phone.close();
  // The nested menu: Agent block, Stop while live, Remove; Remove asks first, then the row leaves and so does the session.
  const agentId = (await working.getAttribute("href"))!.split("/agents/")[1];
  expect(await sessionRows(fx, agentId)).toHaveLength(1);
  // Its status words change when it stops, so find the row by its link from here on.
  const agent = sidebar.locator(`a[href="#/agents/${agentId}"]`);
  await agent.click();
  await expect(page).toHaveURL(new RegExp(`#/agents/${agentId}$`));
  await expect(agent).toHaveAttribute("aria-current", "page");
  await frameButton(agent).click();
  await expect(page.getByRole("menu").getByText("Agent", { exact: true })).toBeVisible();
  await expect(page.getByRole("menuitem", nav("Stop"))).toBeVisible();
  await page.getByRole("menuitem", nav("Remove")).click();
  await expect(page.getByRole("dialog")).toBeVisible();
  await page.getByRole("dialog").getByRole("button", nav("Remove")).click();
  await expect(page).toHaveURL(/#\/agents$/);
  await expect(agent).toHaveCount(0);
  await expect.poll(() => sessionRows(fx, agentId)).toHaveLength(0);
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
  // The badge covers only this workspace's projects: demo has cards here, so its follow-ups show, matching the card's words.
  await expect(inbox).toHaveAttribute("aria-label", "Follow-ups · 4 open");
  await inbox.click();
  await expect(followUpsLine).toContainText("2 bugs · 2 ideas");
  await closeMenus();
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
  await expect(inbox).toHaveAttribute("aria-label", "Follow-ups · 4 open");

  // Move from a sidebar row: the row leaves this workspace and shows in the target.
  const rowMenu = (title: string) => row(title).locator("xpath=..").getByRole("button", nav("More actions"));
  // Hover opens a submenu and a click on its trigger toggles it, so hover first and click only when the hover did not open it.
  const openSubmenu = async (trigger: Locator, child: Locator) => {
    await trigger.hover();
    await expect(async () => {
      if (!(await child.isVisible())) await trigger.click({ timeout: 2_000 });
      await expect(child).toBeVisible({ timeout: 1_500 });
    }).toPass({ timeout: 15_000 });
  };
  const moveTo = async (menu: ReturnType<typeof rowMenu>, item: string, to: string) => {
    await menu.click();
    await menuFocused();
    await openSubmenu(page.getByRole("menuitem", nav("Move to workspace")), page.getByRole("menuitem", nav(to)));
    await page.getByRole("menuitem", nav(to)).click();
    await expect(page.getByRole("menu")).toHaveCount(0);
  };
  // The submenu opens to the side of its item, lists every workspace (the Default workspace too) and marks the current one.
  await rowMenu("ws-impl-epic").click();
  const moveItem = page.getByRole("menuitem", nav("Move to workspace"));
  const current = page.getByRole("menu").last().getByRole("menuitem", { name: /^research/ });
  await menuFocused();
  await openSubmenu(moveItem, current);
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
  // Move from an Epics page card is covered by the EpicCard tests; ws-impl-epic stays in gx bugs for the jump dialog.
  await goto(page, "#/epics");

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
  await openSubmenu(page.getByRole("menuitem", nav("Move to workspace")), page.getByRole("menuitem", nav("research")));
  await page.getByRole("menuitem", nav("research")).click();
  await expect(jumpRow("ws-impl-epic")).toContainText("research");
  await jumpRow("ws-impl-epic").click();
  await expect(page).toHaveURL(/#\/e\/demo:ws-impl-epic$/);
  await expect(page.getByRole("button", nav("Workspace: research"))).toBeVisible();
  await expect(row("ws-impl-epic")).toBeVisible();
  await goto(page, "#/epics");

  // A chat row shows in its workspace only.
  await pick("Default workspace");
  await expect(sidebar.getByRole("img", nav("Chat"))).toHaveCount(2); // one per project
  await pick("research");
  await expect(sidebar.getByRole("img", nav("Chat"))).toHaveCount(0);

  // Creates land in the active workspace: the sidebar's + New (design, chat); the Epics page's New Design is covered by the EpicsPage tests.
  const members = async () => JSON.parse(await fx.cli("server", "workspaces", "list", "--json")).workspaces.find((w: { name: string }) => w.name === "research").members as number;
  const startDesign = async (slug: string) => {
    const dialog = page.getByRole("dialog", nav("New Design"));
    await dialog.getByRole("textbox", nav("Epic slug")).fill(slug);
    await dialog.getByRole("textbox", nav("Rough idea")).fill("an idea");
    await dialog.getByRole("button", nav("Start design")).click();
    await expect(dialog).toHaveCount(0);
  };
  const before = await members();
  await sidebar.getByRole("button", { name: "+ New", exact: true }).click();
  await page.getByRole("menuitem", { name: /^New design/ }).click();
  await startDesign("made-from-sidebar");
  await expect.poll(members).toBe(before + 1);
  await goto(page, "#/epics");
  await sidebar.getByRole("button", { name: "+ New", exact: true }).click();
  await page.getByRole("menuitem", { name: /^New chat/ }).click();
  await page.getByRole("button", { name: "Create session" }).click();
  await expect(sidebar.getByRole("img", nav("Chat"))).toHaveCount(1);
  await expect.poll(members).toBe(before + 2);
  // Later steps expect Default's chat alone, since deleting this workspace moves its items there.
  const madeId = (await sidebar.locator("a[data-chat]").getAttribute("href"))!.split("/agents/")[1];
  await fx.cli("agents", "stop", madeId);
  await fx.cli("agents", "remove", madeId);
  await expect(sidebar.getByRole("img", nav("Chat"))).toHaveCount(0);
  await goto(page, "#/epics");

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

  // Hold from the page header and from a jump row is covered by the EpicPage and JumpPalette tests.

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

  // Other sits below On hold and is the same in every workspace. Its Fix agents have a menu without Move or Hold; the Queue agent has none.
  const other = sidebar.getByRole("button", nav("Other"));
  const otherRows = ["fix-demo_cone-offs_s04", "fix-demo", "one-offs-iter-01"];
  const showOther = async () => {
    if ((await other.getAttribute("aria-expanded")) === "false") await other.click();
    for (const name of otherRows) await expect(row(name)).toBeVisible();
  };
  await showOther();
  for (const name of otherRows) await expect(row(name).locator("xpath=..").getByRole("button", nav("More actions"))).toHaveCount(name === "one-offs-iter-01" ? 0 : 1);
  await row("fix-demo").locator("xpath=..").getByRole("button", nav("More actions")).click();
  await expect(page.getByRole("menuitem", nav("Remove"))).toBeVisible();
  await expect(page.getByRole("menuitem", { name: /^(Move|Hold)/ })).toHaveCount(0);
  await closeMenus();
  await pick("Default workspace");
  await showOther();
  // Opening an Other row leaves the workspace alone.
  await row("fix-demo").click();
  await expect(page).toHaveURL(/#\/agents\//);
  await expect(page.getByRole("button", nav("Workspace: Default workspace"))).toBeVisible();
  await goto(page, "#/epics");
  // The Fix agent dialogs (refusal, nesting under the epic) are covered by the pause and restart-run flows.

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

  // The gear menu holds AFK, Vi mode, Sort, Restart server, More settings, Cleanup and Tips; Sort is per workspace and follows the CLI.
  const gear = page.getByRole("button", nav("Menu"));
  const gearItems = page.getByRole("menu").locator('[role^="menuitem"]');
  await gear.click();
  await expect(gearItems).toHaveText([/^AFK$/, /^Vi mode$/, /^Sort · Default workspace/, /^Restart server$/, /^More settings\.\.\.$/, /^Cleanup/, /^Tips$/]);
  const sortOptions = page.getByRole("menuitemradio");
  const openSort = async (workspace: string) => {
    await closeMenus();
    await gear.click();
    await menuFocused();
    await openSubmenu(page.getByRole("menuitem", { name: new RegExp(`^Sort · ${workspace}`) }), sortOptions.first());
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

  // Under "By last activity", Alt+1 opens the first row of the sorted list.
  await fx.cli("server", "workspaces", "sort", "Default workspace", "activity");
  await goto(page, "#/epics");
  const sortedFirst = (await sidebar.getByRole("link").first().getAttribute("href"))!.slice(1);
  await page.keyboard.press("Alt+1");
  await expect.poll(() => new URL(page.url()).hash.slice(1)).toBe(sortedFirst);
  await fx.cli("server", "workspaces", "sort", "Default workspace", "needs-you");
  await goto(page, "#/epics");

  // A row menu opens from the keyboard: Tab from the row to its menu button, Enter, arrow to an item, Enter.
  const heldBefore = Number(/\d+/.exec((await onHold.textContent())!)![0]);
  const held = onHold.locator("xpath=following::a[1]");
  await onHold.click();
  await held.focus();
  await page.keyboard.press("Tab");
  await expect(held.locator("xpath=..").getByRole("button", nav("More actions"))).toBeFocused();
  await page.keyboard.press("Enter");
  await menuFocused();
  await page.keyboard.press("ArrowDown");
  await expect(page.getByRole("menuitem", nav("Take off hold"))).toHaveAttribute("data-highlighted", "");
  await page.keyboard.press("Enter");
  await expect(page.getByRole("menu")).toHaveCount(0);
  await expect(onHold).toHaveText(`On hold (${heldBefore - 1})`);
  await pick("scratch");
  await openSort("scratch");
  await expect(sortOptions.nth(0)).toHaveAttribute("aria-checked", "true");
  await closeMenus();
  await pick("Default workspace");
  await gear.click();
  await page.getByRole("menuitem", nav("More settings...")).click();
  await expect(page).toHaveURL(/#\/settings$/);
  await goto(page, "#/epics");

  // At 1280px the sidebar is docked: no swipe moves it.
  await swipe(page, [300, 600], [100, 610]);
  await expect(sidebar).toBeVisible();
  await swipe(page, [150, 600], [300, 610]);
  await expect(sidebar).toBeVisible();

  // At 744px the sidebar slides over: picking a row closes it, the jump menu is a popover, no page scrolls sideways.
  await page.setViewportSize({ width: 744, height: 900 });
  await goto(page, "#/epics");
  await expect(sidebar).toBeHidden();
  await page.getByRole("button", nav("Show sidebar")).click();
  await expect(sidebar).toBeVisible();
  await swipe(page, [300, 600], [100, 610]);
  await expect(sidebar).toBeHidden();
  await swipe(page, [150, 600], [300, 610]);
  await expect(sidebar).toBeVisible();
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
  // Stats and Settings are checked for sideways scroll in their own flows.
  for (const hash of [ticketHash, "#/e/demo:ws-design"]) {
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
  // Swipe left closes it; a swipe right from x=150 opens it and one from the system's back-swipe edge (x=5) does not.
  await swipe(page, [200, 700], [200, 300]);
  await expect(sidebar).toBeVisible();
  await swipe(page, [250, 600], [100, 610]);
  await expect(sidebar).toBeHidden();
  await swipe(page, [5, 600], [150, 610]);
  await expect(sidebar).toBeHidden();
  await swipe(page, [150, 600], [300, 610]);
  await expect(sidebar).toBeVisible();
  // The gear menu's dim covers the Hide sidebar toggle: a tap on the toggle and a swipe through the dim still close the menu and the sidebar.
  const gearMenu = page.getByRole("menu");
  await page.getByRole("button", nav("Menu")).click();
  await expect(gearMenu).toBeVisible();
  // Measured once the sidebar has finished sliding in.
  const toggleBox = (await sidebar.getByRole("button", nav("Hide sidebar")).boundingBox())!;
  await page.mouse.click(toggleBox.x + toggleBox.width / 2, toggleBox.y + toggleBox.height / 2);
  await expect(gearMenu).toHaveCount(0);
  await expect(sidebar).toBeHidden();
  await page.getByRole("button", nav("Show sidebar")).click();
  await page.getByRole("button", nav("Menu")).click();
  await expect(gearMenu).toBeVisible();
  await swipe(page, [380, 700], [250, 705]);
  await expect(gearMenu).toHaveCount(0);
  await expect(sidebar).toBeHidden();
  await page.getByRole("button", nav("Show sidebar")).click();
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

  // Returning to the app restarts the stream and refetches: with the stream blocked, a rename from the CLI
  // reaches the sidebar only through visibilitychange and pageshow, with no reload.
  await goto(page, "#/epics");
  const unblock = await blockEvents(page);
  await fx.cli("server", "workspaces", "rename", "scratch", "archive");
  await page.evaluate(() => document.dispatchEvent(new Event("visibilitychange")));
  await page.evaluate(() => window.dispatchEvent(new Event("pageshow")));
  await unblock();
  await switcher.click();
  await expect(page.getByRole("menuitem", { name: /^archive/ })).toBeVisible();
  await closeMenus();

  // After a real menu closes, focus lands in the dialog: the first field at 1280px, the panel itself at 390px.
  await page.setViewportSize({ width: 1280, height: 900 });
  await expect(sidebar).toBeVisible();
  const dialog = page.getByRole("dialog");
  const newItem = async (name: string) => {
    await sidebar.getByRole("button", { name: "+ New" }).click();
    await page.getByRole("menuitem", { name }).click();
    await expect(dialog).toBeVisible();
  };
  const closeDialog = async () => {
    await page.keyboard.press("Escape");
    await expect(dialog).toHaveCount(0);
  };
  await newItem("New workspace…");
  await expect(dialog.getByRole("textbox", nav("Workspace name"))).toBeFocused();
  await closeDialog();
  await switcher.click();
  await page.getByRole("menuitem", { name: "New workspace…" }).click();
  await expect(dialog.getByRole("textbox", nav("Workspace name"))).toBeFocused();
  await closeDialog();
  await newItem("New chat");
  await expect(dialog.getByRole("textbox", nav("Name"))).toBeVisible();
  await expect(dialog.getByRole("textbox", nav("Name"))).toBeFocused();
  await closeDialog();
  for (let n = 0; n < 4; n++) {
    await newItem("New design");
    await expect(dialog.getByRole("textbox", nav("Epic slug"))).toBeFocused();
    await closeDialog();
  }
  await page.setViewportSize({ width: 390, height: 844 });
  await expect(sidebar).toBeVisible();
  for (const name of ["New chat", "New workspace…"]) {
    await newItem(name);
    await expect(dialog).toBeFocused();
    await closeDialog();
  }
  await page.setViewportSize({ width: 1280, height: 900 });

  // New chat and New design start on the page's project, then the Epics filter's single project, then the last used one.
  // Every check opens a blank form: the previous one is closed with nothing typed, so no draft carries over.
  const chatForm = page.getByRole("dialog", { name: "New session" });
  const designForm = dialog.getByRole("textbox", nav("Epic slug"));
  const chatProject = chatForm.getByRole("combobox", nav("Project"));
  const designProject = dialog.getByRole("combobox", nav("Project"));
  const formsShow = async (project: string) => {
    await newItem("New chat");
    await expect(chatProject).toHaveValue(project);
    await closeDialog();
    await newItem("New design");
    await expect(designForm).toBeVisible();
    await expect(designProject).toHaveValue(project);
    await closeDialog();
  };
  const sideChat = (await sessions(fx)).find((s) => s.project === "side" && !s.design)!;
  await goto(page, "#/d/side:side-design");
  await expect(page.getByRole("heading", { name: /side-design/ }).first()).toBeVisible();
  await formsShow("side");
  await goto(page, `#/agents/${sideChat.id}`);
  await expect(page).toHaveURL(new RegExp(`#/agents/${sideChat.id}$`));
  await newItem("New chat");
  await expect(chatProject).toHaveValue("side");
  await closeDialog();

  // On the Epics page, a filter that picks the one project decides.
  await goto(page, "#/epics");
  await page.getByRole("button", nav("Project")).click();
  const projectList = page.getByRole("dialog", nav("Projects"));
  const allProjects = projectList.getByRole("button", nav("All projects"));
  if (await allProjects.isEnabled()) await allProjects.click();
  await projectList.getByRole("checkbox", nav("side")).check();
  await page.keyboard.press("Escape");
  await expect(projectList).toHaveCount(0);
  await formsShow("side");

  // With the filter cleared, a chat created in the second project becomes the last used one, which a Stats page form starts on.
  await page.getByRole("button", nav("Project")).click();
  await projectList.getByRole("button", nav("All projects")).click();
  await page.keyboard.press("Escape");
  await expect(projectList).toHaveCount(0);
  await goto(page, "#/stats");
  await newItem("New chat");
  await expect(chatProject).toHaveValue("demo");
  await chatProject.selectOption("side");
  await chatForm.getByRole("textbox", nav("Name")).fill("side-made");
  await chatForm.getByRole("textbox", nav("First prompt")).fill("hi");
  await chatForm.getByRole("button", nav("Create session")).click();
  await expect(chatForm).toHaveCount(0);
  await goto(page, "#/stats");
  await formsShow("side");

  // The footer's boxes are one size with their icons centered, at 1280px and 390px, and the badges agree.
  const footer = sidebar.locator("footer");
  const epicsBox = footer.getByRole("link", { name: /^Epics/ });
  const followUpsBox = footer.getByRole("button", { name: /^Follow-ups/ });
  const statsBox = footer.getByRole("link", { name: /^Stats/ });
  const gearBox = footer.getByRole("button", nav("Menu"));
  const boxes = [epicsBox, followUpsBox, statsBox, gearBox];
  const measure = async (locator: Locator) => {
    await expect(locator).toBeVisible();
    return (await locator.boundingBox())!;
  };
  const checkBoxes = async () => {
    const sizes = [];
    for (const box of boxes) {
      const b = await measure(box);
      const i = await measure(box.locator("svg"));
      expect(i.x + i.width / 2).toBeCloseTo(b.x + b.width / 2, 0);
      expect(i.y + i.height / 2).toBeCloseTo(b.y + b.height / 2, 0);
      sizes.push([b.width, b.height, b.y]);
    }
    for (const s of sizes) expect(s).toEqual(sizes[0]);
    return sizes[0][0];
  };
  const badgeOf = async (box: Locator) => {
    const badge = box.locator('[data-part="badge"]');
    await expect(badge).toBeVisible();
    const b = (await badge.boundingBox())!;
    const p = (await box.boundingBox())!;
    const css = await badge.evaluate((el) => {
      const s = getComputedStyle(el);
      return [s.fontSize, s.fontWeight, s.lineHeight];
    });
    return { w: b.width, h: b.height, top: b.y - p.y, right: p.x + p.width - (b.x + b.width), css };
  };
  await page.setViewportSize({ width: 1280, height: 900 });
  await expect(epicsBox).toHaveCSS("width", "25px");
  expect(await checkBoxes()).toBe(25);
  expect(await badgeOf(epicsBox)).toEqual(await badgeOf(followUpsBox));
  // The burger icon is three horizontal paths and no circle.
  await expect(gearBox.locator("svg path")).toHaveCount(3);
  await expect(gearBox.locator("svg circle")).toHaveCount(0);
  for (const d of await gearBox.locator("svg path").evaluateAll((ps) => ps.map((p) => p.getAttribute("d")!))) expect(d).toMatch(/^M[\d.]+ [\d.]+h[\d.]+$/);

  // With the health warning, paused and offline indicators all shown, no box shrinks and nothing overflows the sidebar.
  const width = await checkBoxes();
  await fx.setHealth("cpu=75 mem=85");
  await fx.cli("server", "queue", "pause");
  const indicators = footer.getByRole("img", { name: /^(System health is high or critical|Server queue is paused|Disconnected from the server)$/ });
  await expect(indicators).toHaveCount(2);
  await fx.stopServer();
  await expect(indicators).toHaveCount(3);
  const sidebarBox = (await measure(sidebar))!;
  expect(sidebarBox.width).toBeLessThanOrEqual(280);
  expect(await checkBoxes()).toBe(width);
  for (let n = 0; n < 3; n++) {
    const b = await measure(indicators.nth(n));
    expect(b.width).toBe(width);
    expect(b.x + b.width).toBeLessThanOrEqual(sidebarBox.x + sidebarBox.width);
  }
  await expect(footer).toBeVisible();
  expect(await footer.evaluate((el) => el.scrollWidth <= el.clientWidth)).toBe(true);

  // Repeat at 390px.
  await page.setViewportSize({ width: 390, height: 844 });
  await expect(sidebar).toBeVisible();
  await expect(epicsBox).toHaveCSS("width", "34px");
  expect(await checkBoxes()).toBe(34);
  // The touch footer keeps its boxes at 34px with every indicator shown; only the 280px sidebar is held to fit.
  for (let n = 0; n < 3; n++) expect((await measure(indicators.nth(n))).width).toBe(34);
});
